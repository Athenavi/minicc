"""本地工具注册表（Python 端）。

职责：
- 提供统一的工具注册/查找接口
- 支持将工具导出为 OpenAI function-calling schema
- 支持直接执行本地工具（替代 Go /v1/tools/execute）
"""
from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Any, Callable, Awaitable


@dataclass
class ToolDef:
    name: str
    description: str
    parameters: dict
    handler: Callable[..., Awaitable[Any]]


class ToolRegistry:
    def __init__(self) -> None:
        self._tools: dict[str, ToolDef] = {}

    def register(self, name: str, description: str, parameters: dict, handler: Callable[..., Awaitable[Any]]) -> None:
        self._tools[name] = ToolDef(name=name, description=description, parameters=parameters, handler=handler)

    def get(self, name: str) -> ToolDef | None:
        return self._tools.get(name)

    def list_names(self) -> list[str]:
        return list(self._tools.keys())

    def to_openai_tools(self) -> list[dict]:
        converted: list[dict] = []
        for tool in self._tools.values():
            converted.append({
                "type": "function",
                "function": {
                    "name": tool.name,
                    "description": tool.description,
                    "parameters": tool.parameters,
                },
            })
        return converted

    async def execute(self, name: str, params: dict[str, Any]) -> dict[str, Any]:
        tool = self._tools.get(name)
        if not tool:
            return {"error": f"Tool '{name}' not found"}
        try:
            result = await tool.handler(**params)
            if isinstance(result, dict):
                return result
            return {"output": result}
        except Exception as e:
            return {"error": str(e)}


# 全局注册表（进程内单例）
registry = ToolRegistry()
