"""Agents API endpoints."""
from __future__ import annotations

from typing import Any

from fastapi import APIRouter
from pydantic import BaseModel

from app.tools.agent import agent_list, agent_dispatch

router = APIRouter(tags=["agents"])


@router.get("/v1/agents")
async def list_agents() -> dict[str, Any]:
    return await agent_list()


class AgentDispatchRequest(BaseModel):
    task: str
    agent_type: str = ""


@router.post("/v1/agents/dispatch")
async def dispatch_agent(body: AgentDispatchRequest) -> dict[str, Any]:
    return await agent_dispatch(task=body.task, agent_type=body.agent_type)
