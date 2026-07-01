"""操作录制器 — 录制用户操作并生成 DSL。"""

from __future__ import annotations

import json
from datetime import datetime, timezone
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class RecordedAction(BaseModel):
    """一条录制的操作。"""
    timestamp: str = ""
    tool: str = ""
    params: dict[str, Any] = Field(default_factory=dict)
    description: str = ""


_recording: bool = False
_recorded_actions: list[RecordedAction] = []
_recording_name: str = ""


class RecorderStartInput(BaseModel):
    name: str = Field(description="Recording name")


class RecorderStartTool(BaseTool):
    name = "recorder_start"
    description = "Start recording actions. All subsequent tool calls will be recorded."
    input_schema = RecorderStartInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.SESSION

    async def execute(self, input_data: RecorderStartInput, context: ToolUseContext | None = None) -> ToolResult:
        global _recording, _recorded_actions, _recording_name
        _recording = True
        _recorded_actions = []
        _recording_name = input_data.name
        return ToolResult(tool_call_id="", output=f"[recorder] Started recording: {input_data.name}")


class _Empty(BaseModel):
    pass


class RecorderStopTool(BaseTool):
    name = "recorder_stop"
    description = "Stop recording and generate a workflow DSL from recorded actions."
    input_schema = _Empty
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.SESSION

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        global _recording
        _recording = False
        if not _recorded_actions:
            return ToolResult(tool_call_id="", output="[recorder] No actions recorded.")
        output = json.dumps({
            "name": _recording_name,
            "version": "1.0",
            "steps": [a.model_dump() for a in _recorded_actions],
        }, indent=2, ensure_ascii=False)
        return ToolResult(tool_call_id="", output=f"[recorder] Recording stopped. {len(_recorded_actions)} action(s):\n\n{output}")


def record_action(tool: str, params: dict) -> None:
    """记录一个工具调用（由 QueryEngine 调用）。"""
    global _recording, _recorded_actions
    if _recording:
        _recorded_actions.append(RecordedAction(
            timestamp=datetime.now(timezone.utc).isoformat(),
            tool=tool,
            params=params,
        ))


class RecorderStatusTool(BaseTool):
    name = "recorder_status"
    description = "Check if recording is active and how many actions recorded."
    input_schema = _Empty
    permission_level = PermissionLevel.READ
    category = ToolCategory.SESSION

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        if _recording:
            return ToolResult(tool_call_id="", output=f"[recorder] Recording active: {_recording_name} ({len(_recorded_actions)} actions)")
        return ToolResult(tool_call_id="", output=f"[recorder] Not recording. Last session: {_recording_name} ({len(_recorded_actions)} actions)")


def register_recorder_tools(registry) -> None:
    registry.register(RecorderStartTool())
    registry.register(RecorderStopTool())
    registry.register(RecorderStatusTool())
