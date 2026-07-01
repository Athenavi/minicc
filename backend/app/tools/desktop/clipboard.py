"""剪贴板操作工具。"""

from __future__ import annotations

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class _NoInput(BaseModel):
    pass


class ClipboardGetTool(BaseTool):
    name = "clipboard_get"
    description = "Get text from the system clipboard."
    input_schema = _NoInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.SHELL

    async def execute(self, input_data: _NoInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            import pyperclip
        except ImportError:
            return ToolResult(tool_call_id="", output="[clipboard] pyperclip not installed.", is_error=True)
        text = pyperclip.paste()
        return ToolResult(tool_call_id="", output=f"[clipboard] Got {len(text)} chars:\n{text[:5000]}")


class ClipboardSetInput(BaseModel):
    text: str = Field(description="Text to copy to clipboard")


class ClipboardSetTool(BaseTool):
    name = "clipboard_set"
    description = "Copy text to the system clipboard."
    input_schema = ClipboardSetInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.SHELL

    async def execute(self, input_data: ClipboardSetInput, context: ToolUseContext | None = None) -> ToolResult:
        try:
            import pyperclip
        except ImportError:
            return ToolResult(tool_call_id="", output="[clipboard] pyperclip not installed.", is_error=True)
        pyperclip.copy(input_data.text)
        return ToolResult(tool_call_id="", output=f"[clipboard] Copied {len(input_data.text)} chars")


def register_clipboard_tools(registry) -> None:
    registry.register(ClipboardGetTool())
    registry.register(ClipboardSetTool())
