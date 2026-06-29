"""聊天/消息 Pydantic 模型 — MiniCC 的统一消息契约。"""

from __future__ import annotations

import enum
from datetime import datetime, timezone
from typing import Any, Literal, Optional

from pydantic import BaseModel, ConfigDict, Field


class Role(str, enum.Enum):
    """消息角色：谁发送了这条消息。"""
    system = "system"
    user = "user"
    assistant = "assistant"
    tool = "tool"


class ContentBlock(BaseModel):
    """消息内容块 — 支持文本、图片、文件、工具调用等多种类型。

    对应 Anthropic Messages API 的 content block 结构。
    """
    model_config = ConfigDict(frozen=True)

    type: Literal["text", "image", "file", "tool_use", "tool_result"]
    text: Optional[str] = None
    source: Optional[dict[str, Any]] = None
    id: Optional[str] = None  # tool_use id
    name: Optional[str] = None  # tool name
    input: Optional[dict[str, Any]] = None  # tool_use input
    tool_use_id: Optional[str] = None  # tool_result 引用
    content: Optional[list[ContentBlock]] = None  # tool_result 嵌套


class Message(BaseModel):
    """单条消息。"""
    model_config = ConfigDict(frozen=True)

    role: Role
    content: str | list[ContentBlock]
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    model: Optional[str] = None
