"""Tests for the enhanced Agent Engine.

Covers:
  1. test_engine_basic_run — mock gateway, verify events are yielded
  2. test_parallel_tool_execution — verify independent tools run concurrently
  3. test_tool_approval_flow — verify approval request for dangerous tools
  4. test_session_save_load — verify session persistence
"""
from __future__ import annotations

import asyncio
import json
import time
import uuid
from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.agent.engine import (
    AgentEngine,
    AgentEvent,
    AgentSession,
    AgentTask,
    ContextManager,
    DANGEROUS_TOOLS,
    PromptEngine,
    ToolApprovalRequest,
    ToolApprovalResponse,
)
from app.gateway.provider import ChatMessage, ChatResponse, ToolCall


# ═══════════════════════════════════════════════════════════════════════════
# Helpers / Fixtures
# ═══════════════════════════════════════════════════════════════════════════

def _make_gateway(chat_stream_chunks: list[list[ChatResponse]] | None = None):
    """Create a mock gateway that yields predefined responses per turn.

    Parameters
    ----------
    chat_stream_chunks:
        A list of lists.  Each inner list is a sequence of ChatResponse
        objects yielded for one call to ``chat_stream``.  After all chunks
        are exhausted the gateway yields a single stop response.
    """
    gw = MagicMock()
    if chat_stream_chunks is None:
        chat_stream_chunks = [[
            ChatResponse(content="Hello!", finish_reason="stop"),
        ]]

    call_index = {"i": 0}

    async def _chat_stream(*_args, **_kwargs):
        idx = call_index["i"]
        call_index["i"] += 1
        chunks = chat_stream_chunks[idx] if idx < len(chat_stream_chunks) else [
            ChatResponse(content="", finish_reason="stop"),
        ]
        for chunk in chunks:
            yield chunk

    gw.chat_stream = _chat_stream
    return gw


def _make_tool_registry(tools: dict | None = None):
    """Create a mock tool registry.

    Parameters
    ----------
    tools:
        ``{name: handler}`` where *handler* is an async callable.
    """
    registry = MagicMock()
    tools = tools or {}
    registry.get.side_effect = lambda name: (MagicMock() if name in tools else None)

    async def _execute(name, params):
        handler = tools.get(name)
        if handler:
            return await handler(**params)
        return {"error": f"Tool '{name}' not found"}

    registry.execute = AsyncMock(side_effect=_execute)
    return registry


# ═══════════════════════════════════════════════════════════════════════════
# 1. Basic run
# ═══════════════════════════════════════════════════════════════════════════

class TestEngineBasicRun:
    """Verify that the engine yields correct events for a simple text response."""

    @pytest.mark.asyncio
    async def test_basic_text_response(self):
        gateway = _make_gateway()
        engine = AgentEngine(gateway=gateway)
        task = AgentTask(id="t1", content="Hello", max_turns=3)

        events = []
        async for event in engine.run(task):
            events.append(event)

        types = [e.type for e in events]
        assert "text" in types
        assert "done" in types
        # No errors
        assert "error" not in types

    @pytest.mark.asyncio
    async def test_events_have_content(self):
        gateway = _make_gateway()
        engine = AgentEngine(gateway=gateway)
        task = AgentTask(id="t2", content="Hi there")

        events = []
        async for event in engine.run(task):
            events.append(event)

        text_events = [e for e in events if e.type == "text"]
        assert len(text_events) >= 1
        assert text_events[0].content == "Hello!"

    @pytest.mark.asyncio
    async def test_done_event_has_token_counts(self):
        gateway = _make_gateway()
        engine = AgentEngine(gateway=gateway)
        task = AgentTask(id="t3", content="Count tokens")

        events = []
        async for event in engine.run(task):
            events.append(event)

        done_events = [e for e in events if e.type == "done"]
        assert len(done_events) == 1
        # Token counts are non-negative integers
        assert done_events[0].input_tokens >= 0
        assert done_events[0].output_tokens >= 0

    @pytest.mark.asyncio
    async def test_system_prompt_assembly(self):
        """Verify PromptEngine assembles the system prompt."""
        pe = PromptEngine(base_prompt="You are helpful.", extras={"workspace": "Root: /app"})
        prompt = pe.assemble()
        assert "You are helpful" in prompt
        assert "Root: /app" in prompt

    @pytest.mark.asyncio
    async def test_error_handling(self):
        """Verify the engine handles gateway errors gracefully."""
        gw = MagicMock()

        async def _failing_stream(*_a, **_kw):
            raise RuntimeError("LLM exploded")
            yield  # pragma: no cover  (make it an async generator)

        gw.chat_stream = _failing_stream
        engine = AgentEngine(gateway=gw)
        task = AgentTask(id="err1", content="Boom")

        events = []
        async for event in engine.run(task):
            events.append(event)

        assert any(e.type == "error" for e in events)
        assert any("exploded" in e.error for e in events if e.type == "error")


# ═══════════════════════════════════════════════════════════════════════════
# 2. Parallel tool execution
# ═══════════════════════════════════════════════════════════════════════════

class TestParallelToolExecution:
    """Verify that independent (non-dangerous) tool calls run concurrently."""

    @pytest.mark.asyncio
    async def test_parallel_independent_tools(self):
        """Two read_file calls should execute concurrently, not sequentially."""
        call_log: list[tuple[str, float]] = []

        async def slow_read_file(path: str = "."):
            start = time.monotonic()
            await asyncio.sleep(0.15)
            end = time.monotonic()
            call_log.append(("read_file", end))
            return {"content": f"Contents of {path}"}

        registry = _make_tool_registry({"read_file": slow_read_file})

        # Turn 1: LLM requests two read_file calls
        turn1_chunks = [
            ChatResponse(
                content="Let me read those files.",
                tool_calls=[
                    ToolCall(id="tc1", name="read_file", arguments='{"path": "a.py"}'),
                    ToolCall(id="tc2", name="read_file", arguments='{"path": "b.py"}'),
                ],
                finish_reason="tool_calls",
            ),
        ]
        # Turn 2: LLM is done
        turn2_chunks = [
            ChatResponse(content="Done reading!", finish_reason="stop"),
        ]

        gateway = _make_gateway([turn1_chunks, turn2_chunks])
        engine = AgentEngine(gateway=gateway, tool_registry=registry)
        task = AgentTask(
            id="par1",
            content="Read a.py and b.py",
            tools=[
                {"name": "read_file", "description": "Read a file", "parameters": {}},
            ],
            max_turns=5,
        )

        events = []
        async for event in engine.run(task):
            events.append(event)

        # Both tools should have been called
        tool_result_events = [e for e in events if e.type == "tool_result"]
        assert len(tool_result_events) == 2

        # Verify concurrency: if they ran in parallel, total wall time
        # should be ~0.15s, not ~0.3s.  Allow some margin.
        assert len(call_log) == 2
        # The gap between the two finishes should be small (< 0.08s)
        gap = abs(call_log[0][1] - call_log[1][1])
        assert gap < 0.08, f"Tools ran sequentially (gap={gap:.3f}s), expected parallel"

    @pytest.mark.asyncio
    async def test_tool_result_events_emitted(self):
        """Verify tool_result events are yielded with correct metadata."""
        async def echo(**kwargs):
            return {"echo": kwargs}

        registry = _make_tool_registry({"read_file": echo})

        turn1 = [
            ChatResponse(
                tool_calls=[
                    ToolCall(id="t1", name="read_file", arguments='{"path": "x.py"}'),
                ],
                finish_reason="tool_calls",
            ),
        ]
        turn2 = [ChatResponse(content="Done", finish_reason="stop")]
        gateway = _make_gateway([turn1, turn2])
        engine = AgentEngine(gateway=gateway, tool_registry=registry)
        task = AgentTask(
            id="par2",
            content="Read x.py",
            tools=[{"name": "read_file", "description": "Read", "parameters": {}}],
            max_turns=5,
        )

        events = []
        async for event in engine.run(task):
            events.append(event)

        tr_events = [e for e in events if e.type == "tool_result"]
        assert len(tr_events) == 1
        assert tr_events[0].tool_name == "read_file"
        assert tr_events[0].tool_call_id == "t1"


# ═══════════════════════════════════════════════════════════════════════════
# 3. Tool approval flow
# ═══════════════════════════════════════════════════════════════════════════

class TestToolApprovalFlow:
    """Verify that dangerous tools trigger the approval mechanism."""

    @pytest.mark.asyncio
    async def test_dangerous_tool_needs_approval(self):
        """A write_file call should trigger approval when an approval_handler is set."""
        approval_called = {"value": False, "request": None}

        async def mock_approval(req: ToolApprovalRequest) -> ToolApprovalResponse:
            approval_called["value"] = True
            approval_called["request"] = req
            return ToolApprovalResponse(approved=True, reason="go ahead")

        async def write_handler(path: str, content: str = ""):
            return {"bytes": len(content)}

        registry = _make_tool_registry({"write_file": write_handler})

        turn1 = [
            ChatResponse(
                tool_calls=[
                    ToolCall(id="w1", name="write_file", arguments='{"path": "out.txt", "content": "hi"}'),
                ],
                finish_reason="tool_calls",
            ),
        ]
        turn2 = [ChatResponse(content="File written!", finish_reason="stop")]
        gateway = _make_gateway([turn1, turn2])
        engine = AgentEngine(
            gateway=gateway,
            tool_registry=registry,
            approval_handler=mock_approval,
        )
        task = AgentTask(
            id="appr1",
            content="Write out.txt",
            tools=[{"name": "write_file", "description": "Write", "parameters": {}}],
            max_turns=5,
        )

        events = []
        async for event in engine.run(task):
            events.append(event)

        assert approval_called["value"], "Approval handler was not called"
        assert approval_called["request"].tool_name == "write_file"
        assert approval_called["request"].risk_level == "medium"
        assert "out.txt" in approval_called["request"].description

    @pytest.mark.asyncio
    async def test_approval_denied_blocks_execution(self):
        """When approval is denied, the tool should NOT be executed."""
        tool_executed = {"value": False}

        async def deny_approval(req: ToolApprovalRequest) -> ToolApprovalResponse:
            return ToolApprovalResponse(approved=False, reason="too dangerous")

        async def write_handler(path: str, content: str = ""):
            tool_executed["value"] = True
            return {"bytes": len(content)}

        registry = _make_tool_registry({"write_file": write_handler})

        turn1 = [
            ChatResponse(
                tool_calls=[
                    ToolCall(id="w2", name="write_file", arguments='{"path": "bad.txt", "content": "x"}'),
                ],
                finish_reason="tool_calls",
            ),
        ]
        turn2 = [ChatResponse(content="OK", finish_reason="stop")]
        gateway = _make_gateway([turn1, turn2])
        engine = AgentEngine(
            gateway=gateway,
            tool_registry=registry,
            approval_handler=deny_approval,
        )
        task = AgentTask(
            id="appr2",
            content="Write bad.txt",
            tools=[{"name": "write_file", "description": "Write", "parameters": {}}],
            max_turns=5,
        )

        events = []
        async for event in engine.run(task):
            events.append(event)

        assert not tool_executed["value"], "Tool should not have been executed"
        # Should have a tool_result with error
        tr_events = [e for e in events if e.type == "tool_result"]
        assert len(tr_events) == 1
        result = json.loads(tr_events[0].content)
        assert "denied" in result["error"].lower()

    @pytest.mark.asyncio
    async def test_high_risk_for_shell_exec(self):
        """shell_exec should be classified as high risk."""
        async def mock_approval(req: ToolApprovalRequest) -> ToolApprovalResponse:
            assert req.risk_level == "high"
            return ToolApprovalResponse(approved=True)

        async def shell_handler(command: str = "", **kw):
            return {"exit_code": 0}

        registry = _make_tool_registry({"shell_exec": shell_handler})

        turn1 = [
            ChatResponse(
                tool_calls=[
                    ToolCall(id="s1", name="shell_exec", arguments='{"command": "ls"}'),
                ],
                finish_reason="tool_calls",
            ),
        ]
        turn2 = [ChatResponse(content="OK", finish_reason="stop")]
        gateway = _make_gateway([turn1, turn2])
        engine = AgentEngine(
            gateway=gateway,
            tool_registry=registry,
            approval_handler=mock_approval,
        )
        task = AgentTask(
            id="appr3",
            content="Run ls",
            tools=[{"name": "shell_exec", "description": "Shell", "parameters": {}}],
            max_turns=5,
        )

        events = []
        async for event in engine.run(task):
            events.append(event)

        assert any(e.type == "tool_result" for e in events)

    @pytest.mark.asyncio
    async def test_approval_via_submit_approval(self):
        """Test the external approval API (submit_approval) when no handler is set."""
        async def write_handler(path: str, content: str = ""):
            return {"bytes": len(content)}

        registry = _make_tool_registry({"write_file": write_handler})

        turn1 = [
            ChatResponse(
                tool_calls=[
                    ToolCall(id="ext1", name="write_file", arguments='{"path": "out.txt", "content": "data"}'),
                ],
                finish_reason="tool_calls",
            ),
        ]
        turn2 = [ChatResponse(content="Written!", finish_reason="stop")]
        gateway = _make_gateway([turn1, turn2])
        engine = AgentEngine(gateway=gateway, tool_registry=registry)

        task = AgentTask(
            id="appr4",
            content="Write out.txt",
            tools=[{"name": "write_file", "description": "Write", "parameters": {}}],
            max_turns=5,
        )

        collected_events: list[AgentEvent] = []

        async def _run_engine():
            async for event in engine.run(task):
                collected_events.append(event)

        async def _approve_after_delay():
            # Give the engine a moment to reach the approval wait
            await asyncio.sleep(0.1)
            await engine.submit_approval(
                "ext1",
                ToolApprovalResponse(approved=True, reason="ok"),
            )

        # Run both concurrently
        await asyncio.gather(_run_engine(), _approve_after_delay())

        tr_events = [e for e in collected_events if e.type == "tool_result"]
        assert len(tr_events) == 1
        result = json.loads(tr_events[0].content)
        assert result.get("bytes", 0) > 0


# ═══════════════════════════════════════════════════════════════════════════
# 4. Session persistence
# ═══════════════════════════════════════════════════════════════════════════

class TestSessionSaveLoad:
    """Verify session save/load against a mock pool."""

    @pytest.mark.asyncio
    async def test_session_save_and_load(self):
        """Round-trip: save a session, load it back."""
        stored_data: dict = {}

        class FakePool:
            async def execute(self, query, *args):
                if "INSERT" in query or "ON CONFLICT" in query:
                    stored_data["id"] = args[0]
                    stored_data["user_id"] = args[1]
                    stored_data["messages_json"] = args[2]
                    stored_data["created_at"] = args[3]
                    stored_data["updated_at"] = args[4]

            async def fetchrow(self, query, *args):
                if stored_data.get("id") == args[0]:
                    return {
                        "id": stored_data["id"],
                        "user_id": stored_data["user_id"],
                        "messages_json": stored_data["messages_json"],
                        "created_at": stored_data["created_at"],
                        "updated_at": stored_data["updated_at"],
                    }
                return None

        pool = FakePool()

        # Create and save
        session = AgentSession(id="sess-1", user_id="user-42")
        session.append_message("user", "Hello")
        session.append_message("assistant", "Hi there!")
        await session.save(pool)

        assert stored_data["id"] == "sess-1"
        assert stored_data["user_id"] == "user-42"

        # Load back
        loaded = await AgentSession.load(pool, "sess-1")
        assert loaded is not None
        assert loaded.id == "sess-1"
        assert loaded.user_id == "user-42"
        assert len(loaded.messages) == 2
        assert loaded.messages[0]["role"] == "user"
        assert loaded.messages[0]["content"] == "Hello"
        assert loaded.messages[1]["role"] == "assistant"
        assert loaded.messages[1]["content"] == "Hi there!"

    @pytest.mark.asyncio
    async def test_session_load_nonexistent(self):
        """Loading a non-existent session should return None."""
        class FakePool:
            async def fetchrow(self, query, *args):
                return None

        result = await AgentSession.load(FakePool(), "no-such-session")
        assert result is None

    @pytest.mark.asyncio
    async def test_session_load_null_pool(self):
        """Loading with a None pool should return None."""
        result = await AgentSession.load(None, "any-id")
        assert result is None

    @pytest.mark.asyncio
    async def test_session_save_null_pool(self):
        """Saving with a None pool should be a no-op (no error)."""
        session = AgentSession(id="x", user_id="u")
        await session.save(None)  # Should not raise

    @pytest.mark.asyncio
    async def test_engine_saves_session_after_run(self):
        """Verify the engine persists the session after a run."""
        saved_ids: list[str] = []

        class TrackingPool:
            async def execute(self, query, *args):
                if "INSERT" in query or "ON CONFLICT" in query:
                    saved_ids.append(args[0])

            async def fetchrow(self, query, *args):
                return None

        pool = TrackingPool()
        gateway = _make_gateway()
        engine = AgentEngine(gateway=gateway, db_pool=pool)
        task = AgentTask(id="save1", session_id="sess-auto", content="Hi")

        events = []
        async for event in engine.run(task):
            events.append(event)

        assert "sess-auto" in saved_ids

    @pytest.mark.asyncio
    async def test_session_message_helpers(self):
        """Verify helper methods on AgentSession."""
        session = AgentSession(id="h1")
        session.append_message("user", "Hello")
        session.append_message("assistant", "Hi", extra_field="value")
        session.append_tool_call("tc1", "read_file", '{"path": "x.py"}')
        session.append_tool_result("tc1", '{"content": "data"}')

        assert len(session.messages) == 4
        assert session.messages[0] == {"role": "user", "content": "Hello"}
        assert session.messages[1]["extra_field"] == "value"
        assert session.messages[2]["tool_calls"][0]["id"] == "tc1"
        assert session.messages[3]["role"] == "tool"
        assert session.messages[3]["tool_call_id"] == "tc1"


# ═══════════════════════════════════════════════════════════════════════════
# Supplementary tests
# ═══════════════════════════════════════════════════════════════════════════

class TestContextManager:
    """Verify context compression."""

    def test_compress_under_limit(self):
        cm = ContextManager(max_messages=100)
        msgs = [{"role": "user", "content": "hi"}]
        assert cm.compress(msgs) == msgs

    def test_compress_drops_oldest(self):
        cm = ContextManager(max_messages=5)
        msgs = [
            {"role": "system", "content": "sys"},
            {"role": "user", "content": "m1"},
            {"role": "assistant", "content": "m2"},
            {"role": "user", "content": "m3"},
            {"role": "assistant", "content": "m4"},
            {"role": "user", "content": "m5"},
            {"role": "assistant", "content": "m6"},
        ]
        result = cm.compress(msgs)
        # system + at most 4 others
        assert len(result) <= 5
        # System message is always preserved
        assert result[0]["role"] == "system"
        # Last message is preserved
        assert result[-1]["content"] == "m6"

    def test_compress_preserves_system(self):
        cm = ContextManager(max_messages=3)
        msgs = [
            {"role": "system", "content": "You are helpful."},
            {"role": "user", "content": "a"},
            {"role": "assistant", "content": "b"},
            {"role": "user", "content": "c"},
        ]
        result = cm.compress(msgs)
        assert result[0]["role"] == "system"


class TestPromptEngine:
    """Verify prompt assembly."""

    def test_base_only(self):
        pe = PromptEngine(base_prompt="Be helpful.")
        assert pe.assemble() == "Be helpful."

    def test_with_extras(self):
        pe = PromptEngine(
            base_prompt="Base",
            extras={"env": "Env: production"},
        )
        prompt = pe.assemble()
        assert "Base" in prompt
        assert "Env: production" in prompt

    def test_with_context(self):
        pe = PromptEngine(
            base_prompt="Base",
            extras={"ws": "Workspace: {workspace}"},
        )
        prompt = pe.assemble({"workspace": "/home/user/project"})
        assert "/home/user/project" in prompt


class TestToolApprovalRequest:
    """Verify ToolApprovalRequest data class."""

    def test_to_dict(self):
        req = ToolApprovalRequest(
            tool_name="write_file",
            arguments={"path": "x.py"},
            risk_level="medium",
            description="Will write file",
        )
        d = req.to_dict()
        assert d["tool_name"] == "write_file"
        assert d["risk_level"] == "medium"


class TestDangerousTools:
    """Verify the set of dangerous tools matches expectations."""

    def test_write_file_is_dangerous(self):
        assert "write_file" in DANGEROUS_TOOLS

    def test_shell_exec_is_dangerous(self):
        assert "shell_exec" in DANGEROUS_TOOLS

    def test_read_file_is_safe(self):
        assert "read_file" not in DANGEROUS_TOOLS

    def test_grep_files_is_safe(self):
        assert "grep_files" not in DANGEROUS_TOOLS


class TestAgentTask:
    """Verify AgentTask parsing."""

    def test_parse_basic(self):
        data = {
            "task_id": "t1",
            "tenant_id": "ten",
            "user_id": "u1",
            "content": "Hello",
        }
        task = AgentTask.parse(data)
        assert task.id == "t1"
        assert task.tenant_id == "ten"
        assert task.user_id == "u1"
        assert task.content == "Hello"

    def test_parse_defaults(self):
        task = AgentTask.parse({})
        assert task.id == ""
        assert task.max_turns == 10


class TestConvertTools:
    """Verify tool definition conversion."""

    def test_basic_conversion(self):
        engine = AgentEngine(gateway=MagicMock())
        tools = [
            {"name": "read_file", "description": "Read", "parameters": {"type": "object"}},
        ]
        result = engine._convert_tools(tools)
        assert len(result) == 1
        assert result[0]["type"] == "function"
        assert result[0]["function"]["name"] == "read_file"

    def test_parameters_json_fallback(self):
        engine = AgentEngine(gateway=MagicMock())
        tools = [
            {"name": "tool1", "description": "T", "parameters_json": '{"type": "object"}'},
        ]
        result = engine._convert_tools(tools)
        assert result[0]["function"]["parameters"] == {"type": "object"}


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
