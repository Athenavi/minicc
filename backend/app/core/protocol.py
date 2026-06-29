"""统一协议层 — 消息类型枚举与 WebSocket 消息协议。"""

from __future__ import annotations

import enum


class MessageType(str, enum.Enum):
    """WebSocket 消息类型枚举。

    前后端所有通信消息均以 type 字段标识。
    """

    # 客户端 → 服务端 (C2S)
    USER_MESSAGE = "user_message"
    APPROVAL_ACTION = "approval_action"
    CANCEL = "cancel"
    PING = "ping"
    SESSION_RESUME = "session_resume"

    # 服务端 → 客户端 (S2C)
    MESSAGE_CHUNK = "message_chunk"
    MESSAGE_COMPLETE = "message_complete"
    TOOL_CALL_START = "tool_call_start"
    TOOL_CALL_RESULT = "tool_call_result"
    PERMISSION_REQUIRED = "permission_required"
    PERMISSION_RESULT = "permission_result"
    STATUS_UPDATE = "status_update"
    ERROR = "error"
    SESSION_INFO = "session_info"
    PONG = "pong"
