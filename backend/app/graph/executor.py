"""图执行器 — 拓扑排序执行、并行节点、条件路由。

对标 LangGraph graph.invoke() + graph.astream()。
"""

from __future__ import annotations

import asyncio
import logging
import time
from typing import Any, AsyncGenerator

from app.graph.graph import GraphNode, StateGraph

logger = logging.getLogger("minicc.graph.executor")


class NodeResult:
    """单个节点的执行结果。"""
    def __init__(self, node_id: str, status: str = "completed", output: Any = None, error: str | None = None):
        self.node_id = node_id
        self.status = status
        self.output = output
        self.error = error
        self.duration_ms = 0.0


class GraphExecutor:
    """图执行器。

    用法:
        executor = GraphExecutor()
        result = await executor.invoke(graph, {"input": "hello"})
    """

    def __init__(self, tool_registry=None) -> None:
        self._registry = tool_registry
        self._cancel_event = asyncio.Event()
        self.results: dict[str, NodeResult] = {}

    def cancel(self) -> None:
        self._cancel_event.set()

    async def invoke(self, graph: StateGraph, state: dict[str, Any] | None = None) -> dict[str, Any]:
        """同步执行整个图，返回最终状态。"""
        async for _ in self.astream(graph, state):
            pass
        return self._aggregate_state(state or {})

    async def astream(self, graph: StateGraph, state: dict[str, Any] | None = None) -> AsyncGenerator[dict, None]:
        """流式执行，每个节点完成时 yield 事件。"""
        state = state or {}
        completed: set[str] = set()
        running: set[str] = set()
        node_map = {n.id: n for n in graph.nodes}
        adj: dict[str, list[str]] = {n.id: [] for n in graph.nodes}
        for edge in graph.edges:
            adj[edge.source_id].append(edge.target_id)

        # 拓扑排序后的节点列表
        from collections import deque
        in_deg = {n.id: 0 for n in graph.nodes}
        for edge in graph.edges:
            in_deg[edge.target_id] = in_deg.get(edge.target_id, 0) + 1

        ready = deque([nid for nid, d in in_deg.items() if d == 0])

        while ready and not self._cancel_event.is_set():
            # 获取当前批次所有可执行的节点
            batch = list(ready)
            ready.clear()

            # 并行执行
            tasks = []
            for nid in batch:
                tasks.append(self._execute_node(node_map[nid], state))
                running.add(nid)

            finished = await asyncio.gather(*tasks, return_exceptions=True)

            for nid, fut in zip(batch, finished):
                running.discard(nid)
                if isinstance(fut, Exception):
                    self.results[nid] = NodeResult(nid, status="error", error=str(fut))
                    yield {"type": "node_error", "node_id": nid, "error": str(fut)}
                else:
                    result: NodeResult = fut
                    self.results[nid] = result
                    state[nid] = result.output
                    yield {"type": "node_completed", "node_id": nid, "output": result.output}
                    completed.add(nid)

                # 更新下一个批次
                for target in adj.get(nid, []):
                    in_deg[target] -= 1
                    if in_deg[target] == 0:
                        ready.append(target)

        if self._cancel_event.is_set():
            yield {"type": "cancelled"}

    async def _execute_node(self, node: GraphNode, state: dict) -> NodeResult:
        """执行单个节点。"""
        start = time.monotonic()
        result = NodeResult(node_id=node.id)

        try:
            # 模拟执行 — G6 将替换为真实执行器
            await asyncio.sleep(0.1)
            output = f"[{node.node_type}] {node.label} executed"
            result.output = output
            result.status = "completed"
        except Exception as exc:
            result.status = "error"
            result.error = str(exc)

        result.duration_ms = (time.monotonic() - start) * 1000
        return result

    def _aggregate_state(self, state: dict) -> dict:
        """聚合所有节点结果到最终状态。"""
        final = dict(state)
        for nid, result in self.results.items():
            final[nid] = result.output if result.status == "completed" else result.error
        return final
