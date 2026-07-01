"""屏幕 OCR 识别 — Tesseract 文字识别。"""

from __future__ import annotations

from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class _NoInput(BaseModel):
    pass


class DesktopOcrScreenTool(BaseTool):
    """截取屏幕并识别文字。"""

    name = "desktop_ocr_screen"
    description = "Take a screenshot and extract all visible text using OCR."
    input_schema = _NoInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SHELL

    async def execute(self, input_data: _NoInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            import pyautogui, pytesseract
        except ImportError:
            return ToolResult(tool_call_id="", output="[ocr] pytesseract not installed. Run: pip install pytesseract", is_error=True)
        try:
            pytesseract.get_tesseract_version()
        except Exception:
            return ToolResult(tool_call_id="", output="[ocr] Tesseract not found. Install: https://github.com/tesseract-ocr/tesseract", is_error=True)

        img = pyautogui.screenshot()
        text = pytesseract.image_to_string(img, lang="eng+chi_sim")
        return ToolResult(tool_call_id="", output=f"[ocr] Detected text:\n{text.strip()[:10000]}")


class DesktopOcrRegionInput(BaseModel):
    x: int
    y: int
    width: int = Field(ge=10, le=5000)
    height: int = Field(ge=10, le=5000)


class DesktopOcrRegionTool(BaseTool):
    """识别屏幕指定区域的文字。"""

    name = "desktop_ocr_region"
    description = "Extract text from a specific screen region using OCR."
    input_schema = DesktopOcrRegionInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SHELL

    async def execute(self, input_data: DesktopOcrRegionInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            import pyautogui, pytesseract
        except ImportError:
            return ToolResult(tool_call_id="", output="[ocr] pytesseract not installed.", is_error=True)
        img = pyautogui.screenshot(region=(input_data.x, input_data.y, input_data.width, input_data.height))
        text = pytesseract.image_to_string(img, lang="eng+chi_sim")
        detected = text.strip()
        if not detected:
            return ToolResult(tool_call_id="", output=f"[ocr] No text detected in region ({input_data.x},{input_data.y} {input_data.width}×{input_data.height})")
        return ToolResult(tool_call_id="", output=f"[ocr] Region text:\n{detected[:10000]}")


def register_ocr_tools(registry) -> None:
    registry.register(DesktopOcrScreenTool())
    registry.register(DesktopOcrRegionTool())
