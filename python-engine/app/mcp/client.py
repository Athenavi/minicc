"""MCP Client — connects to MCP servers over stdio, discovers and calls tools.

Mirrors Go internal/mcp/client.go with multi-server support.
"""
from __future__ import annotations

import asyncio
import json
import logging
from dataclasses import dataclass, field
from typing import Any, Optional

logger = logging.getLogger(__name__)


@dataclass
class ServerDef:
    """MCP server configuration."""
    name: str
    command: str
    args: list[str] = field(default_factory=list)
    env: dict[str, str] = field(default_factory=dict)


@dataclass
class MCPTool:
    """A tool provided by an MCP server."""
    name: str  # Namespaced: {server_name}_{tool_name}
    description: str
    input_schema: dict[str, Any]
    server_name: str
    local_name: str  # Original tool name on the server


class ServerConnection:
    """Connection to a single MCP server process."""

    def __init__(self, proc: asyncio.subprocess.Process, name: str):
        self.proc = proc
        self.name = name
        self._req_id = 0
        self._lock = asyncio.Lock()

    async def send_jsonrpc(self, method: str, params: Optional[dict] = None) -> dict[str, Any]:
        """Send a JSON-RPC request and read the response."""
        self._req_id += 1
        req = {
            "jsonrpc": "2.0",
            "id": self._req_id,
            "method": method,
            "params": params,
        }
        req_line = json.dumps(req) + "\n"

        async with self._lock:
            self.proc.stdin.write(req_line.encode())
            await self.proc.stdin.drain()

            response_line = await asyncio.wait_for(
                self.proc.stdout.readline(), timeout=30.0
            )
            if not response_line:
                raise ConnectionError(f"No response from MCP server {self.name}")

        resp = json.loads(response_line)
        if "error" in resp and resp["error"]:
            raise RuntimeError(f"MCP error: {resp['error'].get('message', 'unknown')}")
        return resp.get("result", {})

    async def close(self):
        """Kill the server process."""
        if self.proc and self.proc.returncode is None:
            try:
                self.proc.terminate()
                await asyncio.wait_for(self.proc.wait(), timeout=5.0)
            except (asyncio.TimeoutError, ProcessLookupError):
                self.proc.kill()


class MCPClient:
    """Manages connections to multiple MCP servers and their tools."""

    def __init__(self, servers: list[ServerDef]):
        self._servers = servers
        self._conns: dict[str, ServerConnection] = {}
        self._tools: list[MCPTool] = []

    async def start(self):
        """Connect to all configured MCP servers and discover their tools."""
        for server in self._servers:
            try:
                await self._connect_server(server)
            except Exception as e:
                logger.error("MCP connect %s failed: %s", server.name, e)
                raise

    async def _connect_server(self, server: ServerDef):
        """Connect to a single MCP server and discover its tools."""
        env = None
        if server.env:
            import os
            env = {**os.environ, **server.env}

        proc = await asyncio.create_subprocess_exec(
            server.command, *server.args,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.DEVNULL,
            env=env,
        )
        conn = ServerConnection(proc, server.name)
        self._conns[server.name] = conn

        # Initialize
        await conn.send_jsonrpc("initialize", {
            "protocolVersion": "2025-03-26",
            "clientInfo": {"name": "minicc-python", "version": "3.0.0"},
        })

        # List tools
        result = await conn.send_jsonrpc("tools/list", None)
        raw_tools = result.get("tools", [])

        for i, t in enumerate(raw_tools):
            tool = MCPTool(
                name=f"{server.name}_{t.get('name', f'unnamed_{i}')}",
                description=t.get("description", ""),
                input_schema=t.get("inputSchema", {}),
                server_name=server.name,
                local_name=t.get("name", f"unnamed_{i}"),
            )
            self._tools.append(tool)
            logger.info("MCP tool discovered: %s (%s)", tool.name, server.name)

        logger.info("MCP server %s connected: %d tools", server.name, len(raw_tools))

    @property
    def tools(self) -> list[MCPTool]:
        return self._tools

    async def call_tool(self, tool_name: str, arguments: dict[str, Any]) -> dict[str, Any]:
        """Call a tool on the appropriate MCP server."""
        for server in self._servers:
            prefix = f"{server.name}_"
            if tool_name.startswith(prefix):
                local_name = tool_name[len(prefix):]
                conn = self._conns.get(server.name)
                if not conn:
                    return {"error": f"MCP server {server.name} not connected"}
                result = await conn.send_jsonrpc("tools/call", {
                    "name": local_name,
                    "arguments": arguments,
                })
                return result
        return {"error": f"Tool {tool_name} not found on any MCP server"}

    async def close(self):
        """Shut down all MCP server connections."""
        for name, conn in self._conns.items():
            try:
                await conn.close()
            except Exception as e:
                logger.warning("Error closing MCP server %s: %s", name, e)
        self._conns.clear()


async def load_mcp_config(config_path: str) -> list[ServerDef]:
    """Load MCP server definitions from a JSON config file."""
    import os
    from pathlib import Path

    p = Path(config_path)
    if not p.exists():
        return []

    data = json.loads(p.read_text(encoding="utf-8"))
    servers = []
    for s in data.get("mcp_servers", []):
        servers.append(ServerDef(
            name=s["name"],
            command=s["command"],
            args=s.get("args", []),
            env=s.get("env", {}),
        ))
    return servers
