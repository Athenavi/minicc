"""工具调用 Pydantic 模型。"""

from __future__ import annotations

from typing import Any, Literal, Optional

from pydantic import BaseModel, ConfigDict, Field


class ToolCall(BaseModel):
    """一次工具调用请求。"""
    model_config = ConfigDict(frozen=True)

    id: str = Field(description="唯一 ID")
    type: Literal["function", "bash", "file_read", "file_write", "file_edit", "search", "web_fetch", "lsp", "mcp"]
    name: str = Field(description="工具名")
    input: dict[str, Any] = Field(default_factory=dict, description="工具输入参数")
    status: Literal["pending", "approved", "rejected", "running", "completed", "failed"] = "pending"


class ToolResult(BaseModel):
    """一次工具调用的执行结果。"""
    model_config = ConfigDict(frozen=True)

    tool_call_id: str
    output: str
    is_error: bool = False
    metadata: dict[str, Any] = Field(default_factory=dict)
