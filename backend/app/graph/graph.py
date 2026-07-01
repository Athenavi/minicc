"""StateGraph 引擎 — Graph 定义、节点、边、拓扑排序。

对标 LangGraph StateGraph：
- nodes + edges 定义图结构
- Pydantic Schema 定义状态
- 编译时拓扑排序
- 支持条件边、并行节点
"""

from __future__ import annotations

from collections import deque
from typing import Any, Literal, Optional

from pydantic import BaseModel, Field


NodeType = Literal["llm", "tool", "agent", "code", "condition", "input", "output"]


class GraphNode(BaseModel):
    """图节点。"""
    id: str = Field(description="节点唯一标识")
    label: str = Field(description="节点显示名")
    node_type: NodeType = Field(description="节点类型")
    config: dict[str, Any] = Field(default_factory=dict, description="节点配置")

    # 运行时
    parallel_group: int = Field(default=0, description="并行分组 ID（编译时填充）")
    level: int = Field(default=0, description="拓扑层级（编译时填充）")


class GraphEdge(BaseModel):
    """图边。"""
    source_id: str = Field(description="源节点 ID")
    target_id: str = Field(description="目标节点 ID")
    condition: Optional[str] = Field(default=None, description="条件表达式（为空表示无条件）")
    label: Optional[str] = Field(default=None, description="边标签")


class StateGraph(BaseModel):
    """完整状态图定义。"""
    id: str = ""
    name: str = ""
    description: Optional[str] = None
    nodes: list[GraphNode] = Field(default_factory=list)
    edges: list[GraphEdge] = Field(default_factory=list)
    entry_point: str = Field(default="", description="起始节点 ID")
    state_schema: dict[str, Any] = Field(default_factory=dict)


class CompileResult(BaseModel):
    """编译结果。"""
    valid: bool = True
    errors: list[str] = Field(default_factory=list)
    topological_order: list[str] = Field(default_factory=list)
    parallelism_groups: list[list[str]] = Field(default_factory=list)


class GraphBuilder:
    """图构建器。支持链式 API 添加节点和边，编译为 StateGraph。"""

    def __init__(self) -> None:
        self._nodes: dict[str, GraphNode] = {}
        self._edges: list[GraphEdge] = []
        self._entry_point: str | None = None

    def add_node(self, id: str, label: str, node_type: NodeType, config: dict | None = None) -> "GraphBuilder":
        self._nodes[id] = GraphNode(id=id, label=label, node_type=node_type, config=config or {})
        return self

    def add_edge(self, source_id: str, target_id: str, condition: str | None = None, label: str | None = None) -> "GraphBuilder":
        self._edges.append(GraphEdge(source_id=source_id, target_id=target_id, condition=condition, label=label))
        return self

    def set_entry_point(self, node_id: str) -> "GraphBuilder":
        if node_id not in self._nodes:
            raise ValueError(f"Entry point node '{node_id}' not found")
        self._entry_point = node_id
        return self

    def compile(self) -> tuple[StateGraph, CompileResult]:
        """编译图：校验 + 拓扑排序 + 并行分组。"""
        errors: list[str] = []

        if not self._nodes:
            errors.append("Graph has no nodes")

        entry = self._entry_point or (list(self._nodes.keys())[0] if self._nodes else "")
        if entry and entry not in self._nodes:
            errors.append(f"Entry point '{entry}' not found")

        # 校验所有边的 source/target 存在
        for edge in self._edges:
            if edge.source_id not in self._nodes:
                errors.append(f"Edge source '{edge.source_id}' not found")
            if edge.target_id not in self._nodes:
                errors.append(f"Edge target '{edge.target_id}' not found")

        if errors:
            return StateGraph(), CompileResult(valid=False, errors=errors)

        # 拓扑排序 (Kahn 算法)
        adj: dict[str, list[str]] = {n: [] for n in self._nodes}
        in_deg: dict[str, int] = {n: 0 for n in self._nodes}
        for edge in self._edges:
            adj[edge.source_id].append(edge.target_id)
            in_deg[edge.target_id] = in_deg.get(edge.target_id, 0) + 1

        queue: deque[str] = deque([n for n, d in in_deg.items() if d == 0])
        topo_order: list[str] = []
        while queue:
            node = queue.popleft()
            topo_order.append(node)
            for neighbor in adj[node]:
                in_deg[neighbor] -= 1
                if in_deg[neighbor] == 0:
                    queue.append(neighbor)

        if len(topo_order) != len(self._nodes):
            errors.append("Graph contains a cycle")
            return StateGraph(), CompileResult(valid=False, errors=errors)

        # 计算拓扑层级 + 并行分组
        levels: dict[str, int] = {n: 0 for n in self._nodes}
        for node in topo_order:
            for edge in self._edges:
                if edge.source_id == node:
                    levels[edge.target_id] = max(levels[edge.target_id], levels[node] + 1)

        # 并行分组：同一层级且无依赖关系的节点
        groups: dict[int, list[str]] = {}
        for node_id, lv in levels.items():
            groups.setdefault(lv, []).append(node_id)

        # 更新节点属性
        node_list = []
        for nid in topo_order:
            node = self._nodes[nid].model_copy()
            node.level = levels[nid]
            node_list.append(node)

        graph = StateGraph(
            name="compiled_graph",
            nodes=node_list,
            edges=self._edges,
            entry_point=entry,
        )

        return graph, CompileResult(
            valid=True,
            topological_order=topo_order,
            parallelism_groups=[g for _, g in sorted(groups.items())],
        )


def register_graph_tools(registry) -> None:
    """注册 Graph 工具（G6 实现）。"""
    pass
