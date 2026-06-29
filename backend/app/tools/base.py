"""BaseTool 抽象基类与 ToolRegistry 注册中心。

所有内置工具和扩展工具（MCP/插件）都通过 ToolRegistry 统一注册。
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import Any

from pydantic import BaseModel

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult


class BaseTool(ABC):
    """所有工具的抽象基类。

    强制要求：
    - name / description / input_schema 类变量
    - execute() 返回 ToolResult
    - permission_level 定义权限等级
    """

    name: str = ""
    description: str = ""
    input_schema: type[BaseModel] = BaseModel
    permission_level: PermissionLevel = PermissionLevel.READ

    def get_prompt(self) -> str | None:
        """返回工具级 prompt（指导模型如何正确使用此工具）。

        对应 Claude Code 工具自带的 prompt 设计：
        - 不只是在 schema 里写 description
        - 而是用自然语言告诉模型"什么时候用、怎么用、不要怎么用"
        """
        return None

    @abstractmethod
    async def execute(self, input_data: BaseModel) -> ToolResult:
        """执行工具调用。"""
        ...

    def to_anthropic_tool(self) -> dict:
        """序列化为 Anthropic tool 格式。"""
        return {
            "name": self.name,
            "description": self.description,
            "input_schema": self.input_schema.model_json_schema(),
        }

    def to_openai_tool(self) -> dict:
        """序列化为 OpenAI tool 格式。"""
        return {
            "type": "function",
            "function": {
                "name": self.name,
                "description": self.description,
                "parameters": self.input_schema.model_json_schema(),
            },
        }


class ToolRegistry:
    """工具注册中心。支持注册、查找、序列化。"""

    def __init__(self):
        self._tools: dict[str, BaseTool] = {}

    def register(self, tool: BaseTool) -> None:
        if not tool.name:
            raise ValueError(f"Tool must have a name: {type(tool).__name__}")
        self._tools[tool.name] = tool

    def get(self, name: str) -> BaseTool | None:
        return self._tools.get(name)

    def list_tools(self) -> list[BaseTool]:
        return list(self._tools.values())

    def to_anthropic_tools(self) -> list[dict]:
        return [t.to_anthropic_tool() for t in self._tools.values()]

    def to_openai_tools(self) -> list[dict]:
        return [t.to_openai_tool() for t in self._tools.values()]
