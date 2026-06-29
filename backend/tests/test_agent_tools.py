"""Tests: AgentTool and Task tools."""

from __future__ import annotations

import pytest

from app.tools.agent_tools import (
    AgentTool,
    AgentToolInput,
    SubTask,
    TaskCreateTool,
    TaskCreateInput,
    TaskGetTool,
    TaskGetInput,
    TaskListTool,
    TaskOutputTool,
    TaskOutputInput,
    TaskStatus,
    TaskStopTool,
    TaskStopInput,
    TaskUpdateTool,
    TaskUpdateInput,
    sub_agent_manager,
)
from app.models.permission import PermissionLevel

pytestmark = pytest.mark.asyncio


class TestSubAgentManager:
    def test_create_and_get_task(self):
        mgr = sub_agent_manager
        task = mgr.create_task("test task", "do something")
        assert task.id is not None
        assert task.description == "test task"
        assert task.status == TaskStatus.PENDING

        got = mgr.get_task(task.id)
        assert got is not None
        assert got.id == task.id

    def test_list_tasks(self):
        mgr = sub_agent_manager
        mgr.create_task("task1", "do 1")
        mgr.create_task("task2", "do 2")
        tasks = mgr.list_tasks()
        assert len(tasks) >= 2

    def test_update_task(self):
        mgr = sub_agent_manager
        task = mgr.create_task("update test", "test")
        mgr.update_task(task.id, TaskStatus.COMPLETED, output="done")
        updated = mgr.get_task(task.id)
        assert updated is not None
        assert updated.status == TaskStatus.COMPLETED
        assert updated.output == "done"

    def test_cancel_task(self):
        mgr = sub_agent_manager
        task = mgr.create_task("cancel test", "test")
        ok = mgr.cancel_task(task.id)
        assert ok
        cancelled = mgr.get_task(task.id)
        assert cancelled is not None
        assert cancelled.status == TaskStatus.CANCELLED


class TestAgentTool:
    async def test_agent_tool_input_schema(self):
        tool = AgentTool()
        assert tool.name == "agent"
        assert tool.permission_level == PermissionLevel.WRITE

    async def test_agent_tool_execute(self):
        tool = AgentTool()
        result = await tool.execute(AgentToolInput(
            description="test task",
            prompt="do something",
        ))
        assert not result.is_error
        assert "test task" in result.output


class TestTaskTools:
    async def test_task_create_and_get(self):
        create = TaskCreateTool()
        result = await create.execute(TaskCreateInput(description="my task", prompt="do it"))
        assert not result.is_error
        task_id = result.metadata.get("task_id", "")

        get = TaskGetTool()
        result2 = await get.execute(TaskGetInput(task_id=task_id))
        assert not result2.is_error
        assert "my task" in result2.output

    async def test_task_list(self):
        tool = TaskListTool()
        result = await tool.execute(type("_", (), {})())
        assert not result.is_error

    async def test_task_stop(self):
        from app.tools.agent_tools import sub_agent_manager
        task = sub_agent_manager.create_task("stop test", "test")
        tool = TaskStopTool()
        result = await tool.execute(TaskStopInput(task_id=task.id))
        assert not result.is_error

    async def test_task_output(self):
        mgr = sub_agent_manager
        task = mgr.create_task("output test", "test")
        mgr.update_task(task.id, TaskStatus.COMPLETED, output="task result here")
        tool = TaskOutputTool()
        result = await tool.execute(TaskOutputInput(task_id=task.id))
        assert not result.is_error
        assert "task result" in result.output

    async def test_task_output_pending(self):
        mgr = sub_agent_manager
        task = mgr.create_task("pending test", "test")
        tool = TaskOutputTool()
        result = await tool.execute(TaskOutputInput(task_id=task.id))
        # Pending tasks return a message, not error
        assert "pending" in result.output.lower() or "not ready" in result.output.lower()
