"""桌面 GUI 自动化 — 鼠标键盘模拟 + OCR + 窗口管理 + 剪贴板。"""

from __future__ import annotations

import logging
from typing import Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

logger = logging.getLogger("minicc.desktop")


class DesktopClickInput(BaseModel):
    x: int = Field(description="X coordinate")
    y: int = Field(description="Y coordinate")
    button: str = Field(default="left", description="Mouse button: left/right/middle")
    clicks: int = Field(default=1, ge=1, le=3)


class DesktopClickTool(BaseTool):
    name = "desktop_click"
    description = "Click at specified screen coordinates."
    input_schema = DesktopClickInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.SHELL

    async def execute(self, input_data: DesktopClickInput, context: ToolUseContext | None = None) -> ToolResult:
        import pyautogui
        pyautogui.click(input_data.x, input_data.y, button=input_data.button, clicks=input_data.clicks)
        return ToolResult(tool_call_id="", output=f"[desktop] Clicked ({input_data.x}, {input_data.y})")


class DesktopTypeInput(BaseModel):
    text: str = Field(description="Text to type")
    interval: float = Field(default=0.05, ge=0, le=1, description="Interval between keystrokes (seconds)")


class DesktopTypeTool(BaseTool):
    name = "desktop_type"
    description = "Type text at the current cursor position."
    input_schema = DesktopTypeInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.SHELL

    async def execute(self, input_data: DesktopTypeInput, context: ToolUseContext | None = None) -> ToolResult:
        import pyautogui
        pyautogui.typewrite(input_data.text, interval=input_data.interval)
        return ToolResult(tool_call_id="", output=f"[desktop] Typed {len(input_data.text)} characters")


class DesktopHotkeyInput(BaseModel):
    keys: list[str] = Field(description="Keys to press (e.g. ['ctrl', 'c'] for copy)")


class DesktopHotkeyTool(BaseTool):
    name = "desktop_hotkey"
    description = "Press a keyboard shortcut (e.g. ctrl+c, alt+tab)."
    input_schema = DesktopHotkeyInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.SHELL

    async def execute(self, input_data: DesktopHotkeyInput, context: ToolUseContext | None = None) -> ToolResult:
        import pyautogui
        pyautogui.hotkey(*input_data.keys)
        return ToolResult(tool_call_id="", output=f"[desktop] Hotkey: {'+'.join(input_data.keys)}")


class _NoInput(BaseModel):
    pass


class DesktopScreenshotTool(BaseTool):
    name = "desktop_screenshot"
    description = "Take a screenshot of the entire screen or a region."
    input_schema = _NoInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SHELL

    async def execute(self, input_data: _NoInput, context: ToolUseContext | None = None) -> ToolResult:
        import pyautogui, base64
        img = pyautogui.screenshot()
        import io
        buf = io.BytesIO()
        img.save(buf, format="PNG")
        b64 = base64.b64encode(buf.getvalue()).decode()
        return ToolResult(tool_call_id="", output=f"[desktop] Screenshot captured ({len(b64)//1024} KB)\nData:{b64[:100]}...")


class DesktopGetPositionTool(BaseTool):
    name = "desktop_get_position"
    description = "Get the current mouse cursor position."
    input_schema = _NoInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SHELL

    async def execute(self, input_data: _NoInput, context: ToolUseContext | None = None) -> ToolResult:
        import pyautogui
        x, y = pyautogui.position()
        return ToolResult(tool_call_id="", output=f"[desktop] Mouse position: ({x}, {y})", metadata={"x": x, "y": y})


class DesktopDragInput(BaseModel):
    start_x: int
    start_y: int
    end_x: int
    end_y: int
    duration: float = Field(default=0.5, ge=0.1, le=5)


class DesktopDragTool(BaseTool):
    name = "desktop_drag"
    description = "Drag the mouse from one position to another."
    input_schema = DesktopDragInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.SHELL

    async def execute(self, input_data: DesktopDragInput, context: ToolUseContext | None = None) -> ToolResult:
        import pyautogui
        pyautogui.moveTo(input_data.start_x, input_data.start_y)
        pyautogui.drag(input_data.end_x - input_data.start_x, input_data.end_y - input_data.start_y, duration=input_data.duration)
        return ToolResult(tool_call_id="", output=f"[desktop] Dragged ({input_data.start_x},{input_data.start_y}) → ({input_data.end_x},{input_data.end_y})")


def register_mouse_keyboard_tools(registry) -> None:
    for t in [DesktopClickTool, DesktopTypeTool, DesktopHotkeyTool,
              DesktopScreenshotTool, DesktopGetPositionTool, DesktopDragTool]:
        registry.register(t())
