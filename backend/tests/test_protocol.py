"""测试：BaseTool / ToolRegistry / FastAPI 入口。"""

from __future__ import annotations

from pydantic import BaseModel

import pytest

from app.main import app
from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolRegistry


# -- 测试用工具 --

class EchoInput(BaseModel):
    msg: str


class EchoTool(BaseTool):
    name = "echo"
    description = "回声测试工具"
    input_schema = EchoInput
    permission_level = PermissionLevel.READ

    async def execute(self, input_data: EchoInput) -> ToolResult:
        return ToolResult(tool_call_id="test", output=input_data.msg)


class TestBaseTool:
    @pytest.mark.asyncio
    async def test_execute_interface(self):
        tool = EchoTool()
        result = await tool.execute(EchoInput(msg="hello"))
        assert result.output == "hello"
        assert not result.is_error

    def test_to_anthropic_tool(self):
        tool = EchoTool()
        schema = tool.to_anthropic_tool()
        assert schema["name"] == "echo"
        assert "input_schema" in schema
        assert schema["input_schema"]["type"] == "object"

    def test_to_openai_tool(self):
        tool = EchoTool()
        schema = tool.to_openai_tool()
        assert schema["type"] == "function"
        assert schema["function"]["name"] == "echo"


class TestToolRegistry:
    def test_register_and_get(self):
        registry = ToolRegistry()
        tool = EchoTool()
        registry.register(tool)
        assert registry.get("echo") is tool

    def test_get_unknown(self):
        registry = ToolRegistry()
        assert registry.get("nope") is None

    def test_list_tools(self):
        registry = ToolRegistry()
        registry.register(EchoTool())
        tools = registry.list_tools()
        assert len(tools) == 1
        assert tools[0].name == "echo"

    def test_register_empty_name_raises(self):
        registry = ToolRegistry()
        class EmptyNameTool(BaseTool):
            name = ""
            async def execute(self, input_data):
                return ToolResult(tool_call_id="t", output="")
        with pytest.raises(ValueError, match="must have a name"):
            registry.register(EmptyNameTool())

    def test_register_tool_without_name_raises(self):
        registry = ToolRegistry()
        class NoNameTool(BaseTool):
            name = ""
            async def execute(self, input_data):
                return ToolResult(tool_call_id="t", output="")
        with pytest.raises(ValueError, match="must have a name"):
            registry.register(NoNameTool())


class TestHealthEndpoint:
    def test_health(self, client):
        resp = client.get("/health")
        assert resp.status_code == 200
        assert resp.json() == {"status": "ok"}

    def test_list_tools(self, client):
        resp = client.get("/api/tools")
        assert resp.status_code == 200
        data = resp.json()
        assert "tools" in data
        assert "count" in data
