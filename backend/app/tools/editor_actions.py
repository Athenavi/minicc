"""AI 编辑器工具 — AI 直接操作编辑器，替代 write_to_file/str_replace_editor。

提供 4 个工具：
- editor_select: 选中指定范围代码
- editor_replace: 替换选中内容
- editor_insert: 在指定位置插入
- editor_stream: 流式写入内容（逐字符展示）
"""

from __future__ import annotations

from typing import Any, Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext
from app.tools.editor_sync import _safe


class Range(BaseModel):
    start_line: int = Field(description="Start line (1-based)")
    start_col: int = Field(default=1, description="Start column")
    end_line: int = Field(description="End line (1-based)")
    end_col: int = Field(default=1, description="End column")


class EditorSelectInput(BaseModel):
    path: str = Field(description="File path")
    range: Range = Field(description="Selection range")


class EditorReplaceInput(BaseModel):
    path: str = Field(description="File path")
    range: Range = Field(description="Range to replace")
    text: str = Field(description="Replacement text")


class EditorInsertInput(BaseModel):
    path: str = Field(description="File path")
    line: int = Field(description="Line to insert after (1-based)")
    text: str = Field(description="Text to insert")


class EditorStreamInput(BaseModel):
    path: str = Field(description="File path")
    position: int = Field(default=0, description="Character position to start writing")
    characters: str = Field(description="Characters to write")
    finalize: bool = Field(default=False, description="Whether this is the final chunk")


class EditorSelectTool(BaseTool):
    name = "editor_select"
    description = "Select a range of code in the editor. Use to highlight code before asking for changes."
    input_schema = EditorSelectInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.FILE

    async def execute(self, input_data: EditorSelectInput, context: ToolUseContext | None = None) -> ToolResult:
        safe = _safe(input_data.path)
        if not safe.exists():
            return ToolResult(tool_call_id="", output=f"[editor] File not found: {input_data.path}", is_error=True)

        content = safe.read_text(encoding="utf-8", errors="replace")
        lines = content.splitlines()
        r = input_data.range
        selected = "\n".join(lines[r.start_line - 1 : r.end_line])

        return ToolResult(
            tool_call_id="",
            output=f"[editor] Selected lines {r.start_line}-{r.end_line} in {input_data.path} ({len(selected)} chars)\n"
                   f"```\n{selected[:2000]}\n```" if selected else "[editor] Empty selection",
            metadata={"path": input_data.path, "selected_lines": f"{r.start_line}-{r.end_line}"},
        )


class EditorReplaceTool(BaseTool):
    name = "editor_replace"
    description = "Replace a range of code in the editor. Use for precise modifications instead of write_to_file."
    input_schema = EditorReplaceInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.FILE

    async def execute(self, input_data: EditorReplaceInput, context: ToolUseContext | None = None) -> ToolResult:
        safe = _safe(input_data.path)
        if not safe.exists():
            return ToolResult(tool_call_id="", output=f"[editor] File not found: {input_data.path}", is_error=True)

        content = safe.read_text(encoding="utf-8", errors="replace")
        lines = content.splitlines()
        r = input_data.range

        if r.start_line < 1 or r.end_line > len(lines):
            return ToolResult(tool_call_id="", output=f"[editor] Range out of bounds (file has {len(lines)} lines)", is_error=True)

        new_lines = lines[: r.start_line - 1] + [input_data.text] + lines[r.end_line:]
        new_content = "\n".join(new_lines)

        tmp = safe.with_suffix(safe.suffix + ".tmp")
        tmp.write_text(new_content, encoding="utf-8")
        tmp.rename(safe)

        return ToolResult(
            tool_call_id="",
            output=f"[editor] Replaced lines {r.start_line}-{r.end_line} in {input_data.path}",
            metadata={"path": input_data.path, "range": r.model_dump()},
        )


class EditorInsertTool(BaseTool):
    name = "editor_insert"
    description = "Insert text at a specific line in the editor."
    input_schema = EditorInsertInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.FILE

    async def execute(self, input_data: EditorInsertInput, context: ToolUseContext | None = None) -> ToolResult:
        safe = _safe(input_data.path)
        if not safe.exists():
            return ToolResult(tool_call_id="", output=f"[editor] File not found: {input_data.path}", is_error=True)

        content = safe.read_text(encoding="utf-8", errors="replace")
        lines = content.splitlines()
        pos = min(input_data.line, len(lines))

        new_lines = lines[:pos] + [input_data.text] + lines[pos:]
        new_content = "\n".join(new_lines)

        tmp = safe.with_suffix(safe.suffix + ".tmp")
        tmp.write_text(new_content, encoding="utf-8")
        tmp.rename(safe)

        return ToolResult(
            tool_call_id="",
            output=f"[editor] Inserted {len(input_data.text)} chars after line {pos} in {input_data.path}",
            metadata={"path": input_data.path, "line": pos},
        )


class EditorStreamTool(BaseTool):
    name = "editor_stream"
    description = "Stream characters into a file. Use for long code generation that should appear character by character."
    input_schema = EditorStreamInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.FILE

    def __init__(self):
        super().__init__()
        self._buffers: dict[str, str] = {}

    async def execute(self, input_data: EditorStreamInput, context: ToolUseContext | None = None) -> ToolResult:
        safe = _safe(input_data.path)

        if input_data.position == 0 and not input_data.finalize:
            # Start new stream — clear buffer
            self._buffers[input_data.path] = input_data.characters
            return ToolResult(
                tool_call_id="",
                output=f"[editor] Streaming {len(input_data.characters)} chars to {input_data.path}...",
                metadata={"streaming": True, "path": input_data.path},
            )
        elif input_data.finalize:
            # Finalize — write to file
            full_content = self._buffers.get(input_data.path, "") + (input_data.characters or "")
            safe.parent.mkdir(parents=True, exist_ok=True)
            tmp = safe.with_suffix(safe.suffix + ".tmp")
            tmp.write_text(full_content, encoding="utf-8")
            tmp.rename(safe)
            self._buffers.pop(input_data.path, None)
            return ToolResult(
                tool_call_id="",
                output=f"[editor] Stream complete: {len(full_content)} chars written to {input_data.path}",
                metadata={"path": input_data.path, "chars": len(full_content)},
            )
        else:
            # Continue streaming
            self._buffers[input_data.path] = self._buffers.get(input_data.path, "") + input_data.characters
            return ToolResult(
                tool_call_id="",
                output=f"[editor] Streaming... ({len(self._buffers[input_data.path])} chars buffered)",
                metadata={"streaming": True, "path": input_data.path},
            )


def register_editor_tools(registry) -> None:
    registry.register(EditorSelectTool())
    registry.register(EditorReplaceTool())
    registry.register(EditorInsertTool())
    registry.register(EditorStreamTool())
