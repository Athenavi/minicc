"""Graph 工具 — 提供给 AI 的 StateGraph 操作接口。"""

from __future__ import annotations

import json
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.graph.graph import GraphBuilder, NodeType, StateGraph
from app.graph.executor import GraphExecutor
from app.graph.checkpoint import Checkpointer
from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class GraphCreateInput(BaseModel):
    name: str = Field(description="Graph name")
    entry_point: str = Field(description="Entry node ID")
    nodes: list[dict] = Field(description="List of nodes: [{id, label, node_type, config}]")
    edges: list[dict] = Field(description="List of edges: [{source_id, target_id, condition?}]")


class GraphCreateTool(BaseTool):
    name = "graph_create"
    description = "Create a StateGraph workflow from node/edge definitions."
    input_schema = GraphCreateInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: GraphCreateInput, context: ToolUseContext | None = None) -> ToolResult:
        builder = GraphBuilder()
        for n in input_data.nodes:
            builder.add_node(n["id"], n.get("label", n["id"]), n.get("node_type", "llm"), n.get("config"))
        for e in input_data.edges:
            builder.add_edge(e["source_id"], e["target_id"], e.get("condition"), e.get("label"))
        builder.set_entry_point(input_data.entry_point)

        graph, result = builder.compile()
        if not result.valid:
            return ToolResult(tool_call_id="", output=f"[graph] Compile errors:\n" + "\n".join(result.errors), is_error=True)

        output = f"[graph] Created: {input_data.name}\n"
        output += f"  Nodes: {len(graph.nodes)}\n"
        output += f"  Edges: {len(graph.edges)}\n"
        output += f"  Topo order: {' → '.join(result.topological_order)}\n"
        output += f"  Parallel groups: {len(result.parallelism_groups)}"
        return ToolResult(tool_call_id="", output=output, metadata={"node_count": len(graph.nodes)})


class GraphRunInput(BaseModel):
    name: str = Field(description="Graph name")
    entry_point: str = Field(description="Entry node ID")
    nodes: list[dict] = Field(description="Node definitions")
    edges: list[dict] = Field(description="Edge definitions")
    initial_state: Optional[dict] = Field(default=None)


class GraphRunTool(BaseTool):
    name = "graph_run"
    description = "Create and execute a StateGraph workflow in one step."
    input_schema = GraphRunInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: GraphRunInput, context: ToolUseContext | None = None) -> ToolResult:
        builder = GraphBuilder()
        for n in input_data.nodes:
            builder.add_node(n["id"], n.get("label", n["id"]), n.get("node_type", "llm"), n.get("config"))
        for e in input_data.edges:
            builder.add_edge(e["source_id"], e["target_id"], e.get("condition"), e.get("label"))
        builder.set_entry_point(input_data.entry_point)

        graph, result = builder.compile()
        if not result.valid:
            return ToolResult(tool_call_id="", output=f"[graph] Compile errors:\n" + "\n".join(result.errors), is_error=True)

        executor = GraphExecutor()
        state = await executor.invoke(graph, input_data.initial_state or {})

        output = f"[graph] Executed: {input_data.name}\n  Nodes: {len(graph.nodes)}\n  Status: completed\n"
        for nid in result.topological_order[:5]:
            output += f"  [{nid}] → {str(state.get(nid, ''))[:80]}\n"
        return ToolResult(tool_call_id="", output=output, metadata={"status": "completed"})


def register_graph_tools(registry) -> None:
    registry.register(GraphCreateTool())
    registry.register(GraphRunTool())
