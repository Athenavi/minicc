"""Event types — Reasonix-style typed event system.

Events flow from QueryEngine → Broadcaster → SSE → Frontend.
Each event has a 'type' field that identifies its kind.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Optional


# ── Event kinds (matching Reasonix event system) ──

TURN_STARTED = "turn_started"
REASONING = "reasoning"
TEXT = "text"
MESSAGE = "message"
TOOL_DISPATCH = "tool_dispatch"
TOOL_PROGRESS = "tool_progress"
TOOL_RESULT = "tool_result"
USAGE = "usage"
NOTICE = "notice"
PHASE = "phase"
APPROVAL_REQUEST = "approval_request"
ASK_REQUEST = "ask_request"
TURN_DONE = "turn_done"
COMPACTION_STARTED = "compaction_started"
COMPACTION_DONE = "compaction_done"
ERROR = "error"


@dataclass
class MiniCCEvent:
    """A typed event in the MiniCC event system.

    Matches Reasonix's Event struct pattern:
    - kind: string discriminator
    - data: dict with kind-specific payload
    """
    kind: str
    data: dict[str, Any] = field(default_factory=dict)


# ── Broadcaster (fan-out with backpressure) ──
#
# Matches Reasonix's broadcaster.go:
# - Subscribers get a buffered channel
# - Non-blocking send (dropped if subscriber is slow)
# - Marshal once, fan to all


import asyncio
import json
import logging
from typing import Callable

logger = logging.getLogger("minicc.event")


class Broadcaster:
    """Fan-out event bus.

    - Subscribe() returns a receive-only channel
    - Emit() sends to all subscribers (non-blocking)
    - Slow clients miss events instead of blocking the agent
    """

    def __init__(self, buffer_size: int = 64) -> None:
        self._subscribers: dict[int, asyncio.Queue] = {}
        self._counter = 0
        self._lock = asyncio.Lock()

    async def subscribe(self) -> tuple[int, asyncio.Queue]:
        """Register a subscriber. Returns (id, queue)."""
        async with self._lock:
            self._counter += 1
            q: asyncio.Queue = asyncio.Queue(maxsize=64)
            self._subscribers[self._counter] = q
            return self._counter, q

    def unsubscribe(self, sub_id: int) -> None:
        """Remove a subscriber."""
        self._subscribers.pop(sub_id, None)

    async def emit(self, event: MiniCCEvent) -> None:
        """Fan-out event to all subscribers (non-blocking)."""
        async with self._lock:
            dead: list[int] = []
            for sid, q in self._subscribers.items():
                try:
                    q.put_nowait(event)
                except asyncio.QueueFull:
                    dead.append(sid)
            for sid in dead:
                self._subscribers.pop(sid, None)
                logger.debug("Dropped slow subscriber: %d", sid)


# Global broadcaster instance
broadcaster = Broadcaster()
