"""节点级流式输出 — 每个节点执行时推送 SSE 事件。"""

from __future__ import annotations

from typing import Any, AsyncGenerator

from app.core.events import MiniCCEvent, broadcaster

EVENT_GRAPH_START = "graph_start"
EVENT_NODE_START = "node_start"
EVENT_NODE_DONE = "node_done"
EVENT_NODE_ERROR = "node_error"
EVENT_NODE_OUTPUT = "node_output"
EVENT_GRAPH_DONE = "graph_done"


async def emit_node_start(graph_id: str, node_id: str, label: str, node_type: str) -> None:
    await broadcaster.emit(MiniCCEvent(EVENT_NODE_START, {
        "graph_id": graph_id, "node_id": node_id, "label": label, "type": node_type,
    }))


async def emit_node_done(graph_id: str, node_id: str, output: Any, duration_ms: float) -> None:
    await broadcaster.emit(MiniCCEvent(EVENT_NODE_DONE, {
        "graph_id": graph_id, "node_id": node_id, "duration_ms": duration_ms,
    }))


async def emit_node_error(graph_id: str, node_id: str, error: str) -> None:
    await broadcaster.emit(MiniCCEvent(EVENT_NODE_ERROR, {
        "graph_id": graph_id, "node_id": node_id, "error": error,
    }))


async def emit_node_output(graph_id: str, node_id: str, output: str) -> None:
    await broadcaster.emit(MiniCCEvent(EVENT_NODE_OUTPUT, {
        "graph_id": graph_id, "node_id": node_id, "output": output[:1000],
    }))
