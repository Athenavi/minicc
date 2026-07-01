"""Cookie 与会话管理 — 浏览器状态持久化。"""

from __future__ import annotations

from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext
from app.tools.web.browser import browser_manager


class WebCookieGetInput(BaseModel):
    page_id: Optional[str] = None


class WebCookieGetTool(BaseTool):
    name = "web_cookie_get"
    description = "Get all cookies for the current page."
    input_schema = WebCookieGetInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: WebCookieGetInput, context: ToolUseContext | None = None) -> ToolResult:
        if not browser_manager._context:
            return ToolResult(tool_call_id="", output="[cookie] No browser context.", is_error=True)
        cookies = await browser_manager._context.cookies()
        if not cookies:
            return ToolResult(tool_call_id="", output="[cookie] No cookies found.")
        lines = [f"[cookie] {len(cookies)} cookie(s):"]
        for c in cookies:
            lines.append(f"  {c.get('name', '')} = {c.get('value', '')[:50]}")
        return ToolResult(tool_call_id="", output="\n".join(lines), metadata={"cookies": cookies})


class WebCookieSetInput(BaseModel):
    name: str = Field(description="Cookie name")
    value: str = Field(description="Cookie value")
    url: Optional[str] = Field(default=None, description="Cookie domain URL")
    page_id: Optional[str] = None


class WebCookieSetTool(BaseTool):
    name = "web_cookie_set"
    description = "Set a cookie for the current page or a specific domain."
    input_schema = WebCookieSetInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.WEB

    async def execute(self, input_data: WebCookieSetInput, context: ToolUseContext | None = None) -> ToolResult:
        if not browser_manager._context:
            return ToolResult(tool_call_id="", output="[cookie] No browser context.", is_error=True)
        pid = input_data.page_id or browser_manager.current_page_id
        url = input_data.url or (await browser_manager.get_url(pid) if pid else None)
        if not url:
            return ToolResult(tool_call_id="", output="[cookie] No URL to set cookie for.", is_error=True)
        await browser_manager._context.add_cookies([{
            "name": input_data.name,
            "value": input_data.value,
            "url": url,
        }])
        return ToolResult(tool_call_id="", output=f"[cookie] Set: {input_data.name}={input_data.value[:50]}")


class WebLocalStorageGetTool(BaseTool):
    name = "web_local_storage_get"
    description = "Get localStorage data from the current page."
    input_schema = WebCookieGetInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: WebCookieGetInput, context: ToolUseContext | None = None) -> ToolResult:
        pid = input_data.page_id or browser_manager.current_page_id
        if not pid:
            return ToolResult(tool_call_id="", output="[storage] No active page.", is_error=True)
        page = browser_manager._pages.get(pid)
        if not page:
            return ToolResult(tool_call_id="", output="[storage] Page not found.", is_error=True)
        try:
            items = await page.evaluate("JSON.stringify(window.localStorage)")
            import json
            data = json.loads(items)
            lines = [f"[storage] {len(data)} localStorage item(s):"]
            for k, v in list(data.items())[:20]:
                lines.append(f"  {k} = {str(v)[:80]}")
            return ToolResult(tool_call_id="", output="\n".join(lines))
        except Exception as exc:
            return ToolResult(tool_call_id="", output=f"[storage] Error: {exc}", is_error=True)


def register_session_tools(registry) -> None:
    registry.register(WebCookieGetTool())
    registry.register(WebCookieSetTool())
    registry.register(WebLocalStorageGetTool())
