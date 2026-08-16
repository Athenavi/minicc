"""Workflow engine（Python 端）：基于 LangGraph 的 DAG 执行引擎。

目标：
- 实现 Go 侧 `internal/graph/executor.go` 的等价 Python 执行能力
- 支持从 StateGraph JSON 编译并运行 workflow
- 节点类型：input / llm / tool / condition / output
"""
from __future__ import annotations

import time
import uuid
import logging
from dataclasses import dataclass, field
from typing import Any

from langgraph.graph import StateGraph

from app.gateway.router import GatewayRouter
from app.tools.registry import registry as tool_registry

logger = logging.getLogger(__name__)


@dataclass
class NodeResult:
    node_id: str
    status: str  # completed / error / skipped
    output: str = ""
    error: str = ""
    duration_ms: int = 0


@dataclass
class WorkflowInstance:
    instance_id: str
    graph_name: str
    state: dict[str, Any] = field(default_factory=dict)
    results: dict[str, NodeResult] = field(default_factory=dict)
    status: str = "running"
    started_at: float = field(default_factory=time.time)
    finished_at: float | None = None


_instances: dict[str, WorkflowInstance] = {}
_MAX_INSTANCES = 500  # 最大实例数，防止内存泄漏
_instance_order: list[str] = []  # FIFO 顺序


def _topological_sort(nodes: list[dict], edges: list[dict]) -> list[dict]:
    """按 DAG 拓扑顺序排序节点"""
    from collections import defaultdict, deque

    in_degree: dict[str, int] = {n["id"]: 0 for n in nodes}
    adj: dict[str, list[str]] = defaultdict(list)
    for e in edges:
        adj[e["source_id"]].append(e["target_id"])
        in_degree[e["target_id"]] = in_degree.get(e["target_id"], 0) + 1

    node_map = {n["id"]: n for n in nodes}
    queue = deque([n for n in nodes if in_degree.get(n["id"], 0) == 0])
    result: list[dict] = []
    while queue:
        node = queue.popleft()
        result.append(node)
        for neighbor in adj[node["id"]]:
            in_degree[neighbor] -= 1
            if in_degree[neighbor] == 0 and neighbor in node_map:
                queue.append(node_map[neighbor])
    # 添加未在 edges 中出现的孤立节点
    seen = {n["id"] for n in result}
    for n in nodes:
        if n["id"] not in seen:
            result.append(n)
    return result
_MAX_INSTANCES = 500  # 最大实例数，防止内存泄漏
_instance_order: list[str] = []  # FIFO 顺序


def _eval_condition(expression: str, text: str) -> bool:
    if not expression:
        return bool(text)
    return expression.lower() in text.lower()


def _build_langgraph(graph_json: dict, gateway: GatewayRouter) -> Any:
    nodes = graph_json.get("nodes", [])
    edges = graph_json.get("edges", [])
    entry_point = graph_json.get("entry_point", "")

    sg = StateGraph(dict)

    node_map = {n["id"]: n for n in nodes}

    async def _input_node(state: dict) -> dict:
        node_id = state["__current_node__"]
        node = node_map[node_id]
        if node_id in state:
            state["__output__"] = str(state[node_id])
        elif "input" in state:
            state["__output__"] = str(state["input"])
        else:
            state["__output__"] = f"[input] {node.get('label', node_id)}"
        return state

    async def _llm_node(state: dict) -> dict:
        node_id = state["__current_node__"]
        node = node_map[node_id]
        config = node.get("config", {})
        prompt = config.get("prompt", state.get("__output__", ""))
        model = config.get("model", "")

        messages = [{"role": "user", "content": prompt}]
        text = ""
        async for chunk in gateway.chat_stream(messages=messages, model=model or "gpt-4o-mini"):
            if chunk.content:
                text += chunk.content
        state["__output__"] = text
        return state

    async def _tool_node(state: dict) -> dict:
        node_id = state["__current_node__"]
        node = node_map[node_id]
        config = node.get("config", {})
        name = config.get("tool_name", node.get("label", ""))
        params = config.get("params", {})
        result = await tool_registry.execute(name, params)
        state["__output__"] = str(result)
        return state

    async def _condition_node(state: dict) -> dict:
        node_id = state["__current_node__"]
        node = node_map[node_id]
        config = node.get("config", {})
        expr = config.get("expression", "")
        ref = config.get("input", "")
        text = state.get("__output__", "")
        if isinstance(ref, str) and ref.startswith("$"):
            key = ref[1:]
            text = str(state.get(key, text))
        matched = _eval_condition(expr, text)
        state["__output__"] = "true" if matched else "false"
        return state

    async def _output_node(state: dict) -> dict:
        return state

    node_fn = {
        "input": _input_node,
        "llm": _llm_node,
        "tool": _tool_node,
        "condition": _condition_node,
        "output": _output_node,
    }

    for node in nodes:
        sg.add_node(node["id"], node_fn.get(node["node_type"], _input_node))

    for edge in edges:
        sg.add_edge(edge["source_id"], edge["target_id"])

    if entry_point:
        sg.set_entry_point(entry_point)

    return sg.compile()


async def run_workflow(graph_json: dict, gateway: GatewayRouter, initial_state: dict[str, Any] | None = None) -> WorkflowInstance:
    instance = WorkflowInstance(
        instance_id=f"wf_{uuid.uuid4().hex[:10]}",
        graph_name=graph_json.get("name", "workflow"),
        state=dict(initial_state or {}),
    )
    _instances[instance.instance_id] = instance
    # FIFO 淘汰：超过最大数量时删除最旧实例
    _instance_order.append(instance.instance_id)
    if len(_instances) > _MAX_INSTANCES:
        oldest = _instance_order.pop(0)
        _instances.pop(oldest, None)

    try:
        app = _build_langgraph(graph_json, gateway)
        state = dict(instance.state)
        # 按 DAG 拓扑顺序执行节点（而非 nodes 列表顺序）
        for node in _topological_sort(graph_json.get("nodes", []), graph_json.get("edges", [])):
            state["__current_node__"] = node["id"]
            state = await app.ainvoke(state)
            instance.results[node["id"]] = NodeResult(
                node_id=node["id"],
                status="completed",
                output=str(state.get("__output__", "")),
            )
            instance.state.update(state)
        instance.status = "completed"
    except Exception as e:
        logger.error("workflow execution failed: %s", e)
        instance.status = "error"
    finally:
        instance.finished_at = time.time()

    return instance


def get_instance(instance_id: str) -> WorkflowInstance | None:
    return _instances.get(instance_id)
