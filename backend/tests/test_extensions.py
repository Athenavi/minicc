"""Tests: MCP client, LSP client, PluginLoader, ExtensionLoader."""

from __future__ import annotations

from pathlib import Path
from tempfile import TemporaryDirectory

import pytest

from app.core.mcp_client import (
    MCPClient,
    MCPClientSession,
    MCPServerConfig,
    MCPToolAdapter,
    MCPToolDefinition,
    ListMcpResourcesTool,
    ReadMcpResourceTool,
)
from app.core.lsp_client import LSPClient, LSPConfig, Location
from app.core.plugin_loader import PluginLoader
from app.tools.base import BaseTool, ToolRegistry
from app.tools.file_system import ReadFileTool

pytestmark = pytest.mark.asyncio


# ── MCP Data Models ──


class TestMCPDataModels:
    def test_tool_definition(self):
        t = MCPToolDefinition(name="test", description="a tool", inputSchema={"type": "object"})
        assert t.name == "test"
        assert t.inputSchema["type"] == "object"


# ── MCPToolAdapter ──


class TestMCPToolAdapter:
    def test_adapter_name_format(self):
        """Adapter name follows mcp_{server}_{tool} pattern."""
        from unittest.mock import MagicMock
        client = MagicMock(spec=MCPClient)
        client.name = "filesystem"
        client.config = MCPServerConfig(command="test")

        t = MCPToolAdapter(client, MCPToolDefinition(
            name="read", description="read file"
        ))
        assert t.name == "mcp_filesystem_read"
        assert "read file" in t.description

    def test_adapter_permission_level(self):
        """MCP tools default to WRITE permission."""
        from unittest.mock import MagicMock
        client = MagicMock(spec=MCPClient)
        client.name = "test"
        adapter = MCPToolAdapter(client, MCPToolDefinition(name="x", description=""))
        from app.models.permission import PermissionLevel
        assert adapter.permission_level == PermissionLevel.WRITE


# ── ListMcpResourcesTool ──


class TestListMcpResourcesTool:
    async def test_no_clients(self):
        tool = ListMcpResourcesTool({})
        from pydantic import BaseModel
        class Empty(BaseModel):
            pass
        result = await tool.execute(Empty())
        assert "No MCP resources" in result.output


class TestReadMcpResourceTool:
    async def test_unknown_server(self):
        tool = ReadMcpResourceTool({})
        from pydantic import BaseModel
        class Input(BaseModel):
            uri: str = ""
            server: str = "nonexistent"
        result = await tool.execute(Input())
        assert result.is_error
        assert "not found" in result.output


# ── PluginLoader ──


class TestPluginLoader:
    async def test_load_from_empty_dir(self):
        with TemporaryDirectory() as tmp:
            loader = PluginLoader([Path(tmp)])
            registry = ToolRegistry()
            count = await loader.load_plugins(registry)
            assert count == 0

    async def test_load_single_plugin(self):
        with TemporaryDirectory() as tmp:
            plugin_file = Path(tmp) / "my_tool.py"
            plugin_file.write_text("""
from app.tools.base import BaseTool
from app.models.tool import ToolResult
from pydantic import BaseModel

class MyInput(BaseModel):
    msg: str = ""

class MyTool(BaseTool):
    name = "my_tool"
    description = "A test plugin tool"
    async def execute(self, input_data):
        return ToolResult(tool_call_id="", output="plugin executed")
""")
            loader = PluginLoader([Path(tmp)])
            registry = ToolRegistry()
            count = await loader.load_plugins(registry)
            assert count == 1
            tool = registry.get("my_tool")
            assert tool is not None
            assert "plugin" in tool.description

    async def test_skip_private_files(self):
        with TemporaryDirectory() as tmp:
            (Path(tmp) / "__init__.py").write_text("")
            loader = PluginLoader([Path(tmp)])
            count = await loader.load_plugins(ToolRegistry())
            assert count == 0

    async def test_invalid_plugin_skipped(self):
        with TemporaryDirectory() as tmp:
            (Path(tmp) / "bad.py").write_text("this is not valid python ===")
            loader = PluginLoader([Path(tmp)])
            count = await loader.load_plugins(ToolRegistry())
            assert count == 0
