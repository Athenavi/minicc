"""Browser RPA tools 注册到本地工具注册表。

实现对标 Go `internal/tools/rpa_browser.go` 注册的 11 个浏览器工具。
默认使用内存 StubHub；生产环境可注入真实 Hub（Chrome Extension WebSocket 等）。
"""
from __future__ import annotations

import uuid
from dataclasses import dataclass, field
from typing import Any, Protocol

from app.tools.registry import registry


# ── Hub 协议 ──────────────────────────────────────────────────
class BrowserHub(Protocol):
    def connected_client_ids(self) -> list[str]: ...
    def exec_command(self, client_id: str, method: str, params: dict[str, Any]) -> Any: ...


@dataclass
class StubHub:
    """默认内存 Hub，用于测试与占位。"""
    _clients: dict[str, dict[str, Any]] = field(default_factory=lambda: {"stub-client": {"url": "about:blank"}})

    def connected_client_ids(self) -> list[str]:
        return list(self._clients.keys())

    def exec_command(self, client_id: str, method: str, params: dict[str, Any]) -> Any:
        return {"status": "ok", "method": method, "client_id": client_id, "params": params, "note": "stub hub"}


_hub: BrowserHub = StubHub()


def bind_hub(hub: BrowserHub) -> None:
    global _hub
    _hub = hub


def _resolve_client(tab_id: int | None = None) -> str:
    ids = _hub.connected_client_ids()
    if not ids:
        raise RuntimeError("no connected browser clients")
    return ids[0]


def _exec(method: str, params: dict[str, Any], tab_id: int | None = None) -> dict[str, Any]:
    client_id = _resolve_client(tab_id)
    if tab_id and tab_id > 0:
        params = {**params, "tabId": tab_id}
    result = _hub.exec_command(client_id, method, params)
    return result if isinstance(result, dict) else {"result": result}


# ── 工具实现 ──────────────────────────────────────────────────
async def browser_navigate(url: str, tab_id: int | None = None) -> dict[str, Any]:
    if not url:
        return {"error": "url is required"}
    return _exec("browser_navigate", {"url": url}, tab_id)


async def browser_click(selector: str, tab_id: int | None = None) -> dict[str, Any]:
    if not selector:
        return {"error": "selector is required"}
    return _exec("browser_click", {"selector": selector}, tab_id)


async def browser_type(selector: str, text: str, tab_id: int | None = None) -> dict[str, Any]:
    if not selector or not text:
        return {"error": "selector and text are required"}
    return _exec("browser_type", {"selector": selector, "text": text}, tab_id)


async def browser_read(selector: str, tab_id: int | None = None) -> dict[str, Any]:
    if not selector:
        return {"error": "selector is required"}
    return _exec("browser_read", {"selector": selector}, tab_id)


async def browser_screenshot(tab_id: int | None = None, full_page: bool = False) -> dict[str, Any]:
    return _exec("browser_screenshot", {"fullPage": full_page}, tab_id)


async def browser_scroll(direction: str = "down", amount: int = 500, tab_id: int | None = None) -> dict[str, Any]:
    return _exec("browser_scroll", {"direction": direction, "amount": amount}, tab_id)


async def browser_get_state(tab_id: int | None = None) -> dict[str, Any]:
    return _exec("browser_get_state", {}, tab_id)


async def browser_tab_list() -> dict[str, Any]:
    return _exec("browser_tab_list", {})


async def browser_tab_create(url: str = "", tab_id: int | None = None) -> dict[str, Any]:
    params: dict[str, Any] = {}
    if url:
        params["url"] = url
    return _exec("browser_tab_create", params)


async def browser_tab_switch(tab_id: int) -> dict[str, Any]:
    if not tab_id or tab_id <= 0:
        return {"error": "tabId is required"}
    return _exec("browser_tab_switch", {"tabId": tab_id})


async def browser_tab_close(tab_id: int) -> dict[str, Any]:
    if not tab_id or tab_id <= 0:
        return {"error": "tabId is required"}
    return _exec("browser_tab_close", {"tabId": tab_id})


# ── 注册 ──────────────────────────────────────────────────────
_BROWSER_TOOLS = [
    ("browser_navigate", "Navigate to a URL", {"url": {"type": "string"}, "tabId": {"type": "integer"}}, ["url"], browser_navigate),
    ("browser_click", "Click an element by selector", {"selector": {"type": "string"}, "tabId": {"type": "integer"}}, ["selector"], browser_click),
    ("browser_type", "Type text into an element", {"selector": {"type": "string"}, "text": {"type": "string"}, "tabId": {"type": "integer"}}, ["selector", "text"], browser_type),
    ("browser_read", "Read element text/attributes", {"selector": {"type": "string"}, "tabId": {"type": "integer"}}, ["selector"], browser_read),
    ("browser_screenshot", "Take a page screenshot", {"tabId": {"type": "integer"}, "fullPage": {"type": "boolean", "default": False}}, [], browser_screenshot),
    ("browser_scroll", "Scroll the page", {"direction": {"type": "string", "default": "down"}, "amount": {"type": "integer", "default": 500}, "tabId": {"type": "integer"}}, [], browser_scroll),
    ("browser_get_state", "Get current page state", {"tabId": {"type": "integer"}}, [], browser_get_state),
    ("browser_tab_list", "List open tabs", {}, [], browser_tab_list),
    ("browser_tab_create", "Create a new tab", {"url": {"type": "string"}}, [], browser_tab_create),
    ("browser_tab_switch", "Switch to a tab", {"tabId": {"type": "integer"}}, ["tabId"], browser_tab_switch),
    ("browser_tab_close", "Close a tab", {"tabId": {"type": "integer"}}, ["tabId"], browser_tab_close),
]

for name, desc, props, required, handler in _BROWSER_TOOLS:
    registry.register(
        name=name,
        description=desc,
        parameters={"type": "object", "properties": props, "required": required},
        handler=handler,
    )
