"""
MCP (Model Context Protocol) 客户端 — MiniCC 的外部能力接入总线。

设计理念（参考 Claude Code MCP 实现）：
- MCP 接进来的不只是工具，还有资源、prompts 和 skills
- 外部能力统一"翻译"成 MiniCC 的运行时对象（BaseTool）
- MCP 能力不是静态的，server 变了会动态刷新
- 对 MCP 不是"能接就接"，而是带着权限、缓存、交互和安全边界去接
"""

from __future__ import annotations

import asyncio
import json
import logging
import uuid
from abc import ABC, abstractmethod
from typing import Any, Callable, Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolRegistry

logger = logging.getLogger("minicc.mcp")

# ── 数据模型 ─────────────────────────────────────────────


class MCPServerConfig(BaseModel):
    """MCP 服务器配置。"""
    command: Optional[str] = None  # stdio 模式
    args: list[str] = Field(default_factory=list)
    url: Optional[str] = None  # SSE/HTTP 模式
    transport: str = "stdio"  # stdio | sse | http | ws
    env: dict[str, str] = Field(default_factory=dict)


class MCPToolDefinition(BaseModel):
    """MCP 工具定义（来自 tools/list 响应）。"""
    name: str
    description: str = ""
    inputSchema: dict[str, Any] = Field(default_factory=dict)


class MCPResourceDefinition(BaseModel):
    """MCP 资源定义（来自 resources/list 响应）。"""
    uri: str
    name: str
    description: str = ""
    mimeType: Optional[str] = None


# ── JSON-RPC 会话 ─────────────────────────────────────────


class JSONRPCError(Exception):
    def __init__(self, code: int, message: str) -> None:
        self.code = code
        self.message = message
        super().__init__(f"[{code}] {message}")


class MCPClientSession:
    """MCP JSON-RPC 会话层。管理请求/响应的配对。"""

    def __init__(self, stdin, stdout) -> None:
        self._stdin = stdin
        self._stdout = stdout
        self._pending: dict[str, asyncio.Future] = {}
        self._reader_task: Optional[asyncio.Task] = None
        self._request_id = 0
        self._initialized = False

    async def initialize(self) -> dict:
        """发送 initialize 请求，完成 MCP 握手。"""
        result = await self._request("initialize", {
            "protocolVersion": "2025-03-26",
            "capabilities": {},
            "clientInfo": {"name": "minicc", "version": "0.1.0"},
        })
        self._initialized = True
        # 发送 initialized 通知
        await self._notify("notifications/initialized", {})
        return result

    async def list_tools(self) -> list[MCPToolDefinition]:
        """调用 tools/list。"""
        result = await self._request("tools/list", {})
        return [MCPToolDefinition(**t) for t in result.get("tools", [])]

    async def call_tool(self, name: str, arguments: dict) -> dict:
        """调用 tools/call。"""
        return await self._request("tools/call", {
            "name": name,
            "arguments": arguments,
        })

    async def list_resources(self) -> list[MCPResourceDefinition]:
        """调用 resources/list。"""
        result = await self._request("resources/list", {})
        return [MCPResourceDefinition(**r) for r in result.get("resources", [])]

    async def read_resource(self, uri: str) -> dict:
        """调用 resources/read。"""
        return await self._request("resources/read", {"uri": uri})

    async def _request(self, method: str, params: dict) -> dict:
        """发送 JSON-RPC 请求并等待响应。"""
        self._request_id += 1
        req_id = str(self._request_id)
        future: asyncio.Future = asyncio.get_event_loop().create_future()
        self._pending[req_id] = future

        msg = json.dumps({
            "jsonrpc": "2.0",
            "id": req_id,
            "method": method,
            "params": params,
        })
        self._stdin.write((msg + "\n").encode("utf-8"))
        await self._stdin.drain()

        try:
            return await asyncio.wait_for(future, timeout=30)
        except asyncio.TimeoutError:
            self._pending.pop(req_id, None)
            raise JSONRPCError(-1, f"Request timeout: {method}")

    async def _notify(self, method: str, params: dict) -> None:
        """发送 JSON-RPC 通知（无响应）。"""
        msg = json.dumps({
            "jsonrpc": "2.0",
            "method": method,
            "params": params,
        })
        self._stdin.write((msg + "\n").encode("utf-8"))
        await self._stdin.drain()

    def _start_reader(self) -> None:
        """启动后台读取协程。"""
        async def reader():
            buffer = ""
            while True:
                line = await self._stdout.readline()
                if not line:
                    break
                buffer += line.decode("utf-8")
                # MCP 使用新行分隔 JSON-RPC 消息
                while "\n" in buffer:
                    msg_str, buffer = buffer.split("\n", 1)
                    msg_str = msg_str.strip()
                    if not msg_str:
                        continue
                    try:
                        msg = json.loads(msg_str)
                        self._handle_message(msg)
                    except json.JSONDecodeError:
                        logger.warning("MCP: invalid JSON: %s", msg_str[:200])

        self._reader_task = asyncio.create_task(reader())

    def _handle_message(self, msg: dict) -> None:
        """处理收到的 JSON-RPC 消息。"""
        # 响应消息
        if "id" in msg:
            future = self._pending.pop(str(msg["id"]), None)
            if future and not future.done():
                if "error" in msg:
                    err = msg["error"]
                    future.set_exception(JSONRPCError(err.get("code", 0), err.get("message", "")))
                else:
                    future.set_result(msg.get("result", {}))

        # 通知消息（如 tools/list_changed）
        elif "method" in msg:
            method = msg["method"]
            params = msg.get("params", {})
            if method == "notifications/tools/list_changed":
                logger.info("MCP: tools/list_changed received")
                # 通知监听者刷新工具列表
                if self._on_tools_changed:
                    self._on_tools_changed()
            elif method == "notifications/resources/list_changed":
                logger.info("MCP: resources/list_changed received")
            elif method == "notifications/message":
                logger.info("MCP server message: %s", params.get("message", ""))

    def set_tools_changed_callback(self, callback: Callable) -> None:
        """注册工具列表变更回调（tools/list_changed 时触发）。"""
        self._on_tools_changed = callback

    async def close(self) -> None:
        """关闭会话。"""
        if self._reader_task:
            self._reader_task.cancel()
            try:
                await asyncio.wait_for(self._reader_task, timeout=2)
            except (asyncio.TimeoutError, asyncio.CancelledError):
                pass
        # 取消所有待处理的请求
        for future in self._pending.values():
            if not future.done():
                future.cancel()
        self._pending.clear()

    async def __aenter__(self):
        self._start_reader()
        return self

    async def __aexit__(self, *args):
        await self.close()


# ── MCP 客户端 ────────────────────────────────────────────


class MCPClient:
    """MCP 客户端。管理一个 MCP 服务器的完整生命周期。

    支持三种传输方式：
    - stdio: 本地子进程，通过 stdin/stdout 通信
    - sse/http: 远程服务器，通过 HTTP SSE 通信
    - ws: WebSocket 远程连接
    """

    def __init__(self, name: str, config: MCPServerConfig) -> None:
        self.name = name
        self.config = config
        self._process: Optional[asyncio.subprocess.Process] = None
        self._session: Optional[MCPClientSession] = None
        self._tool_cache: list[MCPToolDefinition] = []
        self._resource_cache: list[MCPResourceDefinition] = []
        self._on_tools_changed: Optional[Callable] = None

    async def connect(self) -> bool:
        """连接到 MCP 服务器。返回是否成功。"""
        try:
            if self.config.transport == "stdio":
                await self._connect_stdio()
            elif self.config.transport in ("sse", "http"):
                await self._connect_sse()
            elif self.config.transport == "ws":
                await self._connect_ws()
            else:
                raise ValueError(f"Unsupported transport: {self.config.transport}")

            await self._session.initialize()
            self._session.set_tools_changed_callback(self._on_tools_changed or (lambda: None))

            # 首次拉取工具和资源
            await self.refresh_tools()
            await self.refresh_resources()

            logger.info("MCP connected: %s (%s)", self.name, self.config.transport)
            return True

        except Exception as exc:
            logger.warning("MCP connect failed: %s — %s", self.name, exc)
            return False

    async def _connect_stdio(self) -> None:
        """通过 stdio 连接到本地 MCP 服务器进程。"""
        cmd = [self.config.command] + self.config.args if self.config.command else []
        if not cmd:
            raise ValueError(f"MCP server '{self.name}' has no command configured")

        self._process = await asyncio.create_subprocess_exec(
            *cmd,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            env={**__import__("os").environ, **self.config.env} if self.config.env else None,
        )
        self._session = MCPClientSession(self._process.stdin, self._process.stdout)

    async def _connect_sse(self) -> None:
        """通过 SSE/HTTP 连接到远程 MCP 服务器。"""
        import httpx
        url = self.config.url or ""
        if not url:
            raise ValueError(f"MCP server '{self.name}' has no URL configured")

        # SSE 模式下，我们使用 httpx 发送请求，通过流式响应读取事件
        # 简化实现：使用 JSON-RPC over HTTP POST
        self._http_client = httpx.AsyncClient(base_url=url, timeout=30)
        self._session = MCPHTTPSession(self._http_client)

    async def _connect_ws(self) -> None:
        """通过 WebSocket 连接到远程 MCP 服务器。"""
        import websockets
        url = self.config.url or ""
        self._ws = await websockets.connect(url)
        self._session = MCPWebSocketSession(self._ws)

    async def refresh_tools(self) -> list[MCPToolDefinition]:
        """重新拉取工具列表并更新缓存。"""
        if not self._session:
            return []
        try:
            tools = await self._session.list_tools()
            self._tool_cache = tools
            logger.debug("MCP tools refreshed: %s (%d tools)", self.name, len(tools))
        except Exception as exc:
            logger.warning("MCP tools refresh failed: %s — %s", self.name, exc)
        return self._tool_cache

    async def refresh_resources(self) -> list[MCPResourceDefinition]:
        """重新拉取资源列表并更新缓存。"""
        if not self._session:
            return []
        try:
            resources = await self._session.list_resources()
            self._resource_cache = resources
            logger.debug("MCP resources refreshed: %s (%d resources)", self.name, len(resources))
        except Exception as exc:
            logger.warning("MCP resources refresh failed: %s — %s", self.name, exc)
        return self._resource_cache

    async def call_tool(self, name: str, arguments: dict) -> dict:
        """调用 MCP 工具。"""
        if not self._session:
            raise RuntimeError(f"MCP client '{self.name}' not connected")
        return await self._session.call_tool(name, arguments)

    async def read_resource(self, uri: str) -> dict:
        """读取 MCP 资源。"""
        if not self._session:
            raise RuntimeError(f"MCP client '{self.name}' not connected")
        return await self._session.read_resource(uri)

    def get_tools(self) -> list[MCPToolDefinition]:
        """获取缓存的工具列表。"""
        return self._tool_cache

    def get_resources(self) -> list[MCPResourceDefinition]:
        """获取缓存的资源列表。"""
        return self._resource_cache

    async def shutdown(self) -> None:
        """关闭 MCP 连接。"""
        try:
            if self._session:
                await self._session.close()
            if self._process:
                self._process.terminate()
                try:
                    await asyncio.wait_for(self._process.wait(), timeout=5)
                except asyncio.TimeoutError:
                    self._process.kill()
            if hasattr(self, "_http_client"):
                await self._http_client.aclose()
            if hasattr(self, "_ws"):
                await self._ws.close()
        except Exception as exc:
            logger.warning("MCP shutdown error: %s — %s", self.name, exc)
        logger.info("MCP shutdown: %s", self.name)

    def set_tools_changed_callback(self, callback: Callable) -> None:
        """注册工具列表变更回调。"""
        self._on_tools_changed = callback
        if self._session:
            self._session.set_tools_changed_callback(callback)

    @property
    def connected(self) -> bool:
        return self._session is not None and self._session._initialized


# ── HTTP/WebSocket Session 适配 ──────────────────────────


class MCPHTTPSession:
    """基于 HTTP 的 MCP 会话（简化版：JSON-RPC over HTTP POST）。"""

    def __init__(self, client) -> None:
        self._client = client
        self._request_id = 0
        self._initialized = False

    async def initialize(self) -> dict:
        result = await self._request("initialize", {
            "protocolVersion": "2025-03-26",
            "capabilities": {},
            "clientInfo": {"name": "minicc", "version": "0.1.0"},
        })
        self._initialized = True
        return result

    async def list_tools(self) -> list[MCPToolDefinition]:
        result = await self._request("tools/list", {})
        return [MCPToolDefinition(**t) for t in result.get("tools", [])]

    async def call_tool(self, name: str, arguments: dict) -> dict:
        return await self._request("tools/call", {"name": name, "arguments": arguments})

    async def list_resources(self) -> list[MCPResourceDefinition]:
        result = await self._request("resources/list", {})
        return [MCPResourceDefinition(**r) for r in result.get("resources", [])]

    async def read_resource(self, uri: str) -> dict:
        return await self._request("resources/read", {"uri": uri})

    async def _request(self, method: str, params: dict) -> dict:
        self._request_id += 1
        resp = await self._client.post("/", json={
            "jsonrpc": "2.0",
            "id": str(self._request_id),
            "method": method,
            "params": params,
        })
        resp.raise_for_status()
        data = resp.json()
        if "error" in data:
            raise JSONRPCError(data["error"]["code"], data["error"]["message"])
        return data.get("result", {})

    def set_tools_changed_callback(self, callback: Callable) -> None:
        pass  # HTTP 模式暂不支持推送通知

    async def close(self) -> None:
        pass


class MCPWebSocketSession:
    """基于 WebSocket 的 MCP 会话。"""

    def __init__(self, ws) -> None:
        self._ws = ws
        self._request_id = 0
        self._pending: dict[str, asyncio.Future] = {}
        self._initialized = False

    async def initialize(self) -> dict:
        result = await self._request("initialize", {
            "protocolVersion": "2025-03-26",
            "capabilities": {},
            "clientInfo": {"name": "minicc", "version": "0.1.0"},
        })
        self._initialized = True
        return result

    async def list_tools(self) -> list[MCPToolDefinition]:
        result = await self._request("tools/list", {})
        return [MCPToolDefinition(**t) for t in result.get("tools", [])]

    async def call_tool(self, name: str, arguments: dict) -> dict:
        return await self._request("tools/call", {"name": name, "arguments": arguments})

    async def list_resources(self) -> list[MCPResourceDefinition]:
        result = await self._request("resources/list", {})
        return [MCPResourceDefinition(**r) for r in result.get("resources", [])]

    async def read_resource(self, uri: str) -> dict:
        return await self._request("resources/read", {"uri": uri})

    async def _request(self, method: str, params: dict) -> dict:
        self._request_id += 1
        req_id = str(self._request_id)
        future: asyncio.Future = asyncio.get_event_loop().create_future()
        self._pending[req_id] = future

        await self._ws.send(json.dumps({
            "jsonrpc": "2.0", "id": req_id, "method": method, "params": params,
        }))

        try:
            return await asyncio.wait_for(future, timeout=30)
        except asyncio.TimeoutError:
            self._pending.pop(req_id, None)
            raise JSONRPCError(-1, f"Request timeout: {method}")

    def set_tools_changed_callback(self, callback: Callable) -> None:
        pass

    async def close(self) -> None:
        for future in self._pending.values():
            if not future.done():
                future.cancel()
        self._pending.clear()


# ── MCP 工具适配器 ────────────────────────────────────────


class MCPToolAdapter(BaseTool):
    """将 MCP 工具定义包装为 MiniCC 的 BaseTool。

    MCP 工具和内置工具在 ToolRegistry 中一视同仁，
    LLM 不需要知道工具来源是本地还是 MCP。
    """

    def __init__(self, client: MCPClient, definition: MCPToolDefinition) -> None:
        super().__init__()
        self._client = client
        self._definition = definition
        self.name = f"mcp_{client.name}_{definition.name}"
        self.description = f"[MCP/{client.name}] {definition.description}"
        self.permission_level = PermissionLevel.WRITE  # MCP 工具默认需要审批

        # 从 JSON Schema 动态构建 input_schema
        schema = definition.inputSchema or {"type": "object", "properties": {}}
        self._input_schema_dict = schema

    @property
    def input_schema(self):
        """动态构建 Pydantic 模型。"""
        from pydantic import create_model
        fields = {}
        props = self._input_schema_dict.get("properties", {})
        required = set(self._input_schema_dict.get("required", []))
        for name, prop in props.items():
            ptype = str if prop.get("type") == "string" else (
                int if prop.get("type") == "integer" else (
                    float if prop.get("type") == "number" else Any
                )
            )
            default = ... if name in required else None
            fields[name] = (Optional[ptype], default) if name not in required else (ptype, ...)
        return create_model(f"{self.name}_input", **fields)  # type: ignore

    async def execute(self, input_data) -> ToolResult:
        try:
            result = await self._client.call_tool(
                self._definition.name,
                input_data.model_dump() if hasattr(input_data, "model_dump") else dict(input_data),
            )
            # 提取文本内容
            content = result.get("content", [])
            output = "\n".join(
                c.get("text", "") for c in content if c.get("type") == "text"
            )
            return ToolResult(tool_call_id="", output=output)
        except Exception as exc:
            return ToolResult(tool_call_id="", output=f"MCP tool error: {exc}", is_error=True)


class ListMcpResourcesTool(BaseTool):
    """列出所有已连接 MCP 服务器的可用资源。"""

    name = "list_mcp_resources"
    description = "List all available resources from connected MCP servers."
    permission_level = PermissionLevel.READ

    def __init__(self, clients: dict[str, MCPClient]) -> None:
        super().__init__()
        self._clients = clients

    async def execute(self, input_data) -> ToolResult:
        lines = []
        for name, client in self._clients.items():
            resources = client.get_resources()
            if resources:
                lines.append(f"[{name}]")
                for r in resources:
                    lines.append(f"  {r.uri} — {r.name}")
                    if r.description:
                        lines.append(f"    {r.description}")
        if not lines:
            return ToolResult(tool_call_id="", output="No MCP resources available.")
        return ToolResult(tool_call_id="", output="\n".join(lines))


class ReadMcpResourceTool(BaseTool):
    """读取指定 MCP 资源内容。"""

    name = "read_mcp_resource"
    description = "Read content from a specific MCP resource URI."
    permission_level = PermissionLevel.READ

    def __init__(self, clients: dict[str, MCPClient]) -> None:
        super().__init__()
        self._clients = clients

    async def execute(self, input_data) -> ToolResult:
        uri = input_data.uri if hasattr(input_data, "uri") else ""
        server = input_data.server if hasattr(input_data, "server") else ""

        client = self._clients.get(server)
        if not client:
            return ToolResult(tool_call_id="", output=f"MCP server not found: {server}", is_error=True)

        try:
            result = await client.read_resource(uri)
            content = result.get("contents", [])
            output = "\n".join(
                c.get("text", "") for c in content if c.get("text")
            )
            return ToolResult(tool_call_id="", output=output)
        except Exception as exc:
            return ToolResult(tool_call_id="", output=f"Read resource error: {exc}", is_error=True)
