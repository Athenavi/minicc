"""Tests: PermissionHandler — approval state machine."""

from __future__ import annotations

import asyncio

import pytest

from app.core.permission import PermissionHandler, PermissionResult
from app.models.permission import PermissionLevel
from app.models.tool import ToolCall


@pytest.fixture
def handler():
    return PermissionHandler()


@pytest.fixture
def tool_call():
    return ToolCall(id="tc1", name="write_to_file", type="file_write", input={"path": "test.py", "content": "x"})


@pytest.mark.asyncio
class TestPermissionHandler:
    async def test_read_level_auto_approved(self, handler, tool_call):
        """READ 级别自动放行，不触发审批。"""
        result = await handler.request_permission(tool_call, PermissionLevel.READ)
        assert result == PermissionResult.APPROVED
        assert len(handler._pending) == 0

    async def test_write_level_waits_for_approval(self, handler, tool_call):
        """WRITE 级别等待审批，批准后放行。"""
        async def approve():
            await asyncio.sleep(0.05)
            handler.handle_user_response("test_id", "approve")

        # 先启动审批
        async def request():
            # mock: override the request id
            handler._pending.clear()
            return await handler.request_permission(tool_call, PermissionLevel.WRITE)

        # 实际测试用正常流程
        async def run():
            # Start approval in background
            task = asyncio.create_task(handler.request_permission(tool_call, PermissionLevel.WRITE, reason="test"))
            await asyncio.sleep(0.05)
            # Find the pending request and approve it
            for rid, pending in handler._pending.items():
                handler.handle_user_response(rid, "approve")
            result = await task
            return result

        result = await run()
        assert result == PermissionResult.APPROVED

    async def test_write_level_rejected(self, handler, tool_call):
        """拒绝后返回 REJECTED。"""
        async def run():
            task = asyncio.create_task(handler.request_permission(tool_call, PermissionLevel.WRITE, reason="test"))
            await asyncio.sleep(0.05)
            for rid in handler._pending:
                handler.handle_user_response(rid, "reject")
            return await task

        result = await run()
        assert result == PermissionResult.REJECTED

    async def test_always_allow_skips_approval(self, handler, tool_call):
        """'始终允许'后同一工具不再触发审批。"""
        # 第一次：需要审批
        async def first():
            task = asyncio.create_task(handler.request_permission(tool_call, PermissionLevel.WRITE, reason="test"))
            await asyncio.sleep(0.05)
            for rid in handler._pending:
                handler.handle_user_response(rid, "always_allow")
            return await task

        result1 = await first()
        assert result1 == PermissionResult.APPROVED
        assert handler._is_always_allowed("write_to_file", PermissionLevel.WRITE)

        # 第二次：自动放行
        result2 = await handler.request_permission(tool_call, PermissionLevel.WRITE)
        assert result2 == PermissionResult.APPROVED

    async def test_always_allow_higher_covers_lower(self, handler):
        """EXECUTE 级别的始终允许包含 WRITE 和 READ。"""
        handler._always_allow["bash"] = PermissionLevel.EXECUTE
        tc = ToolCall(id="t2", name="bash", type="bash", input={"command": "ls"})
        assert handler._is_always_allowed("bash", PermissionLevel.EXECUTE)
        assert handler._is_always_allowed("bash", PermissionLevel.WRITE)
        assert handler._is_always_allowed("bash", PermissionLevel.READ)

    async def test_denied_tool_remembered(self, handler, tool_call):
        """拒绝后同一工具不再重复请求。"""
        async def run():
            task = asyncio.create_task(handler.request_permission(tool_call, PermissionLevel.WRITE, reason="test"))
            await asyncio.sleep(0.05)
            for rid in handler._pending:
                handler.handle_user_response(rid, "reject")
            await task
            # 第二次请求同一工具
            result = await handler.request_permission(tool_call, PermissionLevel.WRITE)
            return result

        result = await run()
        assert result == PermissionResult.REJECTED

    async def test_cancel_all_pending(self, handler):
        """取消所有待审批请求。"""
        tc1 = ToolCall(id="a", name="write_to_file", type="file_write", input={})
        tc2 = ToolCall(id="b", name="bash", type="bash", input={"command": "ls"})

        async def run():
            t1 = asyncio.create_task(handler.request_permission(tc1, PermissionLevel.WRITE, reason="test"))
            t2 = asyncio.create_task(handler.request_permission(tc2, PermissionLevel.EXECUTE, reason="test"))
            await asyncio.sleep(0.05)
            handler.cancel_all_pending()
            r1 = await t1
            r2 = await t2
            return r1, r2

        r1, r2 = await run()
        assert r1 == PermissionResult.REJECTED
        assert r2 == PermissionResult.REJECTED

    async def test_timeout_results_in_rejection(self, handler, tool_call):
        """超时自动拒绝。"""
        handler._approval_timeout = 0.1  # 100ms for testing
        result = await handler.request_permission(tool_call, PermissionLevel.WRITE, reason="test", diff_preview="+x")
        assert result == PermissionResult.TIMEOUT
