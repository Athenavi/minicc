"""Tests: Context compression — BudgetManager, Snip, Collapse, AutoCompact."""

from __future__ import annotations

from datetime import datetime, timezone

import pytest

from app.engine.compactor import (
    AutoCompactor,
    BudgetManager,
    CompactPipeline,
    ContextCollapser,
    SnipCompactor,
)
from app.models.chat import ContentBlock, Message, Role


def make_msg(role: Role, text: str) -> Message:
    return Message(role=role, content=text, created_at=datetime.now(timezone.utc))


def make_tool_result(text: str) -> Message:
    return Message(
        role=Role.tool,
        content=[ContentBlock(
            type="tool_result",
            tool_use_id="t1",
            content=[ContentBlock(type="text", text=text)],
        )],
        created_at=datetime.now(timezone.utc),
    )


class TestBudgetManager:
    def test_short_tool_result_preserved(self):
        mgr = BudgetManager(max_tool_result_chars=100)
        msg = make_tool_result("short")
        result = mgr.apply([msg])
        assert len(result.messages) == 1
        assert "short" in str(result.messages[0].content)

    def test_long_tool_result_truncated(self):
        mgr = BudgetManager(max_tool_result_chars=10)
        msg = make_tool_result("x" * 100)
        result = mgr.apply([msg])
        assert "truncated" in str(result.messages[0].content)

    def test_estimate_tokens(self):
        mgr = BudgetManager()
        assert mgr.estimate_tokens("hello") == 1  # 5/4 = 1
        assert mgr.estimate_tokens("a" * 100) == 25


class TestSnipCompactor:
    def test_below_threshold_no_change(self):
        snip = SnipCompactor(threshold=100_000)
        msgs = [make_msg(Role.user, "hi")]
        result = snip.compact_if_needed(msgs, 50)
        assert len(result.messages) == 1

    def test_above_threshold_trims_long_tool_results(self):
        snip = SnipCompactor(threshold=10)
        msgs = [
            make_msg(Role.user, "hello"),
            make_tool_result("x" * 5000),
            make_tool_result("y" * 5000),
        ]
        result = snip.compact_if_needed(msgs, 20000)
        assert result.tokens_freed > 0


class TestContextCollapser:
    def test_below_threshold_no_change(self):
        collapser = ContextCollapser(threshold=100_000)
        msgs = [make_msg(Role.user, "hi")]
        result = collapser.apply_if_needed(msgs, 50)
        assert len(result.messages) == 1

    def test_collapse_tool_rounds(self):
        collapser = ContextCollapser(threshold=10, collapse_window=2)
        msgs = [
            make_msg(Role.user, "check files"),
            make_msg(Role.assistant, "let me look"),
            Message(
                role=Role.assistant,
                content=[ContentBlock(type="tool_use", id="tu1", name="read_file", input={"path": "x.py"})],
                created_at=datetime.now(timezone.utc),
            ),
            make_tool_result("content here"),
        ]
        result = collapser.apply_if_needed(msgs, 50000)
        # Should have collapsed
        assert len(result.messages) < len(msgs)


class TestAutoCompactor:
    @pytest.mark.asyncio
    async def test_below_min_keep_no_change(self):
        comp = AutoCompactor(min_keep_messages=6)
        msgs = [make_msg(Role.user, f"msg{i}") for i in range(4)]
        result = await comp.compact(msgs, 100, 1000)
        assert len(result.messages) == 4

    @pytest.mark.asyncio
    async def test_compresses_early_messages(self):
        comp = AutoCompactor(min_keep_messages=2, target_ratio=0.5)
        msgs = [make_msg(Role.user, f"early msg {i}") for i in range(10)]
        result = await comp.compact(msgs, 50000, 30000)
        # Should have compressed early messages into summary
        assert len(result.messages) < 10


class TestCompactPipeline:
    @pytest.mark.asyncio
    async def test_full_pipeline_no_crash(self):
        pipeline = CompactPipeline()
        msgs = [
            make_msg(Role.user, f"test message {i}") for i in range(20)
        ]
        result = await pipeline.apply_all(msgs, 5000)
        assert result.messages is not None
