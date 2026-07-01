"""任务调度器 — Cron/事件触发的工作流执行。"""

from __future__ import annotations

import asyncio
import logging
from datetime import datetime, timezone
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.automator.dsl import WorkflowDefinition
from app.automator.workflow import WorkflowExecutor
from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

logger = logging.getLogger("minicc.scheduler")


class SchedulerEntry(BaseModel):
    """一个调度任务。"""
    id: str = ""
    name: str
    cron: str = ""
    workflow: dict[str, Any]
    enabled: bool = True
    last_run: Optional[str] = None
    next_run: Optional[str] = None


_scheduled_tasks: dict[str, SchedulerEntry] = {}
_scheduler_loop: asyncio.Task | None = None


class SchedulerCreateInput(BaseModel):
    name: str = Field(description="Schedule name")
    cron: str = Field(description="Cron expression (e.g. '0 9 * * 1-5')")
    workflow: dict = Field(description="Workflow definition dict")


class SchedulerCreateTool(BaseTool):
    name = "scheduler_create"
    description = "Create a scheduled workflow task with cron expression."
    input_schema = SchedulerCreateInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: SchedulerCreateInput, context: ToolUseContext | None = None) -> ToolResult:
        import uuid
        entry = SchedulerEntry(
            id=uuid.uuid4().hex[:12],
            name=input_data.name,
            cron=input_data.cron,
            workflow=input_data.workflow,
        )
        _scheduled_tasks[entry.id] = entry
        return ToolResult(
            tool_call_id="",
            output=f"[scheduler] Created: {entry.name} (cron: {entry.cron})\nID: {entry.id}",
            metadata={"id": entry.id},
        )


class _Empty(BaseModel):
    pass


class SchedulerListTool(BaseTool):
    name = "scheduler_list"
    description = "List all scheduled tasks."
    input_schema = _Empty
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        if not _scheduled_tasks:
            return ToolResult(tool_call_id="", output="[scheduler] No scheduled tasks.")
        lines = ["[scheduler] Scheduled tasks:"]
        for s in _scheduled_tasks.values():
            status = "🟢" if s.enabled else "🔴"
            lines.append(f"  {status} {s.name} — cron: {s.cron} (ID: {s.id})")
        return ToolResult(tool_call_id="", output="\n".join(lines))


class SchedulerDeleteInput(BaseModel):
    task_id: str = Field(description="Task ID to delete")


class SchedulerDeleteTool(BaseTool):
    name = "scheduler_delete"
    description = "Delete a scheduled task."
    input_schema = SchedulerDeleteInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: SchedulerDeleteInput, context: ToolUseContext | None = None) -> ToolResult:
        if input_data.task_id in _scheduled_tasks:
            del _scheduled_tasks[input_data.task_id]
            return ToolResult(tool_call_id="", output=f"[scheduler] Deleted: {input_data.task_id}")
        return ToolResult(tool_call_id="", output=f"[scheduler] Not found: {input_data.task_id}", is_error=True)


def register_scheduler_tools(registry) -> None:
    registry.register(SchedulerCreateTool())
    registry.register(SchedulerListTool())
    registry.register(SchedulerDeleteTool())
