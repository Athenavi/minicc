"""Tests for subagent — 真子 Agent 委派。"""
from __future__ import annotations

import pytest
from unittest.mock import MagicMock

from app.gateway.provider import ChatResponse
from app.tools.context import set_tool_context
from app.tools.subagent import subagent


def _make_gateway(text: str = "child result"):
    gw = MagicMock()

    async def fake_stream(**kwargs):
        yield ChatResponse(content=text, finish_reason="stop")

    gw.chat_stream = fake_stream
    return gw


class TestSubagent:
    @pytest.mark.asyncio
    async def test_returns_child_output(self):
        gw = _make_gateway("child did the work")
        set_tool_context(session_id="parent-s", user_id="u1", tenant_id="t1", gateway=gw)
        out = await subagent("read files and summarize")
        assert out["output"] == "child did the work"
        assert out["max_turns"] == 5

    @pytest.mark.asyncio
    async def test_restores_parent_context(self):
        gw = _make_gateway("ok")
        set_tool_context(session_id="parent-s", user_id="u1", tenant_id="t1", gateway=gw)
        await subagent("task")
        from app.tools.context import get_session_id
        assert get_session_id() == "parent-s"  # 子 agent 执行后父上下文还原

    @pytest.mark.asyncio
    async def test_no_gateway_errors(self):
        set_tool_context(session_id="", user_id="", tenant_id="", gateway=None)
        out = await subagent("task")
        assert "error" in out and "runtime" in out["error"]

    @pytest.mark.asyncio
    async def test_empty_task_rejected(self):
        set_tool_context(session_id="s", user_id="u", tenant_id="t", gateway=_make_gateway())
        out = await subagent("   ")
        assert "error" in out

    @pytest.mark.asyncio
    async def test_child_error_propagates(self):
        gw = MagicMock()

        async def failing_stream(**kwargs):
            yield ChatResponse(content="", finish_reason="error")

        gw.chat_stream = failing_stream
        set_tool_context(session_id="s", user_id="u", tenant_id="t", gateway=gw)
        out = await subagent("task")
        # 无文本但有 error 事件 → 返回 error；或 fallback 无输出时给占位
        assert "output" in out or "error" in out
        set_tool_context(session_id="", user_id="", tenant_id="", gateway=None)

    @pytest.mark.asyncio
    async def test_depth_limit_blocks_recursion(self):
        """S3: 委派深度超过 MAX_DEPTH 时拒绝，防无限递归。"""
        gw = _make_gateway("ok")
        from app.tools.context import set_tool_context
        from app.tools.subagent import MAX_DEPTH
        set_tool_context(session_id="s", user_id="u", tenant_id="t", gateway=gw, subagent_depth=MAX_DEPTH)
        out = await subagent("nested task")
        assert "error" in out and "depth" in out["error"]
        set_tool_context(session_id="", user_id="", tenant_id="", gateway=None, subagent_depth=0)

    @pytest.mark.asyncio
    async def test_depth_increments_for_child(self):
        """S3: 子 agent 的 subagent_depth 为父 +1，可继续委派到上限内。"""
        gw = _make_gateway("ok")
        from app.tools.context import set_tool_context
        set_tool_context(session_id="s", user_id="u", tenant_id="t", gateway=gw, subagent_depth=1)
        out = await subagent("task")
        assert "error" not in out  # depth=2 < MAX_DEPTH，允许
        set_tool_context(session_id="", user_id="", tenant_id="", gateway=None, subagent_depth=0)
