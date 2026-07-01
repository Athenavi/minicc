"""窗口管理工具。"""

from __future__ import annotations

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class _NoInput(BaseModel):
    pass


class DesktopListWindowsTool(BaseTool):
    name = "desktop_list_windows"
    description = "List all visible windows on the desktop."
    input_schema = _NoInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SHELL

    async def execute(self, input_data: _NoInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            import pygetwindow as gw
        except ImportError:
            return ToolResult(tool_call_id="", output="[window] pygetwindow not available.", is_error=True)
        wins = gw.getWindowsWithTitle("")
        visible = [w for w in wins if w.visible]
        lines = [f"[window] {len(visible)} visible window(s):"]
        for w in visible[:30]:
            lines.append(f"  {w.title[:80]} — ({w.left},{w.top}) {w.width}×{w.height}")
        return ToolResult(tool_call_id="", output="\n".join(lines))


class DesktopActivateWindowInput(BaseModel):
    title: str = Field(description="Window title (substring match)")


class DesktopActivateWindowTool(BaseTool):
    name = "desktop_activate_window"
    description = "Bring a window to the foreground by title."
    input_schema = DesktopActivateWindowInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.SHELL

    async def execute(self, input_data: DesktopActivateWindowInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            import pygetwindow as gw
        except ImportError:
            return ToolResult(tool_call_id="", output="[window] pygetwindow not available.", is_error=True)
        wins = gw.getWindowsWithTitle(input_data.title)
        if not wins:
            return ToolResult(tool_call_id="", output=f"[window] No window found: {input_data.title}", is_error=True)
        wins[0].activate()
        return ToolResult(tool_call_id="", output=f"[window] Activated: {wins[0].title}")


class DesktopResizeWindowInput(BaseModel):
    title: str = Field(description="Window title")
    width: int = Field(ge=100, le=5000)
    height: int = Field(ge=100, le=5000)


class DesktopResizeWindowTool(BaseTool):
    name = "desktop_resize_window"
    description = "Resize and reposition a window."
    input_schema = DesktopResizeWindowInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.SHELL

    async def execute(self, input_data: DesktopResizeWindowInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            import pygetwindow as gw
        except ImportError:
            return ToolResult(tool_call_id="", output="[window] pygetwindow not available.", is_error=True)
        wins = gw.getWindowsWithTitle(input_data.title)
        if not wins:
            return ToolResult(tool_call_id="", output=f"[window] No window found: {input_data.title}", is_error=True)
        wins[0].resizeTo(input_data.width, input_data.height)
        return ToolResult(tool_call_id="", output=f"[window] Resized: {wins[0].title} to {input_data.width}×{input_data.height}")


def register_window_tools(registry) -> None:
    registry.register(DesktopListWindowsTool())
    registry.register(DesktopActivateWindowTool())
    registry.register(DesktopResizeWindowTool())
