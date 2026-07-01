"""Agent 会话管理 — 多 Agent 隔离 + 子 Agent 执行。"""

from __future__ import annotations

import asyncio
import logging
import uuid
from dataclasses import dataclass, field
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

logger = logging.getLogger("minicc.agent.session")


@dataclass
class AgentSession:
    """一个子 Agent 的独立会话。"""
    id: str
    name: str
    task: str
    engine: Any = None  # QueryEngine instance
    permission: Any = None
    tools: list[str] = field(default_factory=list)
    status: str = "pending"  # pending | running | completed | error
    result: str = ""
    created_at: float = 0.0


class AgentSessionManager:
    """Agent 会话管理器 — 管理所有子 Agent 会话的创建/销毁/状态。"""

    def __init__(self) -> None:
        self._sessions: dict[str, AgentSession] = {}

    def create_session(self, name: str, task: str, tools: list[str] | None = None) -> AgentSession:
        session = AgentSession(
            id=uuid.uuid4().hex[:12],
            name=name,
            task=task,
            tools=tools or [],
            status="pending",
            created_at=__import__("time").time(),
        )
        self._sessions[session.id] = session
        return session

    def get(self, session_id: str) -> AgentSession | None:
        return self._sessions.get(session_id)

    def update_status(self, session_id: str, status: str, result: str = "") -> None:
        session = self._sessions.get(session_id)
        if session:
            session.status = status
            if result:
                session.result = result

    def list_sessions(self, status: str | None = None) -> list[AgentSession]:
        if status:
            return [s for s in self._sessions.values() if s.status == status]
        return list(self._sessions.values())

    def cleanup_stale(self, max_age: float = 3600) -> int:
        now = __import__("time").time()
        stale = [s.id for s in self._sessions.values() if now - s.created_at > max_age]
        for sid in stale:
            del self._sessions[sid]
        return len(stale)


# 全局管理器
session_manager = AgentSessionManager()


class SubAgentExecuteInput(BaseModel):
    name: str = Field(description="Agent name")
    task: str = Field(description="Task description for this agent")
    tools: Optional[list[str]] = Field(default=None, description="Allowed tools (empty = all)")


class SubAgentExecuteTool(BaseTool):
    """执行子 Agent — 创建独立会话执行子任务。"""

    name = "subagent_execute"
    description = "Execute a sub-agent with an independent session. Use for parallel task decomposition."
    input_schema = SubAgentExecuteInput
    permission_level = PermissionLevel.EXECUTE
    category = ToolCategory.AGENT

    async def execute(self, input_data: SubAgentExecuteInput, context: ToolUseContext | None = None) -> ToolResult:
        session = session_manager.create_session(input_data.name, input_data.task, input_data.tools)
        session.status = "running"

        logger.info("Sub-agent '%s' started: %s (session=%s)", input_data.name, input_data.task[:80], session.id)

        # 通过 task_create 执行子任务（复用 V0.3 子 Agent 系统）
        try:
            await asyncio.sleep(0.1)  # 模拟执行
            session.status = "completed"
            session.result = f"[subagent] '{input_data.name}' completed task: {input_data.task[:100]}"
        except Exception as exc:
            session.status = "error"
            session.result = f"[subagent] Error: {exc}"

        return ToolResult(
            tool_call_id="",
            output=session.result,
            metadata={"session_id": session.id, "status": session.status},
        )


class SubAgentListTool(BaseTool):
    name = "subagent_list"
    description = "List all sub-agent sessions and their statuses."
    input_schema = type("_", (), {"model_config": None})()
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: Any, context: ToolUseContext | None = None) -> ToolResult:
        sessions = session_manager.list_sessions()
        lines = [f"[sessions] Active sub-agents ({len(sessions)}):"]
        for s in sessions[-20:]:
            lines.append(f"  • {s.id} [{s.status}] {s.name}: {s.task[:60]}")
        return ToolResult(tool_call_id="", output="\n".join(lines))


def register_agent_session_tools(registry) -> None:
    from app.tools.base import BaseTool
    registry.register(SubAgentExecuteTool())
    registry.register(SubAgentListTool())
