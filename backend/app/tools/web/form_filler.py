"""表单自动填写工具 — 智能表单识别与填充。"""

from __future__ import annotations

from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext
from app.tools.web.browser import browser_manager


class FormDetectInput(BaseModel):
    page_id: Optional[str] = None


class WebFormDetectTool(BaseTool):
    """检测当前页面中的表单字段并返回其信息。"""

    name = "web_form_detect"
    description = "Detect all form fields on the current page and return their names, types, and selectors."
    input_schema = FormDetectInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.WEB

    async def execute(self, input_data: FormDetectInput, context: ToolUseContext | None = None) -> ToolResult:
        pid = input_data.page_id or browser_manager.current_page_id
        if not pid:
            return ToolResult(tool_call_id="", output="[form] No active page.", is_error=True)

        html = await browser_manager.get_html(pid)
        from bs4 import BeautifulSoup
        soup = BeautifulSoup(html, "html.parser")

        fields = []
        for tag in soup.find_all(["input", "select", "textarea", "button"]):
            ftype = tag.get("type", "")
            name = tag.get("name", "") or tag.get("id", "") or ""
            field_id = tag.get("id", "")
            placeholder = tag.get("placeholder", "")
            label = ""
            # 查找关联的 label
            if field_id:
                lbl = soup.find("label", attrs={"for": field_id})
                if lbl:
                    label = lbl.get_text(strip=True)
            if not label and name:
                lbl2 = soup.find("label", string=lambda t: t and name in t)
                if lbl2:
                    label = lbl2.get_text(strip=True)

            fields.append({
                "tag": tag.name,
                "type": ftype,
                "name": name,
                "id": field_id,
                "label": label or name,
                "placeholder": placeholder,
                "required": tag.get("required") is not None,
                "selector": f"#{field_id}" if field_id else f"input[name='{name}']" if name else "",
            })

        if not fields:
            return ToolResult(tool_call_id="", output="[form] No form fields detected.")

        output = f"[form] Detected {len(fields)} field(s):\n"
        for f in fields:
            req = " *" if f["required"] else ""
            output += f"  [{f['tag']}] {f['label']}{req} — {f['selector']}\n"
        return ToolResult(tool_call_id="", output=output, metadata={"fields": fields})


class FormFillInput(BaseModel):
    fields: dict[str, str] = Field(description="Map of field identifiers (name/id/label) to values")
    page_id: Optional[str] = None
    submit_selector: Optional[str] = Field(default=None, description="Submit button selector to click after filling")


class WebFormFillTool(BaseTool):
    """自动填写表单字段。支持通过 name、id、label 定位字段。"""

    name = "web_form_fill"
    description = "Fill multiple form fields at once. Map field names/IDs to values."
    input_schema = FormFillInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.WEB

    async def execute(self, input_data: FormFillInput, context: ToolUseContext | None = None) -> ToolResult:
        pid = input_data.page_id or browser_manager.current_page_id
        if not pid:
            return ToolResult(tool_call_id="", output="[form] No active page.", is_error=True)

        results = []
        for field_id, value in input_data.fields.items():
            # 尝试多种选择器策略
            selectors = [
                f"#{field_id}",
                f"[name='{field_id}']",
                f"[placeholder='{field_id}']",
                f"label:has-text('{field_id}') + input",
                f"label:has-text('{field_id}') + textarea",
                f"label:has-text('{field_id}') ~ select",
            ]
            filled = False
            for sel in selectors:
                ok = await browser_manager.fill(pid, sel, value)
                if ok:
                    results.append(f"  ✅ {field_id} → {sel}")
                    filled = True
                    break
            if not filled:
                results.append(f"  ❌ {field_id} — no matching field found")

        # Submit if requested
        if input_data.submit_selector:
            ok = await browser_manager.click(pid, input_data.submit_selector)
            results.append(f"  {'✅' if ok else '❌'} Submit: {input_data.submit_selector}")

        output = f"[form] Fill results ({len(input_data.fields)} field(s)):\n" + "\n".join(results)
        return ToolResult(tool_call_id="", output=output)


def register_form_tools(registry) -> None:
    registry.register(WebFormDetectTool())
    registry.register(WebFormFillTool())
