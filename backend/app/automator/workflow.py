"""工作流执行器 — 解析 DSL 并按步骤调用工具。"""

from __future__ import annotations

import asyncio
import logging
import uuid
from datetime import datetime, timezone
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.automator.dsl import (
    VariableResolver,
    WorkflowCondition,
    WorkflowDefinition,
    WorkflowLoop,
    WorkflowStep,
)

logger = logging.getLogger("minicc.automator")


class WorkflowContext:
    """工作流执行上下文。存储变量、步骤结果、状态。"""

    def __init__(self, workflow: WorkflowDefinition, env: dict[str, str] | None = None) -> None:
        self.workflow_id = uuid.uuid4().hex[:12]
        self.workflow = workflow
        self.steps: dict[str, Any] = {}
        self.variables: dict[str, Any] = dict(workflow.variables)
        self.env = env or {}
        self.status = "pending"
        self.error: str | None = None
        self.start_time = datetime.now(timezone.utc)
        self.end_time: datetime | None = None

    def get_resolver_context(self) -> dict:
        return {
            "steps": self.steps,
            "env": self.env,
            "variables": self.variables,
        }


class WorkflowResult(BaseModel):
    """工作流执行结果。"""
    workflow_id: str
    status: str
    steps: dict[str, dict]
    error: Optional[str] = None
    duration_seconds: float = 0.0
    output: str = ""


class WorkflowExecutor:
    """工作流执行器。解析 DSL 并按步骤执行。"""

    def __init__(self, tool_registry=None) -> None:
        self._registry = tool_registry
        self._cancel_event = asyncio.Event()

    def cancel(self) -> None:
        self._cancel_event.set()

    async def execute(self, workflow: WorkflowDefinition, env: dict[str, str] | None = None) -> WorkflowResult:
        ctx = WorkflowContext(workflow, env)
        output_lines = [f"[workflow] Starting: {workflow.name}"]

        try:
            ctx.status = "running"
            for step_entry in workflow.steps:
                if self._cancel_event.is_set():
                    ctx.status = "cancelled"
                    break

                if isinstance(step_entry, WorkflowStep):
                    result = await self._execute_step(step_entry, ctx, output_lines)
                elif isinstance(step_entry, WorkflowCondition):
                    result = await self._execute_condition(step_entry, ctx, output_lines)
                elif isinstance(step_entry, WorkflowLoop):
                    result = await self._execute_loop(step_entry, ctx, output_lines)
                else:
                    result = {"error": f"Unknown step type: {type(step_entry)}"}

                if result and "error" in result:
                    ctx.error = result["error"]
                    ctx.status = "failed"
                    break

            if ctx.status == "running":
                ctx.status = "completed"

        except Exception as exc:
            ctx.status = "failed"
            ctx.error = str(exc)
            output_lines.append(f"  ❌ Workflow error: {exc}")

        ctx.end_time = datetime.now(timezone.utc)
        duration = (ctx.end_time - ctx.start_time).total_seconds()

        return WorkflowResult(
            workflow_id=ctx.workflow_id,
            status=ctx.status,
            steps={k: {"status": "ok" if "error" not in str(v) else "error"} for k, v in ctx.steps.items()},
            error=ctx.error,
            duration_seconds=duration,
            output="\n".join(output_lines),
        )

    async def _execute_step(self, step: WorkflowStep, ctx: WorkflowContext, output: list[str]) -> dict:
        resolver = VariableResolver(ctx.get_resolver_context())
        params = resolver.resolve_params(step.params)
        output.append(f"  ▶ {step.id}: {step.tool}")

        if self._registry:
            tool = self._registry.get(step.tool.replace(".", "_"))
            if tool:
                try:
                    from pydantic import BaseModel as PBM
                    # Create a dynamic input model
                    input_obj = type("Input", (PBM,), {})()
                    for k, v in params.items():
                        setattr(input_obj, k, v)
                    result = await tool.execute(input_obj)
                    ctx.steps[step.id] = {
                        "result": result.output[:1000],
                        "success": not result.is_error,
                    }
                    output.append(f"    ✅ Result: {result.output[:100]}")
                    return {}
                except Exception as exc:
                    err = f"Tool error: {exc}"
                    ctx.steps[step.id] = {"error": err, "success": False}
                    output.append(f"    ❌ {err}")
                    return {"error": err} if step.retry == 0 else await self._retry(step, ctx, output)

        # No registry — simulate
        ctx.steps[step.id] = {"result": f"Simulated: {step.tool}", "success": True}
        return {}

    async def _retry(self, step: WorkflowStep, ctx: WorkflowContext, output: list[str]) -> dict:
        for attempt in range(step.retry):
            output.append(f"    🔄 Retry {attempt + 1}/{step.retry}")
            result = await self._execute_step(step, ctx, output)
            if "error" not in result:
                return result
        return {"error": f"Failed after {step.retry} retries"}

    async def _execute_condition(self, cond: WorkflowCondition, ctx: WorkflowContext, output: list[str]) -> dict:
        resolver = VariableResolver(ctx.get_resolver_context())
        expr = resolver.resolve(cond.condition)
        # Simple evaluation: if expr contains "true" or "success" consider it truthy
        is_true = expr.lower() in ("true", "yes", "ok", "success")
        output.append(f"  ▶ Condition: {cond.condition} → {'✅' if is_true else '❌'}")
        branch = cond.then if is_true else (cond.else_ or [])
        for step_entry in branch:
            r = await self._execute_step(step_entry, ctx, output)
            if "error" in r:
                return r
        return {}

    async def _execute_loop(self, loop: WorkflowLoop, ctx: WorkflowContext, output: list[str]) -> dict:
        resolver = VariableResolver(ctx.get_resolver_context())
        items = resolver.resolve(loop.over)
        if isinstance(items, str):
            items = items.split(",")
        output.append(f"  ▶ Loop over {loop.over} ({len(items)} items)")
        for item in items:
            ctx.variables[loop.as_] = item
            for step_entry in loop.steps:
                r = await self._execute_step(step_entry, ctx, output)
                if "error" in r:
                    return r
        return {}
