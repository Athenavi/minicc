"""宸ュ叿鎵ц涓婁笅鏂?鈥?contextvars 浼犳挱褰撳墠杩愯鐨?agent 涓婁笅鏂囥€?
deepseek-harness 鐨勫伐鍏风粡 exec context锛圱oolExecution锛夋惡甯﹁皟鐢ㄦ柟淇℃伅锛?chiron 鐨?registry.execute 鍙湁 (name, params)锛屽伐鍏锋棤娉曟劅鐭?session/缃戝叧銆?姝ゅ鐢?contextvars 鍦?AgentRuntime.run() 鍐呰缃紝宸ュ叿閫氳繃 get_* 璇诲彇锛?鍦?async 鐜涓嚜鍔ㄦ部浠诲姟浼犳挱锛堟棤闇€鏀?registry 绛惧悕锛夈€?"""
from __future__ import annotations

import contextvars
from typing import Any, Optional

_current_context: contextvars.ContextVar[dict[str, Any]] = contextvars.ContextVar(
    "chiron_tool_context", default={}
)


def set_tool_context(**kwargs: Any) -> None:
    """鍦?agent 寰幆鍐呰缃綋鍓嶄笂涓嬫枃锛坰ession_id/user_id/tenant_id/gateway锛夈€?""
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
    """褰撳墠杩愯鐨?GatewayRouter 寮曠敤锛堝瓙 agent 濮旀淳闇€瑕侊級銆?""
    return get_tool_context("gateway", None)


def get_all() -> dict[str, Any]:
    """瀹屾暣蹇収褰撳墠涓婁笅鏂囷紙瀛?agent 濮旀淳鍓嶄繚瀛樸€佸畬鎴愬悗鎭㈠锛夈€?""
    return dict(_current_context.get())


def restore_context(snapshot: dict[str, Any]) -> None:
    """鎭㈠涓婁笅鏂囧揩鐓э紙瀛?agent 鎵ц浼氭敼鍐?context锛岀埗浠诲姟闇€瑕佽繕鍘燂級銆?""
    _current_context.set(snapshot)


