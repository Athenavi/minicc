"""网页数据提取工具 — 表格/列表/JSON/文本提取。"""

from __future__ import annotations

import json
from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext
from app.tools.web.browser import browser_manager


class ExtractTableInput(BaseModel):
    selector: str = Field(description="CSS selector for the table element")
    page_id: Optional[str] = None
    include_headers: bool = True
    max_rows: int = Field(default=100, ge=1, le=10000)


class WebExtractTableTool(BaseTool):
    """提取 HTML 表格为结构化数据（Markdown + JSON）。"""

    name = "web_extract_table"
    description = "Extract structured data from an HTML table. Returns Markdown table + JSON."
    input_schema = ExtractTableInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: ExtractTableInput, context: ToolUseContext | None = None) -> ToolResult:
        pid = input_data.page_id or browser_manager.current_page_id
        if not pid:
            return ToolResult(tool_call_id="", output="[extract] No active page.", is_error=True)

        html = await browser_manager.get_html(pid, input_data.selector)
        if html.startswith("Element not found") or html.startswith("Get HTML failed"):
            return ToolResult(tool_call_id="", output=html, is_error=True)

        # Parse table from HTML
        from bs4 import BeautifulSoup
        soup = BeautifulSoup(html, "html.parser")
        table = soup.find("table")
        if not table:
            return ToolResult(tool_call_id="", output=f"[extract] No table found in selector: {input_data.selector}", is_error=True)

        rows = table.find_all("tr")
        headers = []
        data = []

        for i, row in enumerate(rows):
            if i > input_data.max_rows:
                break
            cells = row.find_all(["th", "td"])
            cell_texts = [cell.get_text(strip=True) for cell in cells]

            if input_data.include_headers and not headers and row.find("th"):
                headers = cell_texts
            else:
                data.append(cell_texts)

        # Build Markdown table
        md_lines = []
        if headers:
            md_lines.append("| " + " | ".join(headers) + " |")
            md_lines.append("| " + " | ".join(["---"] * len(headers)) + " |")
        for row_data in data[:input_data.max_rows]:
            md_lines.append("| " + " | ".join(row_data) + " |")

        result = {
            "rows": len(data),
            "columns": len(headers) if headers else (len(data[0]) if data else 0),
            "markdown": "\n".join(md_lines),
            "json": [dict(zip(headers, row)) if headers else row for row in data],
        }

        output = f"[extract] Table: {result['rows']} rows × {result['columns']} columns\n\n"
        output += result["markdown"][:30000]
        return ToolResult(tool_call_id="", output=output, metadata=result)


class ExtractListInput(BaseModel):
    selector: str = Field(description="CSS selector for the list container")
    item_selector: str = Field(description="CSS selector for list items")
    page_id: Optional[str] = None
    max_items: int = Field(default=50, ge=1, le=1000)


class WebExtractListTool(BaseTool):
    """提取列表数据为结构化 JSON。"""

    name = "web_extract_list"
    description = "Extract list items from a page as structured data."
    input_schema = ExtractListInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: ExtractListInput, context: ToolUseContext | None = None) -> ToolResult:
        pid = input_data.page_id or browser_manager.current_page_id
        if not pid:
            return ToolResult(tool_call_id="", output="[extract] No active page.", is_error=True)

        from bs4 import BeautifulSoup
        html = await browser_manager.get_html(pid, input_data.selector)
        soup = BeautifulSoup(html, "html.parser")

        items = soup.select(input_data.item_selector) if input_data.item_selector else soup.find_all(["li", "option", "a"])
        results = []
        for item in items[:input_data.max_items]:
            text = item.get_text(strip=True)
            href = item.get("href", "")
            results.append({"text": text, "href": href, "tag": item.name})

        output = f"[extract] List: {len(results)} items\n"
        for r in results:
            line = f"  • {r['text'][:100]}"
            if r['href']:
                line += f" ({r['href']})"
            output += line + "\n"

        return ToolResult(tool_call_id="", output=output, metadata={"items": results})


class ExtractTextInput(BaseModel):
    selector: str = Field(description="CSS selector for the target element")
    page_id: Optional[str] = None
    attribute: Optional[str] = Field(default=None, description="Extract attribute instead of text")


class WebExtractTextTool(BaseTool):
    """从页面中提取指定元素的文本或属性。"""

    name = "web_extract_text"
    description = "Extract text content or attributes from page elements."
    input_schema = ExtractTextInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: ExtractTextInput, context: ToolUseContext | None = None) -> ToolResult:
        pid = input_data.page_id or browser_manager.current_page_id
        if not pid:
            return ToolResult(tool_call_id="", output="[extract] No active page.", is_error=True)

        if input_data.attribute:
            from bs4 import BeautifulSoup
            html = await browser_manager.get_html(pid, input_data.selector)
            soup = BeautifulSoup(html, "html.parser")
            els = soup.select(input_data.selector)
            values = [el.get(input_data.attribute, "") for el in els]
            output = "\n".join(values[:100])
            return ToolResult(tool_call_id="", output=output or f"[extract] No elements found: {input_data.selector}")

        text = await browser_manager.get_text(pid, input_data.selector)
        return ToolResult(tool_call_id="", output=text[:50000])


class WebExtractJsonTool(BaseTool):
    """提取页面中的 JSON-LD 结构化数据。"""

    name = "web_extract_json"
    description = "Extract JSON-LD structured data and inline JSON from the page."
    input_schema = ExtractTextInput  # reuse: selector optional
    permission_level = PermissionLevel.READ

    async def execute(self, input_data: ExtractTextInput, context: ToolUseContext | None = None) -> ToolResult:
        pid = input_data.page_id or browser_manager.current_page_id
        if not pid:
            return ToolResult(tool_call_id="", output="[extract] No active page.", is_error=True)

        html = await browser_manager.get_html(pid)
        from bs4 import BeautifulSoup
        soup = BeautifulSoup(html, "html.parser")

        # Extract JSON-LD
        results = []
        for script in soup.find_all("script", type="application/ld+json"):
            try:
                data = json.loads(script.string)
                results.append(json.dumps(data, indent=2, ensure_ascii=False)[:2000])
            except (json.JSONDecodeError, TypeError):
                continue

        if not results:
            return ToolResult(tool_call_id="", output="[extract] No JSON-LD found on page.")

        output = f"[extract] Found {len(results)} JSON-LD block(s)\n\n"
        output += "\n\n---\n\n".join(results)
        return ToolResult(tool_call_id="", output=output)


# 批量注册
def register_scraper_tools(registry) -> None:
    for tool in [WebExtractTableTool, WebExtractListTool, WebExtractTextTool, WebExtractJsonTool]:
        registry.register(tool())
