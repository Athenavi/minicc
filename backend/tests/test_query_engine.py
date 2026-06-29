"""Tests: QueryEngine — main loop orchestration."""

from __future__ import annotations

import pytest

pytestmark = pytest.mark.asyncio

from datetime import datetime, timezone
from typing import AsyncGenerator
from unittest.mock import AsyncMock, MagicMock

import pytest
from pydantic import BaseModel

from app.core.context_builder import ContextBuilder, SystemContext
from app.engine.llm_provider import LLMProvider, StreamEvent
from app.engine.query_engine import QueryEngine
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

    async def execute(self, input_data: EchoInput) -> ToolResult:
        return ToolResult(tool_call_id="test", output=input_data.msg)


class FailTool(BaseTool):
    name = "fail"
    description = "always fails"
    input_schema = EchoInput
    permission_level = PermissionLevel.READ

    async def execute(self, input_data: EchoInput) -> ToolResult:
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


# -- Tests --


class TestQueryEngineBasics:
    async def test_empty_response(self, registry, context_builder):
        """Text-only response with no tool calls."""
        provider = MockProvider([StreamEvent(type="end", data={})])
        engine = QueryEngine("s1", provider, registry, context_builder)
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
        engine = QueryEngine("s2", provider, registry, context_builder)
        events = await _collect(engine, "hi")

        texts = [e["payload"]["text"] for e in events if e["type"] == "text_chunk"]
        assert texts == ["Hello", " world"]

    async def test_mutable_messages_accumulate(self, registry, context_builder):
        """Messages accumulate across turns (session state)."""
        provider = MockProvider([StreamEvent(type="end", data={})])
        engine = QueryEngine("s3", provider, registry, context_builder)
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
            StreamEvent(type="end", data={}),
        ])
        engine = QueryEngine("s4", provider, registry, context_builder)
        engine.cancel()
        events = await _collect(engine, "test")
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
        engine = QueryEngine("s5", provider, registry, context_builder)
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
        engine = QueryEngine("s6", provider, registry, context_builder)
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
        engine = QueryEngine("s7", provider, registry, context_builder)
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
        engine = QueryEngine("s8", provider, registry, context_builder)
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
        engine = QueryEngine("s9", provider, registry, context_builder, max_tool_rounds=2)
        events = await _collect(engine, "loop test")
        assert events[-1]["type"] == "message_complete"
