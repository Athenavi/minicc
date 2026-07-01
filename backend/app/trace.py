"""OpenTelemetry 追踪中间件 — 每个请求/节点执行追踪。"""

from __future__ import annotations

import contextlib
import json
import logging
import time
import uuid
from collections import deque
from typing import Any

logger = logging.getLogger("minicc.trace")

_MAX_SPANS = 1000


class Span:
    """一个追踪跨度。"""
    def __init__(self, name: str, span_type: str = "default", parent_id: str | None = None):
        self.id = uuid.uuid4().hex[:12]
        self.name = name
        self.type = span_type
        self.parent_id = parent_id
        self.started_at = time.time()
        self.ended_at: float | None = None
        self.status = "ok"
        self.attributes: dict[str, Any] = {}
        self.events: list[dict] = []

    def end(self, status: str = "ok") -> None:
        self.ended_at = time.time()
        self.status = status

    def add_event(self, name: str, attrs: dict | None = None) -> None:
        self.events.append({"name": name, "timestamp": time.time(), "attributes": attrs or {}})

    @property
    def duration_ms(self) -> float:
        if self.ended_at:
            return (self.ended_at - self.started_at) * 1000
        return 0

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "name": self.name,
            "type": self.type,
            "parent_id": self.parent_id,
            "started_at": self.started_at,
            "ended_at": self.ended_at,
            "duration_ms": round(self.duration_ms, 2),
            "status": self.status,
            "attributes": self.attributes,
            "events": self.events,
        }


class Tracer:
    """追踪器 — 管理 Span 链。"""

    def __init__(self) -> None:
        self._spans: deque[Span] = deque(maxlen=_MAX_SPANS)
        self._current: Span | None = None

    def start_span(self, name: str, span_type: str = "default") -> Span:
        parent_id = self._current.id if self._current else None
        span = Span(name, span_type, parent_id)
        self._spans.append(span)
        self._current = span
        return span

    def end_span(self, status: str = "ok") -> None:
        if self._current:
            self._current.end(status)
            self._current = None
            # Walk up parent chain
            for s in reversed(self._spans):
                if s.ended_at is None:
                    self._current = s
                    break

    @contextlib.contextmanager
    def span(self, name: str, span_type: str = "default", attrs: dict | None = None):
        s = self.start_span(name, span_type)
        if attrs:
            s.attributes.update(attrs)
        try:
            yield s
        except Exception as exc:
            s.end("error")
            s.add_event("error", {"message": str(exc)})
            raise
        finally:
            if s.ended_at is None:
                s.end("ok")

    def get_trace(self) -> list[dict]:
        return [s.to_dict() for s in self._spans]

    def clear(self) -> None:
        self._spans.clear()
        self._current = None


class TraceMiddleware:
    """FastAPI 中间件 — 为每个请求创建追踪。"""

    def __init__(self, app, tracer: Tracer):
        self.app = app
        self.tracer = tracer

    async def __call__(self, scope, receive, send):
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        span = self.tracer.start_span(f"{scope['method']} {scope['path']}", "http")
        span.attributes["method"] = scope["method"]
        span.attributes["path"] = scope["path"]

        async def wrapped_send(event):
            if event["type"] == "http.response.start":
                span.attributes["status"] = event["status"]
            await send(event)

        try:
            await self.app(scope, receive, wrapped_send)
            span.end("ok")
        except Exception as exc:
            span.end("error")
            span.add_event("error", {"message": str(exc)})
            raise


tracer = Tracer()
