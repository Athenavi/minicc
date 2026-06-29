"""
LSP (Language Server Protocol) 客户端 — MiniCC 的语言智能。

与 MCP 的差异：
- MCP = 向外扩能力（接入外部系统、工具、资源）
- LSP = 向内补语义（诊断、符号、引用、跳转定义）

Claude Code 中使用 LSP 获取：
- go_to_definition: 跳转到符号定义
- find_references: 查找所有引用
- hover: 悬停文档提示
- completion: 代码补全
"""

from __future__ import annotations

import asyncio
import json
import logging
import uuid
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool

logger = logging.getLogger("minicc.lsp")


class Location(BaseModel):
    """LSP 位置信息。"""
    uri: str
    range: dict[str, Any] = Field(default_factory=dict)


class LSPConfig(BaseModel):
    """LSP 服务器配置。"""
    language: str
    command: list[str]
    args: list[str] = Field(default_factory=list)
    env: dict[str, str] = Field(default_factory=dict)


class LSPClient:
    """LSP 客户端。管理一个语言服务器的完整生命周期。

    惰性初始化：连接在首次调用时建立，非启动时。
    每个语言一个 LSP 客户端实例。
    """

    def __init__(self, config: LSPConfig) -> None:
        self.config = config
        self._process: Optional[asyncio.subprocess.Process] = None
        self._reader: Optional[asyncio.Task] = None
        self._pending: dict[str, asyncio.Future] = {}
        self._request_id = 0
        self._initialized = False
        self._capabilities: dict[str, Any] = {}

    async def start(self) -> bool:
        """启动 LSP 服务器进程并初始化。返回是否成功。"""
        try:
            self._process = await asyncio.create_subprocess_exec(
                *self.config.command,
                *self.config.args,
                stdin=asyncio.subprocess.PIPE,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )

            self._reader = asyncio.create_task(self._read_loop())

            # 发送 initialize 请求
            result = await self._request("initialize", {
                "processId": None,
                "capabilities": {},
                "rootUri": None,
            })
            self._capabilities = result.get("capabilities", {})

            # 发送 initialized 通知
            await self._notify("initialized", {})

            self._initialized = True
            logger.info("LSP started: %s", self.config.language)
            return True

        except Exception as exc:
            logger.warning("LSP start failed: %s — %s", self.config.language, exc)
            return False

    async def _read_loop(self) -> None:
        """读取 LSP 服务器的 JSON-RPC 响应。"""
        buffer = ""
        content_length = 0

        while True:
            line = await self._process.stdout.readline()
            if not line:
                break

            decoded = line.decode("utf-8", errors="replace")

            # LSP 使用 HTTP 风格的头部
            if decoded.startswith("Content-Length:"):
                content_length = int(decoded.strip().split(":", 1)[1].strip())
            elif decoded.strip() == "" and content_length > 0:
                # 读取消息体
                body = await self._process.stdout.readexactly(content_length)
                content_length = 0
                try:
                    msg = json.loads(body.decode("utf-8"))
                    self._handle_message(msg)
                except json.JSONDecodeError:
                    logger.warning("LSP: invalid JSON response")

    def _handle_message(self, msg: dict) -> None:
        """处理 LSP 响应。"""
        if "id" in msg:
            future = self._pending.pop(str(msg["id"]), None)
            if future and not future.done():
                if "error" in msg:
                    future.set_exception(RuntimeError(msg["error"].get("message", "LSP error")))
                else:
                    future.set_result(msg.get("result", {}))

    async def _request(self, method: str, params: dict) -> dict:
        """发送 LSP 请求。"""
        self._request_id += 1
        req_id = str(self._request_id)
        future: asyncio.Future = asyncio.get_event_loop().create_future()
        self._pending[req_id] = future

        body = json.dumps({
            "jsonrpc": "2.0",
            "id": req_id,
            "method": method,
            "params": params,
        })
        header = f"Content-Length: {len(body)}\r\n\r\n"
        self._process.stdin.write((header + body).encode("utf-8"))
        await self._process.stdin.drain()

        try:
            return await asyncio.wait_for(future, timeout=15)
        except asyncio.TimeoutError:
            self._pending.pop(req_id, None)
            raise TimeoutError(f"LSP request timeout: {method}")

    async def _notify(self, method: str, params: dict) -> None:
        """发送 LSP 通知（无响应）。"""
        body = json.dumps({"jsonrpc": "2.0", "method": method, "params": params})
        header = f"Content-Length: {len(body)}\r\n\r\n"
        self._process.stdin.write((header + body).encode("utf-8"))
        await self._process.stdin.drain()

    # ── 公开 API ──

    async def go_to_definition(self, file_path: str, line: int, col: int) -> list[Location]:
        """跳转到定义。"""
        result = await self._request("textDocument/definition", {
            "textDocument": {"uri": f"file://{file_path}"},
            "position": {"line": line, "character": col},
        })
        if isinstance(result, list):
            return [Location(**loc) for loc in result]
        if isinstance(result, dict):
            return [Location(**result)]
        return []

    async def find_references(self, file_path: str, line: int, col: int) -> list[Location]:
        """查找引用。"""
        result = await self._request("textDocument/references", {
            "textDocument": {"uri": f"file://{file_path}"},
            "position": {"line": line, "character": col},
            "context": {"includeDeclaration": True},
        })
        if isinstance(result, list):
            return [Location(**loc) for loc in result]
        return []

    async def hover(self, file_path: str, line: int, col: int) -> Optional[str]:
        """悬停文档。"""
        result = await self._request("textDocument/hover", {
            "textDocument": {"uri": f"file://{file_path}"},
            "position": {"line": line, "character": col},
        })
        if not result:
            return None
        contents = result.get("contents", {})
        if isinstance(contents, str):
            return contents
        if isinstance(contents, dict):
            return contents.get("value", "")
        if isinstance(contents, list):
            return "\n".join(
                c.get("value", "") if isinstance(c, dict) else str(c)
                for c in contents
            )
        return str(contents) if contents else None

    async def completion(self, file_path: str, line: int, col: int) -> list[str]:
        """代码补全。"""
        result = await self._request("textDocument/completion", {
            "textDocument": {"uri": f"file://{file_path}"},
            "position": {"line": line, "character": col},
        })
        items = result.get("items", result if isinstance(result, list) else [])
        return [
            item.get("insertText", item.get("label", ""))
            for item in items
        ]

    async def shutdown(self) -> None:
        """关闭 LSP 连接。"""
        if self._initialized:
            try:
                await self._request("shutdown", {})
                await self._notify("exit", {})
            except Exception:
                pass

        if self._reader:
            self._reader.cancel()
            try:
                await asyncio.wait_for(self._reader, timeout=2)
            except (asyncio.TimeoutError, asyncio.CancelledError):
                pass

        if self._process:
            self._process.terminate()
            try:
                await asyncio.wait_for(self._process.wait(), timeout=5)
            except asyncio.TimeoutError:
                self._process.kill()

        for future in self._pending.values():
            if not future.done():
                future.cancel()
        self._pending.clear()
        logger.info("LSP shutdown: %s", self.config.language)


# ── LSP 工具包装 ──────────────────────────────────────────


class LSPGoToDefinitionTool(BaseTool):
    name = "lsp_go_to_definition"
    description = "Jump to the definition of a symbol at a given file position."
    permission_level = PermissionLevel.READ

    def __init__(self, client: LSPClient) -> None:
        super().__init__()
        self._client = client

    async def execute(self, input_data) -> ToolResult:
        try:
            locations = await self._client.go_to_definition(
                input_data.path, input_data.line, input_data.col
            )
            if not locations:
                return ToolResult(tool_call_id="", output="No definition found.")
            lines = [f"{loc.uri}:{loc.range.get('start', {}).get('line', 0)}" for loc in locations]
            return ToolResult(tool_call_id="", output="\n".join(lines))
        except Exception as exc:
            return ToolResult(tool_call_id="", output=f"LSP error: {exc}", is_error=True)


class LSPFindReferencesTool(BaseTool):
    name = "lsp_find_references"
    description = "Find all references to a symbol at a given file position."
    permission_level = PermissionLevel.READ

    def __init__(self, client: LSPClient) -> None:
        super().__init__()
        self._client = client

    async def execute(self, input_data) -> ToolResult:
        try:
            locations = await self._client.find_references(
                input_data.path, input_data.line, input_data.col
            )
            if not locations:
                return ToolResult(tool_call_id="", output="No references found.")
            lines = [f"{loc.uri}:{loc.range.get('start', {}).get('line', 0)}" for loc in locations]
            return ToolResult(tool_call_id="", output="\n".join(lines))
        except Exception as exc:
            return ToolResult(tool_call_id="", output=f"LSP error: {exc}", is_error=True)


class LSPHoverTool(BaseTool):
    name = "lsp_hover"
    description = "Show documentation for a symbol at a given file position."
    permission_level = PermissionLevel.READ

    def __init__(self, client: LSPClient) -> None:
        super().__init__()
        self._client = client

    async def execute(self, input_data) -> ToolResult:
        try:
            doc = await self._client.hover(input_data.path, input_data.line, input_data.col)
            if not doc:
                return ToolResult(tool_call_id="", output="No documentation available.")
            return ToolResult(tool_call_id="", output=doc)
        except Exception as exc:
            return ToolResult(tool_call_id="", output=f"LSP error: {exc}", is_error=True)
