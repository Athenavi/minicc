"""Web 自动化工具集 — Playwright 浏览器控制。

对标影刀的"浏览器控制"能力，AI Native 优势：
AI 可理解页面语义，自动选择合适的选择器。
"""

from __future__ import annotations

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext
from app.tools.web.browser import browser_manager


class BrowserNavigateInput(BaseModel):
    url: str = Field(description="URL to navigate to")
    page_id: str | None = Field(default=None, description="Tab ID (defaults to current tab)")


class BrowserNavigateTool(BaseTool):
    name = "browser_navigate"
    description = "Navigate a browser tab to a URL. Opens a new tab if no browser is running."
    input_schema = BrowserNavigateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.WEB

    async def execute(self, input_data: BrowserNavigateInput, context: ToolUseContext | None = None) -> ToolResult:
        if not browser_manager.launched:
            ok = await browser_manager.launch()
            if not ok:
                return ToolResult(tool_call_id="", output="[browser] Failed to launch browser", is_error=True)
            pid = await browser_manager.new_page(input_data.url)
            return ToolResult(tool_call_id="", output=f"[browser] Launched and navigated to {input_data.url}\nPage ID: {pid}\nTitle: {await browser_manager.get_title(pid)}", metadata={"page_id": pid})

        pid = input_data.page_id or browser_manager.current_page_id
        if not pid or pid not in browser_manager._pages:
            pid = await browser_manager.new_page(input_data.url)
            return ToolResult(tool_call_id="", output=f"[browser] New tab: {input_data.url}\nPage ID: {pid}", metadata={"page_id": pid})

        ok = await browser_manager.navigate(pid, input_data.url)
        if not ok:
            return ToolResult(tool_call_id="", output=f"[browser] Navigation failed: {input_data.url}", is_error=True)
        return ToolResult(
            tool_call_id="",
            output=f"[browser] Navigated to {input_data.url}\nTitle: {await browser_manager.get_title(pid)}",
            metadata={"page_id": pid, "url": input_data.url, "title": await browser_manager.get_title(pid)},
        )


class BrowserClickInput(BaseModel):
    selector: str = Field(description="CSS selector or XPath of the element to click")
    page_id: str | None = None


class BrowserClickTool(BaseTool):
    name = "browser_click"
    description = "Click an element on the page using a CSS selector."
    input_schema = BrowserClickInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.WEB

    async def execute(self, input_data: BrowserClickInput, context: ToolUseContext | None = None) -> ToolResult:
        if not browser_manager.launched:
            return ToolResult(tool_call_id="", output="[browser] No browser running. Use browser_navigate first.", is_error=True)
        pid = input_data.page_id or browser_manager.current_page_id
        if not pid:
            return ToolResult(tool_call_id="", output="[browser] No active page.", is_error=True)
        ok = await browser_manager.click(pid, input_data.selector)
        if not ok:
            return ToolResult(tool_call_id="", output=f"[browser] Click failed: {input_data.selector}", is_error=True)
        return ToolResult(tool_call_id="", output=f"[browser] Clicked: {input_data.selector}")


class BrowserFillInput(BaseModel):
    selector: str = Field(description="CSS selector of the input field")
    text: str = Field(description="Text to enter")
    page_id: str | None = None


class BrowserFillTool(BaseTool):
    name = "browser_fill"
    description = "Fill text into an input field on the page."
    input_schema = BrowserFillInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.WEB

    async def execute(self, input_data: BrowserFillInput, context: ToolUseContext | None = None) -> ToolResult:
        if not browser_manager.launched:
            return ToolResult(tool_call_id="", output="[browser] No browser running.", is_error=True)
        pid = input_data.page_id or browser_manager.current_page_id
        ok = await browser_manager.fill(pid, input_data.selector, input_data.text)
        if not ok:
            return ToolResult(tool_call_id="", output=f"[browser] Fill failed: {input_data.selector}", is_error=True)
        return ToolResult(tool_call_id="", output=f"[browser] Filled '{input_data.text[:50]}' into {input_data.selector}")


class BrowserSelectInput(BaseModel):
    selector: str = Field(description="CSS selector of the select element")
    value: str = Field(description="Value to select")
    page_id: str | None = None


class BrowserSelectTool(BaseTool):
    name = "browser_select"
    description = "Select an option from a dropdown element."
    input_schema = BrowserSelectInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.WEB

    async def execute(self, input_data: BrowserSelectInput, context: ToolUseContext | None = None) -> ToolResult:
        if not browser_manager.launched:
            return ToolResult(tool_call_id="", output="[browser] No browser running.", is_error=True)
        pid = input_data.page_id or browser_manager.current_page_id
        ok = await browser_manager.select_option(pid, input_data.selector, input_data.value)
        if not ok:
            return ToolResult(tool_call_id="", output=f"[browser] Select failed: {input_data.selector}", is_error=True)
        return ToolResult(tool_call_id="", output=f"[browser] Selected '{input_data.value}' from {input_data.selector}")


class BrowserGetHtmlInput(BaseModel):
    page_id: str | None = None
    selector: str | None = Field(default=None, description="CSS selector to scope the HTML to a specific element")


class BrowserGetHtmlTool(BaseTool):
    name = "browser_get_html"
    description = "Get the HTML content of the current page or a specific element."
    input_schema = BrowserGetHtmlInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: BrowserGetHtmlInput, context: ToolUseContext | None = None) -> ToolResult:
        if not browser_manager.launched:
            return ToolResult(tool_call_id="", output="[browser] No browser running.", is_error=True)
        pid = input_data.page_id or browser_manager.current_page_id
        html = await browser_manager.get_html(pid, input_data.selector)
        return ToolResult(tool_call_id="", output=html[:50000])


class BrowserScreenshotInput(BaseModel):
    page_id: str | None = None
    full_page: bool = Field(default=False, description="Capture full page screenshot")


class BrowserScreenshotTool(BaseTool):
    name = "browser_screenshot"
    description = "Take a screenshot of the current page. Returns base64 encoded image."
    input_schema = BrowserScreenshotInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: BrowserScreenshotInput, context: ToolUseContext | None = None) -> ToolResult:
        if not browser_manager.launched:
            return ToolResult(tool_call_id="", output="[browser] No browser running.", is_error=True)
        pid = input_data.page_id or browser_manager.current_page_id
        b64 = await browser_manager.screenshot(pid, input_data.full_page)
        if b64.startswith("Screenshot failed"):
            return ToolResult(tool_call_id="", output=b64, is_error=True)
        return ToolResult(tool_call_id="", output=f"[browser] Screenshot captured ({len(b64) // 1024} KB base64)\nData:{b64[:100]}...")


class BrowserGetInfoInput(BaseModel):
    page_id: str | None = None


class BrowserGetInfoTool(BaseTool):
    name = "browser_get_info"
    description = "Get current browser state: page URL, title, open tabs."
    input_schema = BrowserGetInfoInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: BrowserGetInfoInput, context: ToolUseContext | None = None) -> ToolResult:
        if not browser_manager.launched:
            return ToolResult(tool_call_id="", output="[browser] No browser running.")
        pages = await browser_manager.list_pages()
        lines = [f"Browser status: running ({len(pages)} tab(s))"]
        for p in pages:
            marker = " ◀ CURRENT" if p["current"] else ""
            lines.append(f"  [{p['id']}] {p['title']}{marker}")
            lines.append(f"         {p['url']}")
        return ToolResult(tool_call_id="", output="\n".join(lines))


# 批量注册函数
def register_web_tools(registry) -> None:
    """注册所有 Web 自动化工具。"""
    tools = [
        BrowserNavigateTool, BrowserClickTool, BrowserFillTool,
        BrowserSelectTool, BrowserGetHtmlTool, BrowserScreenshotTool,
        BrowserGetInfoTool,
    ]
    for tool_cls in tools:
        registry.register(tool_cls())
