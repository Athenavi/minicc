"""Tests: QueryEngine — main loop orchestration."""

from __future__ import annotations

import pytest

pytestmark = pytest.mark.asyncio

from datetime import datetime, timezone
from typing import AsyncGenerator
from unittest.mock import AsyncMock, MagicMock

import pytest
from pydantic import BaseModel

from app.core.context_builder import ContextBuilder
from app.engine.llm_provider import LLMProvider, StreamEvent
from app.engine.query_engine import QueryEngine, QueryEngineConfig
from app.models.chat import Message, Role
from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolRegistry


# -- Mock LLM Provider --


class MockProvider(LLMProvider):
    """Mock provider that yields canned events."""

    def __init__(self, events: list[StreamEvent] | None = None):
        super().__init__("mock-key", "mock-model")
        self.events = events or []

    async def send_message(
        self, system_prompt: str, messages: list[dict], tools: list[dict] | None = None, max_tokens: int = 8192
    ) -> AsyncGenerator[StreamEvent, None]:
        for event in self.events:
            yield event


# -- Mock Tools --


class EchoInput(BaseModel):
    msg: str


class EchoTool(BaseTool):
    name = "echo"
    description = "echo back the input"
    input_schema = EchoInput
    permission_level = PermissionLevel.READ

    async def execute(self, input_data: EchoInput, context=None) -> ToolResult:
        return ToolResult(tool_call_id="test", output=input_data.msg)


class FailTool(BaseTool):
    name = "fail"
    description = "always fails"
    input_schema = EchoInput
    permission_level = PermissionLevel.READ

    async def execute(self, input_data: EchoInput, context=None) -> ToolResult:
        return ToolResult(tool_call_id="test", output="simulated failure", is_error=True)


# -- Fixtures --


@pytest.fixture
def registry():
    r = ToolRegistry()
    r.register(EchoTool())
    r.register(FailTool())
    return r


@pytest.fixture
def context_builder(tmp_path):
    return ContextBuilder(str(tmp_path))


async def _collect(engine: QueryEngine, content: str) -> list[dict]:
    """Helper: collect all events from submit_message."""
    events = []
    async for event in engine.submit_message(content):
        events.append(event)
    return events


def make_engine(session_id: str, provider: LLMProvider, registry: ToolRegistry, ctx_builder: ContextBuilder,
                **kwargs) -> QueryEngine:
    """Helper: create a QueryEngine with QueryEngineConfig."""
    return QueryEngine(QueryEngineConfig(
        session_id=session_id,
        provider=provider,
        tool_registry=registry,
        context_builder=ctx_builder,
        **kwargs,
    ))


# -- Tests --


class TestQueryEngineBasics:
    async def test_empty_response(self, registry, context_builder):
        """Text-only response with no tool calls."""
        provider = MockProvider([StreamEvent(type="end", data={})])
        engine = make_engine("s1", provider, registry, context_builder)
        events = await _collect(engine, "hello")

        assert len(events) >= 2
        # First event should be user_message
        assert events[0]["type"] == "user_message"
        # Last event should be message_complete
        assert events[-1]["type"] == "message_complete"

    async def test_text_streaming(self, registry, context_builder):
        """Text chunks are yielded in order."""
        provider = MockProvider([
            StreamEvent(type="text", data={"text": "Hello"}),
            StreamEvent(type="text", data={"text": " world"}),
            StreamEvent(type="end", data={}),
        ])
        engine = make_engine("s2", provider, registry, context_builder)
        events = await _collect(engine, "hi")

        texts = [e["payload"]["text"] for e in events if e["type"] == "text_chunk"]
        assert texts == ["Hello", " world"]

    async def test_mutable_messages_accumulate(self, registry, context_builder):
        """Messages accumulate across turns (session state)."""
        provider = MockProvider([StreamEvent(type="end", data={})])
        engine = make_engine("s3", provider, registry, context_builder)
        await _collect(engine, "first")
        await _collect(engine, "second")

        assert len(engine.mutable_messages) == 2
        assert engine.mutable_messages[0].role == Role.user
        assert engine.mutable_messages[0].content == "first"
        assert engine.mutable_messages[1].role == Role.user
        assert engine.mutable_messages[1].content == "second"

    async def test_cancel_stops_execution(self, registry, context_builder):
        """abort_event stops the main loop."""
        provider = MockProvider([
            StreamEvent(type="text", data={"text": "thinking..."}),
            StreamEvent(type="text", data={"text": "still going"}),
            StreamEvent(type="end", data={}),
        ])
        engine = make_engine("s4", provider, registry, context_builder)

        async def collect_with_cancel():
            events = []
            async for event in engine.submit_message("test"):
                events.append(event)
                # Cancel after receiving first text chunk
                if event["type"] == "text_chunk":
                    engine.cancel()
            return events

        events = await collect_with_cancel()
        assert events[-1]["type"] == "message_complete"
        assert events[-1]["payload"].get("interrupted") is True


class TestQueryEngineToolCalls:
    async def test_tool_call_execution(self, registry, context_builder):
        """Tool call is executed and result is yielded."""
        provider = MockProvider([
            StreamEvent(
                type="tool_use",
                data={"id": "tc1", "name": "echo", "input": {"msg": "ping"}},
            ),
            StreamEvent(type="end", data={}),
        ])
        engine = make_engine("s5", provider, registry, context_builder)
        events = await _collect(engine, "run echo")

        # Should have tool_call_start
        starts = [e for e in events if e["type"] == "tool_call_start"]
        assert len(starts) >= 1
        assert starts[0]["payload"]["name"] == "echo"

        # Should have tool_call_result
        results = [e for e in events if e["type"] == "tool_call_result"]
        assert len(results) >= 1
        assert results[0]["payload"]["output"] == "ping"

    async def test_unknown_tool(self, registry, context_builder):
        """Unknown tool name returns error."""
        provider = MockProvider([
            StreamEvent(
                type="tool_use",
                data={"id": "tc2", "name": "nonexistent", "input": {}},
            ),
            StreamEvent(type="end", data={}),
        ])
        engine = make_engine("s6", provider, registry, context_builder)
        events = await _collect(engine, "run bad tool")

        results = [e for e in events if e["type"] == "tool_call_result"]
        assert len(results) >= 1
        assert results[0]["payload"]["is_error"] is True

    async def test_tool_execution_error(self, registry, context_builder):
        """Tool that returns is_error=True is propagated."""
        provider = MockProvider([
            StreamEvent(
                type="tool_use",
                data={"id": "tc3", "name": "fail", "input": {"msg": "x"}},
            ),
            StreamEvent(type="end", data={}),
        ])
        engine = make_engine("s7", provider, registry, context_builder)
        events = await _collect(engine, "run fail")

        results = [e for e in events if e["type"] == "tool_call_result"]
        assert results[0]["payload"]["is_error"] is True
        assert "simulated failure" in results[0]["payload"]["output"]


class TestQueryEngineMultipleTurns:
    async def test_text_then_tool_then_text(self, registry, context_builder):
        """Multi-turn: text → tool → text completes."""
        provider = MockProvider([
            StreamEvent(type="text", data={"text": "Let me check..."}),
            StreamEvent(
                type="tool_use",
                data={"id": "t1", "name": "echo", "input": {"msg": "check"}},
            ),
            StreamEvent(type="end", data={}),
        ])
        engine = make_engine("s8", provider, registry, context_builder)
        events = await _collect(engine, "multi turn")

        assert any(e["type"] == "text_chunk" for e in events)
        assert any(e["type"] == "tool_call_start" for e in events)
        assert any(e["type"] == "tool_call_result" for e in events)
        assert events[-1]["type"] == "message_complete"

    async def test_max_tool_rounds_enforced(self, registry, context_builder):
        """Engine stops after reaching max_tool_rounds."""
        provider = MockProvider([
            StreamEvent(
                type="tool_use",
                data={"id": "tx", "name": "echo", "input": {"msg": "x"}},
            ),
            StreamEvent(type="end", data={}),
        ])
        engine = make_engine("s9", provider, registry, context_builder, max_tool_rounds=2)
        events = await _collect(engine, "loop test")
        assert events[-1]["type"] == "message_complete"


class TestQueryEngineCrossTurnState:
    async def test_discover_skill(self, registry, context_builder):
        """discover_skill tracks unique names."""
        engine = make_engine("s10", MockProvider([StreamEvent(type="end", data={})]), registry, context_builder)
        assert engine.discover_skill("grep") is True
        assert engine.discover_skill("grep") is False  # already known
        assert engine.discover_skill("bash") is True
        assert engine.discovered_skill_names == {"grep", "bash"}

    async def test_mark_memory_loaded(self, registry, context_builder):
        """mark_memory_loaded tracks unique paths."""
        engine = make_engine("s11", MockProvider([StreamEvent(type="end", data={})]), registry, context_builder)
        assert engine.mark_memory_loaded(".minicc/memory.md") is True
        assert engine.mark_memory_loaded(".minicc/memory.md") is False  # already loaded

    async def test_permission_denial_tracking(self, registry, context_builder):
        """Denials are recorded as structured objects."""
        provider = MockProvider([
            StreamEvent(type="tool_use", data={"id": "td1", "name": "echo", "input": {"msg": "x"}}),
            StreamEvent(type="end", data={}),
        ])
        engine = make_engine("s12", provider, registry, context_builder)

        # Simulate a denial directly — PermissionDenial is in query_engine module
        from app.engine.query_engine import PermissionDenial as DenialModel
        denial = DenialModel(tool_name="echo", reason="rejected", input_preview="{'msg': 'x'}", turn=0, timestamp="now")
        engine.permission_denials.append(denial)  # type: ignore[attr-defined]

        assert len(engine.permission_denials) == 1
        assert engine.permission_denials[0].tool_name == "echo"
        assert engine.has_recent_denial("echo") is True
        assert engine.has_recent_denial("bash") is False

    async def test_usage_stats(self, registry, context_builder):
        """total_usage tracks across turns."""
        engine = make_engine("s13", MockProvider([StreamEvent(type="end", data={})]), registry, context_builder)
        assert engine.total_usage.input_tokens == 0
        assert engine.total_usage.turn_count == 0

        await _collect(engine, "hi")
        assert engine.total_usage.turn_count >= 1

    async def test_file_cache(self, registry, context_builder):
        """File cache stores read results."""
        engine = make_engine("s14", MockProvider([StreamEvent(type="end", data={})]), registry, context_builder)

        # Simulate a file read cache entry
        from app.engine.query_engine import FileCacheEntry
        import hashlib
        engine._file_cache["test.py"] = FileCacheEntry(
            content="print('hello')",
            total_lines=1,
            hash=hashlib.md5(b"print('hello')").hexdigest(),
        )

        info = engine.get_file_cache_info()
        assert len(info) == 1
        assert info[0]["path"] == "test.py"
        assert info[0]["total_lines"] == 1
