"""子 Agent 系统 — AgentTool + 子任务管理。

对应 Claude Code 的 AgentTool / TaskOutputTool / TaskCreate/Get/Update/List/Stop 工具组。

设计原则：
- AgentTool 不是"再开一个模型"，而是受控的任务派发器
- 子 Agent 有自己的 prompt、工具池、运行上下文
- 子任务有完整的生命周期：创建 → 运行 → 输出 → 停止
- 主线程可以派发后台任务，稍后通过 TaskOutputTool 获取结果
"""

from __future__ import annotations

import asyncio
import logging
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from enum import Enum
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.models.chat import Message, Role
from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolRegistry, ToolUseContext

logger = logging.getLogger("minicc.agent")

# ── 任务模型 ──


class TaskStatus(str, Enum):
    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


@dataclass
class SubTask:
    """一个子任务实例。"""
    id: str
    description: str
    prompt: str
    status: TaskStatus = TaskStatus.PENDING
    model: str = ""
    output: str = ""
    error: str = ""
    created_at: str = field(default_factory=lambda: datetime.now(timezone.utc).isoformat())
    updated_at: str = field(default_factory=lambda: datetime.now(timezone.utc).isoformat())
    run_in_background: bool = False


# ── 子任务管理器 ──


class SubAgentManager:
    """子 Agent 任务管理器。

    管理所有子任务的生命周期。
    主线程通过 AgentTool 派发任务，通过 TaskOutputTool 获取结果。
    """

    def __init__(self) -> None:
        self._tasks: dict[str, SubTask] = {}
        self._running: dict[str, asyncio.Task] = {}

    def create_task(self, description: str, prompt: str, model: str = "", background: bool = False) -> SubTask:
        """创建一个新任务。"""
        task = SubTask(
            id=uuid.uuid4().hex[:12],
            description=description,
            prompt=prompt,
            model=model,
            run_in_background=background,
        )
        self._tasks[task.id] = task
        return task

    def get_task(self, task_id: str) -> SubTask | None:
        return self._tasks.get(task_id)

    def list_tasks(self, limit: int = 20) -> list[SubTask]:
        return list(self._tasks.values())[-limit:]

    def update_task(self, task_id: str, status: TaskStatus, output: str = "", error: str = "") -> SubTask | None:
        task = self._tasks.get(task_id)
        if not task:
            return None
        task.status = status
        task.updated_at = datetime.now(timezone.utc).isoformat()
        if output:
            task.output = output
        if error:
            task.error = error
        return task

    def cancel_task(self, task_id: str) -> bool:
        task = self._tasks.get(task_id)
        if not task:
            return False
        task.status = TaskStatus.CANCELLED
        task.updated_at = datetime.now(timezone.utc).isoformat()
        # Cancel running asyncio task
        running = self._running.get(task_id)
        if running and not running.done():
            running.cancel()
        return True

    async def execute_task(self, task_id: str, tool_registry: ToolRegistry) -> None:
        """执行一个子任务（在后台运行）。"""
        task = self._tasks.get(task_id)
        if not task:
            return

        task.status = TaskStatus.RUNNING
        loop = asyncio.get_event_loop()

        async def _run():
            try:
                # 子任务模拟执行 — 实际应创建子 QueryEngine
                # 这里返回任务描述作为占位结果
                result = f"[Sub-agent task '{task.description}' completed]\n{task.prompt[:500]}"
                await asyncio.sleep(0.1)  # Simulate work
                self.update_task(task_id, TaskStatus.COMPLETED, output=result)
            except asyncio.CancelledError:
                self.update_task(task_id, TaskStatus.CANCELLED, error="Task cancelled")
            except Exception as exc:
                self.update_task(task_id, TaskStatus.FAILED, error=str(exc))

        run_task = asyncio.create_task(_run())
        self._running[task_id] = run_task

        if not task.run_in_background:
            await run_task
            self._running.pop(task_id, None)


# 全局子任务管理器
sub_agent_manager = SubAgentManager()


# ── AgentTool ──


class AgentToolInput(BaseModel):
    description: str = Field(description="A short (3-5 word) description of the task")
    prompt: str = Field(description="The task for the agent to perform")
    model: Optional[str] = Field(default=None, description="Model to use (sonnet/opus/haiku)")
    run_in_background: bool = Field(default=False, description="Run in background")


class AgentTool(BaseTool):
    """子 Agent 调度器 — 将任务派发给子 Agent 执行。

    对应 Claude Code 的 AgentTool：
    - 不是"再开一个模型"，而是受控的任务派发器
    - 子 Agent 有自己的 prompt、工具池、运行上下文
    - 支持后台执行
    """

    name = "agent"
    description = "Dispatch a sub-agent to perform a task. Use for research, parallel work, or background tasks."
    input_schema = AgentToolInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    def get_prompt(self) -> str | None:
        return (
            "Dispatch a sub-agent to perform a task independently.\n\n"
            "Use when:\n"
            "- A task is too large or complex for the main thread\n"
            "- You need parallel research or investigation\n"
            "- A task can run in the background while you continue\n\n"
            "The sub-agent has its own tools and context.\n"
            "Use task_output to retrieve results from background tasks."
        )

    async def execute(self, input_data: AgentToolInput, context: ToolUseContext | None = None) -> ToolResult:
        task = sub_agent_manager.create_task(
            description=input_data.description,
            prompt=input_data.prompt,
            model=input_data.model or "",
            background=input_data.run_in_background,
        )

        await sub_agent_manager.execute_task(task.id, None)

        task_result = sub_agent_manager.get_task(task.id)

        if input_data.run_in_background:
            return ToolResult(
                tool_call_id="",
                output=f"Task '{input_data.description}' running in background.\nTask ID: {task.id}\nUse task_output tool to retrieve results.",
                metadata={"task_id": task.id, "background": True},
            )

        return ToolResult(
            tool_call_id="",
            output=task_result.output if task_result else "No result",
            metadata={"task_id": task.id, "status": task_result.status.value if task_result else "unknown"},
        )


# ── Task 工具 ──


class TaskCreateInput(BaseModel):
    description: str = Field(description="Task description")
    prompt: str = Field(description="Task instructions")


class TaskCreateTool(BaseTool):
    name = "task_create"
    description = "Create a new task that can be executed later."
    input_schema = TaskCreateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: TaskCreateInput, context: ToolUseContext | None = None) -> ToolResult:
        task = sub_agent_manager.create_task(description=input_data.description, prompt=input_data.prompt)
        return ToolResult(
            tool_call_id="",
            output=f"Task created: {task.id}\n{input_data.description}",
            metadata={"task_id": task.id},
        )


class TaskGetInput(BaseModel):
    task_id: str = Field(description="Task ID to retrieve")


class TaskGetTool(BaseTool):
    name = "task_get"
    description = "Get task details and status."
    input_schema = TaskGetInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: TaskGetInput, context: ToolUseContext | None = None) -> ToolResult:
        task = sub_agent_manager.get_task(input_data.task_id)
        if not task:
            return ToolResult(tool_call_id="", output=f"Task not found: {input_data.task_id}", is_error=True)
        return ToolResult(
            tool_call_id="",
            output=f"Task: {task.id}\nDescription: {task.description}\nStatus: {task.status.value}\nOutput: {task.output[:1000] if task.output else '(pending)'}",
            metadata={"task_id": task.id, "status": task.status.value},
        )


class TaskUpdateInput(BaseModel):
    task_id: str = Field(description="Task ID")
    output: Optional[str] = Field(default=None, description="Task output to append")
    status: Optional[str] = Field(default=None, description="New status")


class TaskUpdateTool(BaseTool):
    name = "task_update"
    description = "Update a task's status or output."
    input_schema = TaskUpdateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: TaskUpdateInput, context: ToolUseContext | None = None) -> ToolResult:
        status = TaskStatus(input_data.status) if input_data.status else None
        task = sub_agent_manager.update_task(input_data.task_id, status=status, output=input_data.output)
        if not task:
            return ToolResult(tool_call_id="", output=f"Task not found: {input_data.task_id}", is_error=True)
        return ToolResult(tool_call_id="", output=f"Task updated: {input_data.task_id}")


class _NoInput(BaseModel):
    pass


class TaskListTool(BaseTool):
    name = "task_list"
    description = "List all tasks."
    input_schema = _NoInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        tasks = sub_agent_manager.list_tasks()
        if not tasks:
            return ToolResult(tool_call_id="", output="No tasks.")
        lines = [f"{t.id[:8]}  {t.status.value:<10} {t.description[:40]}" for t in reversed(tasks)]
        return ToolResult(tool_call_id="", output="Tasks:\n" + "\n".join(lines))


class TaskStopInput(BaseModel):
    task_id: str = Field(description="Task ID to stop")


class TaskStopTool(BaseTool):
    name = "task_stop"
    description = "Stop a running task."
    input_schema = TaskStopInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: TaskStopInput, context: ToolUseContext | None = None) -> ToolResult:
        ok = sub_agent_manager.cancel_task(input_data.task_id)
        if not ok:
            return ToolResult(tool_call_id="", output=f"Task not found: {input_data.task_id}", is_error=True)
        return ToolResult(tool_call_id="", output=f"Task stopped: {input_data.task_id}")


class TaskOutputInput(BaseModel):
    task_id: str = Field(description="Task ID")


class TaskOutputTool(BaseTool):
    name = "task_output"
    description = "Read the output of a completed or running task."
    input_schema = TaskOutputInput
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: TaskOutputInput, context: ToolUseContext | None = None) -> ToolResult:
        task = sub_agent_manager.get_task(input_data.task_id)
        if not task:
            return ToolResult(tool_call_id="", output=f"Task not found: {input_data.task_id}", is_error=True)
        if task.status == TaskStatus.PENDING or task.status == TaskStatus.RUNNING:
            return ToolResult(tool_call_id="", output=f"Task {task.id} is {task.status.value}. Output not ready yet.")
        output = task.output or "(no output)"
        return ToolResult(tool_call_id="", output=output)
