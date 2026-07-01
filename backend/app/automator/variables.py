"""变量管理工具 — 工作流变量的设置/获取/列表。"""

from __future__ import annotations

from typing import Any, Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

# 全局变量存储
_variables: dict[str, Any] = {}


class VarSetInput(BaseModel):
    name: str = Field(description="Variable name")
    value: Any = Field(description="Variable value")
    scope: str = Field(default="global", description="Scope: global | session")


class VarSetTool(BaseTool):
    name = "var_set"
    description = "Set a variable that can be used in workflows."
    input_schema = VarSetInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.SESSION

    async def execute(self, input_data: VarSetInput, context: ToolUseContext | None = None) -> ToolResult:
        key = f"{input_data.scope}:{input_data.name}"
        _variables[key] = input_data.value
        return ToolResult(tool_call_id="", output=f"[var] Set {input_data.name} = {str(input_data.value)[:100]}")


class VarGetInput(BaseModel):
    name: str = Field(description="Variable name")
    scope: str = Field(default="global", description="Scope: global | session")


class VarGetTool(BaseTool):
    name = "var_get"
    description = "Get the value of a variable."
    input_schema = VarGetInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SESSION

    async def execute(self, input_data: VarGetInput, context: ToolUseContext | None = None) -> ToolResult:
        key = f"{input_data.scope}:{input_data.name}"
        val = _variables.get(key)
        if val is None:
            return ToolResult(tool_call_id="", output=f"[var] {input_data.name} is not set")
        return ToolResult(tool_call_id="", output=f"[var] {input_data.name} = {str(val)[:5000]}")


class _Empty(BaseModel):
    pass


class VarListTool(BaseTool):
    name = "var_list"
    description = "List all variables."
    input_schema = _Empty
    permission_level = PermissionLevel.READ
    category = ToolCategory.SESSION

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        if not _variables:
            return ToolResult(tool_call_id="", output="[var] No variables set.")
        lines = ["[var] Variables:"]
        for k, v in _variables.items():
            lines.append(f"  {k} = {str(v)[:80]}")
        return ToolResult(tool_call_id="", output="\n".join(lines))


def register_var_tools(registry) -> None:
    registry.register(VarSetTool())
    registry.register(VarGetTool())
    registry.register(VarListTool())
