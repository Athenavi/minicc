"""工作流工具 — 提供给 AI 的工作流操作接口。"""

from __future__ import annotations

from pydantic import BaseModel, Field

from app.automator.dsl import WorkflowDefinition
from app.automator.workflow import WorkflowExecutor
from app.models.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext


class WorkflowRunInput(BaseModel):
    name: str = Field(description="Workflow name")
    steps: list[dict] = Field(description="List of workflow step definitions")


class WorkflowRunTool(BaseTool):
    name = "workflow_run"
    description = "Run a multi-step automation workflow defined in YAML/JSON."
    input_schema = WorkflowRunInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: WorkflowRunInput, context: ToolUseContext | None = None) -> ToolResult:
        from app.automator.dsl import WorkflowDefinition, WorkflowStep
        steps = []
        for s in input_data.steps:
            if isinstance(s, dict):
                steps.append(WorkflowStep(**s))
            else:
                steps.append(s)
        wf = WorkflowDefinition(name=input_data.name, steps=steps)
        executor = WorkflowExecutor()
        result = await executor.execute(wf)
        return ToolResult(
            tool_call_id="",
            output=result.output,
            metadata={"status": result.status, "duration": result.duration_seconds, "id": result.workflow_id},
        )


class WorkflowCancelInput(BaseModel):
    workflow_id: str = Field(description="Workflow ID to cancel")


class WorkflowCancelTool(BaseTool):
    name = "workflow_cancel"
    description = "Cancel a running workflow."
    input_schema = WorkflowCancelInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: WorkflowCancelInput, context: ToolUseContext | None = None) -> ToolResult:
        return ToolResult(tool_call_id="", output=f"[workflow] Cancel requested: {input_data.workflow_id}")


def register_workflow_tools(registry) -> None:
    registry.register(WorkflowRunTool())
    registry.register(WorkflowCancelTool())
