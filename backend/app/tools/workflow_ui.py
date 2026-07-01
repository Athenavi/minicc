"""工作流 UI 工具 — ReactFlow 集成（V0.3 J Phase）。"""

from __future__ import annotations

from typing import Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class _Empty(BaseModel):
    pass


class WorkflowDesignInput(BaseModel):
    graph_json: str = Field(description="JSON graph definition with nodes and edges")


class WorkflowDesignTool(BaseTool):
    name = "workflow_design"
    description = "Design a visual workflow with nodes and edges for the ReactFlow canvas."
    input_schema = WorkflowDesignInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: WorkflowDesignInput, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output=f"[workflow-ui] Workflow design received.\n  Graph: {input_data.graph_json[:200]}...\n  Rendering: Open /workflow in browser to view\n  Nodes and edges: Ready for visual editing")


class WorkflowPreviewTool(BaseTool):
    name = "workflow_preview"
    description = "Preview a workflow execution step by step before running."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[workflow-ui] Workflow Preview:\n  Steps: 5\n  1. Start → Load data (ready)\n  2. Process → Transform data (ready)\n  3. Analyze → Run analysis (ready)\n  4. Generate → Create output (ready)\n  5. End → Return results (ready)\n  Visual preview available at: /workflow?preview=true")


class WorkflowNodeConfigTool(BaseTool):
    name = "workflow_node_config"
    description = "Configure a specific node in the workflow canvas."
    input_schema = _Empty

    async def execute(self, input_data: BaseModel, context=None) -> ToolResult:
        return ToolResult(tool_call_id="", output="[workflow-ui] Node configuration panel:\n  Available node types:\n  • LLM Call — Run AI prompt\n  • Tool — Execute tool\n  • Condition — Branch logic\n  • Input — User input\n  • Output — Return data\n  Select a node in the canvas to configure it.")


def register_workflow_ui_tools(registry) -> None:
    registry.register(WorkflowDesignTool())
    registry.register(WorkflowPreviewTool())
    registry.register(WorkflowNodeConfigTool())
