"""AI 原生增强 — 自适应定位 + Self-healing 机制。"""

from __future__ import annotations

from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class _Empty(BaseModel):
    pass


class AiAnalyzePageTool(BaseTool):
    """分析页面结构，推荐选择器策略。"""

    name = "ai_analyze_page"
    description = "Analyze the current browser page structure and suggest element selectors."
    input_schema = _Empty
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: _Empty, context: ToolUseContext | None = None) -> ToolResult:
        from app.tools.web.browser import browser_manager
        pid = browser_manager.current_page_id
        if not pid:
            return ToolResult(tool_call_id="", output="[ai] No active page.", is_error=True)
        html = await browser_manager.get_html(pid)
        from bs4 import BeautifulSoup
        soup = BeautifulSoup(html, "html.parser")

        # Extract interactive elements
        interactives = []
        for tag in soup.find_all(["a", "button", "input", "select", "textarea", "form"]):
            tid = tag.get("id", "")
            cls = tag.get("class", [])
            text = tag.get_text(strip=True)[:50]
            attrs = {}
            if tid:
                attrs["id"] = tid
            if cls:
                attrs["class"] = " ".join(cls[:3])
            if tag.name == "a":
                attrs["href"] = tag.get("href", "")[:80]
            interactives.append({
                "tag": tag.name,
                "text": text,
                **attrs,
            })

        output_lines = [
            f"[ai] Page analysis: {len(interactives)} interactive elements\n",
            "Interactive elements:",
        ]
        for el in interactives[:30]:
            line = f"  <{el['tag']}>"
            if el.get("id"):
                line += f" #{el['id']}"
            if el.get("text"):
                line += f" '{el['text']}'"
            output_lines.append(line)

        return ToolResult(tool_call_id="", output="\n".join(output_lines), metadata={"elements": interactives})


class AiSmartLocateInput(BaseModel):
    target: str = Field(description="Description of the element to find (e.g. 'login button', 'search box')")
    page_id: Optional[str] = None


class AiSmartLocateTool(BaseTool):
    """AI 驱动的元素定位 — 通过语义描述找到页面元素。"""

    name = "ai_smart_locate"
    description = "Find an element on the page by its semantic description (e.g. 'the login button')."
    input_schema = AiSmartLocateInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: AiSmartLocateInput, context: ToolUseContext | None = None) -> ToolResult:
        from app.tools.web.browser import browser_manager
        pid = input_data.page_id or browser_manager.current_page_id
        if not pid:
            return ToolResult(tool_call_id="", output="[ai] No active page.", is_error=True)
        html = await browser_manager.get_html(pid)
        from bs4 import BeautifulSoup
        soup = BeautifulSoup(html, "html.parser")
        target_lower = input_data.target.lower()

        candidates = []
        for tag in soup.find_all(["a", "button", "input", "textarea", "select", "div", "span"]):
            text = tag.get_text(strip=True).lower()
            tid = (tag.get("id", "") or "").lower()
            name = (tag.get("name", "") or "").lower()
            placeholder = (tag.get("placeholder", "") or "").lower()
            aria = (tag.get("aria-label", "") or "").lower()
            cls = " ".join(tag.get("class", [])).lower()

            score = 0
            if target_lower in text: score += 3
            if target_lower in tid: score += 3
            if target_lower in name: score += 2
            if target_lower in placeholder: score += 2
            if target_lower in aria: score += 2
            if target_lower in cls: score += 1

            if score > 0:
                selector = f"#{tag['id']}" if tag.get("id") else (
                    f"{tag.name}[name='{tag['name']}']" if tag.get("name") else (
                        f"{tag.name}:has-text('{tag.get_text(strip=True)[:20]}')"))
                candidates.append((score, selector, tag.name, tag.get_text(strip=True)[:50]))

        if not candidates:
            return ToolResult(tool_call_id="", output=f"[ai] No element found matching '{input_data.target}'", is_error=True)

        candidates.sort(key=lambda x: -x[0])
        output_lines = [f"[ai] Found {len(candidates)} candidate(s) for '{input_data.target}':"]
        for score, sel, tag, text in candidates[:10]:
            output_lines.append(f"  [{score}] {sel}  <{tag}> '{text}'")

        return ToolResult(tool_call_id="", output="\n".join(output_lines))


def register_ai_tools(registry) -> None:
    registry.register(AiAnalyzePageTool())
    registry.register(AiSmartLocateTool())
