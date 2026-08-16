"""工具执行上下文 — contextvars 传播当前运行的 agent 上下文。

deepseek-harness 的工具经 exec context（ToolExecution）携带调用方信息；
minicc 的 registry.execute 只有 (name, params)，工具无法感知 session/网关。
此处用 contextvars 在 AgentRuntime.run() 内设置，工具通过 get_* 读取，
在 async 环境中自动沿任务传播（无需改 registry 签名）。
"""
from __future__ import annotations

import contextvars
from typing import Any, Optional

_current_context: contextvars.ContextVar[dict[str, Any]] = contextvars.ContextVar(
    "minicc_tool_context", default={}
)


def set_tool_context(**kwargs: Any) -> None:
    """在 agent 循环内设置当前上下文（session_id/user_id/tenant_id/gateway）。"""
    merged = dict(_current_context.get())
    merged.update(kwargs)
    _current_context.set(merged)


def get_tool_context(key: str, default: Any = None) -> Any:
    return _current_context.get().get(key, default)


def get_session_id() -> str:
    return str(get_tool_context("session_id", ""))


def get_user_id() -> str:
    return str(get_tool_context("user_id", ""))


def get_tenant_id() -> str:
    return str(get_tool_context("tenant_id", ""))


def get_gateway():
    """当前运行的 GatewayRouter 引用（子 agent 委派需要）。"""
    return get_tool_context("gateway", None)


def get_all() -> dict[str, Any]:
    """完整快照当前上下文（子 agent 委派前保存、完成后恢复）。"""
    return dict(_current_context.get())


def restore_context(snapshot: dict[str, Any]) -> None:
    """恢复上下文快照（子 agent 执行会改写 context，父任务需要还原）。"""
    _current_context.set(snapshot)
