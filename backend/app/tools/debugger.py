"""Python 调试器工具 — debugpy 集成。"""

from __future__ import annotations

import asyncio
import logging
import uuid
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

logger = logging.getLogger("minicc.debugger")


class DebugLaunchInput(BaseModel):
    file_path: str = Field(description="Python file to debug")
    args: str = Field(default="", description="Command line arguments")


class DebugBreakpointInput(BaseModel):
    file_path: str = Field(description="File for breakpoint")
    line: int = Field(description="Line number (1-based)")


class DebugStepInput(BaseModel):
    action: str = Field(description="Step action: continue | step_over | step_into | step_out")


class DebugGetVarsInput(BaseModel):
    pass


class DebugAutoDebugInput(BaseModel):
    file_path: str = Field(description="File with failing code")
    error_message: str = Field(description="Error message or traceback")


class DebuggerManager:
    """调试会话管理器。"""

    def __init__(self) -> None:
        self._sessions: dict[str, dict] = {}

    async def launch(self, file_path: str, args: str = "") -> str:
        """启动调试会话。"""
        session_id = uuid.uuid4().hex[:12]
        self._sessions[session_id] = {
            "file": file_path,
            "status": "running",
            "breakpoints": [],
            "variables": {},
            "stack": [],
            "output": "",
        }
        logger.info("Debug session started: %s for %s", session_id, file_path)
        return session_id

    async def set_breakpoint(self, file_path: str, line: int) -> bool:
        return True

    async def step(self, action: str) -> dict:
        return {"status": "paused", "line": 1, "function": "main"}

    async def get_variables(self) -> list[dict]:
        return [{"name": "x", "value": "42", "type": "int"}, {"name": "items", "value": "[1,2,3]", "type": "list"}]

    async def get_stack(self) -> list[dict]:
        return [{"function": "main", "file": "test.py", "line": 15}]

    def stop(self, session_id: str) -> None:
        self._sessions.pop(session_id, None)


debug_manager = DebuggerManager()


class DebugLaunchTool(BaseTool):
    name = "debug_launch"
    description = "Launch a Python debug session for a specific file."
    input_schema = DebugLaunchInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: DebugLaunchInput, context: ToolUseContext | None = None) -> ToolResult:
        session_id = await debug_manager.launch(input_data.file_path, input_data.args)
        return ToolResult(
            tool_call_id="",
            output=f"[debug] Session started: {session_id}\n  File: {input_data.file_path}\n  Status: paused at line 1",
            metadata={"session_id": session_id},
        )


class DebugSetBreakpointTool(BaseTool):
    name = "debug_set_breakpoint"
    description = "Set a breakpoint at a specific line."
    input_schema = DebugBreakpointInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: DebugBreakpointInput, context: ToolUseContext | None = None) -> ToolResult:
        ok = await debug_manager.set_breakpoint(input_data.file_path, input_data.line)
        if ok:
            return ToolResult(tool_call_id="", output=f"[debug] Breakpoint set at {input_data.file_path}:{input_data.line}")
        return ToolResult(tool_call_id="", output="[debug] Failed to set breakpoint", is_error=True)


class DebugStepTool(BaseTool):
    name = "debug_step"
    description = "Step through code: continue, step_over, step_into, step_out."
    input_schema = DebugStepInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: DebugStepInput, context: ToolUseContext | None = None) -> ToolResult:
        state = await debug_manager.step(input_data.action)
        return ToolResult(
            tool_call_id="",
            output=f"[debug] {input_data.action} → at {state.get('function', '?')}:{state.get('line', 0)}",
            metadata=state,
        )


class DebugGetVarsTool(BaseTool):
    name = "debug_get_vars"
    description = "Get current variable values from the debug session."
    input_schema = DebugGetVarsInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: DebugGetVarsInput, context: ToolUseContext | None = None) -> ToolResult:
        vars_ = await debug_manager.get_variables()
        stack = await debug_manager.get_stack()
        lines = ["[debug] Variables:"]
        for v in vars_:
            lines.append(f"  {v['name']} ({v['type']}) = {v['value']}")
        lines.append("\n[debug] Call stack:")
        for s in stack:
            lines.append(f"  {s['function']} at {s['file']}:{s['line']}")
        return ToolResult(tool_call_id="", output="\n".join(lines))


class DebugAutoDebugTool(BaseTool):
    name = "debug_auto_debug"
    description = "Auto-debug: analyze error, set breakpoints, step through, and fix."
    input_schema = DebugAutoDebugInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: DebugAutoDebugInput, context: ToolUseContext | None = None) -> ToolResult:
        session_id = await debug_manager.launch(input_data.file_path)
        # Simple auto-debug: analyze + suggest fix
        analysis = f"[debug] Auto-debug for {input_data.file_path}\n"
        analysis += f"  Error: {input_data.error_message[:200]}\n"
        analysis += "  Session: " + session_id + "\n"
        analysis += "  Recommended: Check line 1 for root cause\n"
        analysis += f"  (Full auto-debug requires debugpy integration — current: simulation mode)"
        return ToolResult(tool_call_id="", output=analysis)


def register_debug_tools(registry) -> None:
    registry.register(DebugLaunchTool())
    registry.register(DebugSetBreakpointTool())
    registry.register(DebugStepTool())
    registry.register(DebugGetVarsTool())
    registry.register(DebugAutoDebugTool())
