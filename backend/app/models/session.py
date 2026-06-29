"""会话状态 Pydantic 模型。"""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Optional

from pydantic import BaseModel, ConfigDict, Field

from .chat import Message
from .tool import ToolCall


class SessionState(BaseModel):
    """会话的完整运行时状态。

    对应 Claude Code QueryEngine 持有的跨轮次状态。
    """
    model_config = ConfigDict(frozen=True)

    session_id: str = Field(description="UUID")
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    messages: list[Message] = Field(default_factory=list)
    pending_tool_calls: list[ToolCall] = Field(default_factory=list)
    approved_tool_calls: list[ToolCall] = Field(default_factory=list)
    metadata: dict[str, Any] = Field(default_factory=dict)
