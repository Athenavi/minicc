"""Agents API endpoints."""
from __future__ import annotations

from typing import Any

from fastapi import APIRouter
from pydantic import BaseModel

from app.tools.agent import agent_list

router = APIRouter(tags=["agents"])


@router.get("/v1/agents")
async def list_agents() -> dict[str, Any]:
    """Agent 列表（页面主链路已由 Go 的 DB agents 表提供；此端点保留给工具链）。"""
    return await agent_list()


class AgentDispatchRequest(BaseModel):
    task: str
    agent_type: str = ""
    # 完整 Agent 配置（Go 从 DB agents 表读取后传入）→ SubAgent 真执行
    name: str = ""
    description: str = ""
    system_prompt: str = ""
    tools: list[dict] = []
    model: str = ""
    max_turns: int = 5
    max_tokens: int = 4096
    temperature: float = 0.7
    tenant_id: str = ""
    session_id: str = ""


@router.post("/v1/agents/dispatch")
async def dispatch_agent(body: AgentDispatchRequest) -> dict[str, Any]:
    """
    派发 Agent 任务。

    - 携带 system_prompt（Go 传入 DB 配置）：用 SubAgent 执行完整 agent loop
      （LLM 流式 + 工具调用 + 多轮），返回真实结果。
    - 否则（工具链调用）：回退到内存 registry 的假派发。
    """
    if body.system_prompt.strip():
        # 标记用户活跃（驱动 MCP 插件轮询范围）
        from app.main import touch_user
        touch_user(body.tenant_id)

        # 延迟导入：避免 app.main ↔ app.api 循环依赖
        from app.agent.multi_agent import SubAgent
        from app.main import get_gateway

        try:
            gateway = await get_gateway()
        except RuntimeError:
            gateway = None
        if gateway is None:
            return {"success": False, "error": "LLM gateway not initialized", "output": ""}

        agent = SubAgent(
            name=body.name or body.agent_type or "agent",
            description=body.description or "",
            system_prompt=body.system_prompt,
            tools=body.tools or None,
            gateway=gateway,
            model=body.model or "deepseek-chat",
            max_turns=body.max_turns or 5,
            max_tokens=body.max_tokens or 4096,
            temperature=body.temperature or 0.7,
        )
        result = await agent.run(
            task=body.task,
            context={"session_id": body.session_id} if body.session_id else None,
            tenant_id=body.tenant_id,
        )
        return {
            "success": result.success,
            "output": result.output,
            "error": result.error,
            "tool_calls": result.tool_calls,
            "token_usage": result.token_usage,
            "duration": result.duration,
            "session_id": body.session_id,
        }

    from app.tools.agent import agent_dispatch

    return await agent_dispatch(task=body.task, agent_type=body.agent_type)
