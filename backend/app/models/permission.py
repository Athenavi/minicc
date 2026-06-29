"""权限与审批 Pydantic 模型。"""

from __future__ import annotations

import enum
from typing import Any, Literal, Optional

from pydantic import BaseModel, ConfigDict, Field


class PermissionLevel(str, enum.Enum):
    """权限等级：数值越高越严格。"""
    READ = "read"  # 自动允许
    WRITE = "write"  # 需用户审批
    EXECUTE = "execute"  # 需严格审批（醒目提示）

    def __ge__(self, other: "PermissionLevel") -> bool:
        order = ["read", "write", "execute"]
        return order.index(self.value) >= order.index(other.value)


class PermissionRequest(BaseModel):
    """一次待审批的权限请求。"""
    model_config = ConfigDict(frozen=True)

    id: str
    tool_name: str
    tool_input: dict[str, Any]
    level: PermissionLevel
    reason: str = Field(description="AI 解释为什么需要这个操作")
    diff_preview: Optional[str] = Field(default=None, description="WRITE 操作的 diff 预览")
    status: Literal["pending", "approved", "rejected", "always_allowed"] = "pending"
