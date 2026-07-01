"""AI 批量编辑与 Undo/Redo — Multi-cursor + 操作历史。"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext
from app.tools.editor_sync import _safe, WORKSPACE

_HISTORY_DB = WORKSPACE / ".minicc" / "edit_history.json"


class EditAction(BaseModel):
    """一次编辑操作记录。"""
    id: str
    path: str
    action_type: str  # replace | insert | delete
    original_content: str
    new_content: str
    timestamp: str
    agent_id: str = "ai"


class BatchEditInput(BaseModel):
    path: str = Field(description="File path")
    edits: list[dict] = Field(description="List of edits: [{range: {start_line, end_line}, text: str}]")


class UndoInput(BaseModel):
    path: str = Field(description="File path")
    steps: int = Field(default=1, description="Number of edits to undo")


class MultiCursorEditTool(BaseTool):
    name = "multi_cursor_edit"
    description = "Apply multiple edits to the same file simultaneously. Use for batch refactoring."
    input_schema = BatchEditInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.FILE

    async def execute(self, input_data: BatchEditInput, context: ToolUseContext | None = None) -> ToolResult:
        safe = _safe(input_data.path)
        if not safe.exists():
            return ToolResult(tool_call_id="", output=f"[editor] File not found: {input_data.path}", is_error=True)

        content = safe.read_text(encoding="utf-8", errors="replace")
        lines = content.splitlines()

        # Sort edits by line descending (bottom-up to preserve line numbers)
        sorted_edits = sorted(input_data.edits, key=lambda e: e.get("range", {}).get("start_line", 0), reverse=True)

        results = []
        for edit in sorted_edits:
            r = edit.get("range", {})
            sl = max(0, r.get("start_line", 1) - 1)
            el = min(len(lines), r.get("end_line", sl + 1))
            original = "\n".join(lines[sl:el])
            lines[sl:el] = [edit.get("text", "")]
            results.append({"range": r, "original_len": len(original), "new_len": len(edit.get("text", ""))})

        new_content = "\n".join(lines)
        tmp = safe.with_suffix(safe.suffix + ".tmp")
        tmp.write_text(new_content, encoding="utf-8")
        tmp.rename(safe)

        total_edits = len(sorted_edits)
        total_changed = sum(r.get("original_len", 0) for r in results)
        return ToolResult(
            tool_call_id="",
            output=f"[editor] Multi-cursor: {total_edits} edits applied to {input_data.path} ({total_changed} chars changed)",
            metadata={"edits": total_edits, "path": input_data.path},
        )


class EditorUndoTool(BaseTool):
    name = "editor_undo"
    description = "Undo the last AI edit operation(s)."
    input_schema = UndoInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.FILE

    async def execute(self, input_data: UndoInput, context: ToolUseContext | None = None) -> ToolResult:
        history = _load_history()
        edits = [h for h in history if h["path"] == input_data.path]

        if not edits:
            return ToolResult(tool_call_id="", output=f"[editor] No edit history for: {input_data.path}", is_error=True)

        undone = 0
        for i in range(min(input_data.steps, len(edits))):
            edit = edits[-(i + 1)]
            safe = _safe(edit["path"])
            if safe.exists():
                safe.write_text(edit["original_content"], encoding="utf-8")
            undone += 1
            history.remove(edit)

        _save_history(history)

        return ToolResult(
            tool_call_id="",
            output=f"[editor] Undone {undone} edit(s) on {input_data.path}",
            metadata={"undone": undone, "path": input_data.path},
        )


def _load_history() -> list[dict]:
    try:
        if _HISTORY_DB.exists():
            return json.loads(_HISTORY_DB.read_text(encoding="utf-8"))
    except Exception:
        pass
    return []


def _save_history(history: list[dict]) -> None:
    _HISTORY_DB.parent.mkdir(parents=True, exist_ok=True)
    _HISTORY_DB.write_text(json.dumps(history[-100:], indent=2), encoding="utf-8")


def _record_edit(path: str, action_type: str, original: str, new: str) -> None:
    import uuid, datetime
    history = _load_history()
    history.append({
        "id": uuid.uuid4().hex[:12],
        "path": path,
        "action_type": action_type,
        "original_content": original,
        "new_content": new,
        "timestamp": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "agent_id": "ai",
    })
    _save_history(history)


def register_batch_tools(registry) -> None:
    registry.register(MultiCursorEditTool())
    registry.register(EditorUndoTool())
