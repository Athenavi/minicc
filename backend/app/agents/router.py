"""路由 Agent + Agent 通信总线 — 多 Agent 协调。"""

from __future__ import annotations

import asyncio
import json
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Optional

from pydantic import BaseModel, Field

from app.core.permission import PermissionLevel
from app.models.tool import ToolResult
from app.tools.base import BaseTool, ToolCategory, ToolUseContext

class _Empty(BaseModel):
    pass

router = None  # 全局 router 实例


@dataclass
class AgentMessage:
    """Agent 间消息。"""
    from_agent: str
    to_agent: str
    content: str
    msg_type: str = "text"
    id: str = ""
    timestamp: str = ""


class AgentRegistry:
    """Agent 注册表 — 管理可用 Agent 类型和路由。"""

    def __init__(self) -> None:
        self._agents: dict[str, dict] = {}

    def register(self, agent_type: str, name: str, description: str, handler=None) -> None:
        self._agents[agent_type] = {
            "name": name,
            "description": description,
            "handler": handler,
            "type": agent_type,
        }

    def get(self, agent_type: str) -> Optional[dict]:
        return self._agents.get(agent_type)

    def list_agents(self) -> list[dict]:
        return [{"type": k, "name": v["name"], "description": v["description"]} for k, v in self._agents.items()]

    def route(self, task: str) -> Optional[str]:
        """根据任务描述路由到最合适的 Agent 类型。"""
        task_lower = task.lower()
        # 简单关键词路由
        if any(w in task_lower for w in ["代码", "code", "写", "修复", "debug", "文件", "python"]):
            return "code"
        if any(w in task_lower for w in ["搜索", "search", "知识", "文档", "查询", "rag"]):
            return "knowledge"
        if any(w in task_lower for w in ["浏览器", "browser", "web", "点击", "填写", "表单"]):
            return "rpa"
        if any(w in task_lower for w in ["mcp", "api", "工具", "tool"]):
            return "tool"
        return "code"  # default


agent_registry = AgentRegistry()


# 注册默认 Agent
agent_registry.register("code", "代码 Agent", "编写、修改、分析代码")
agent_registry.register("knowledge", "知识 Agent", "检索知识库和文档")
agent_registry.register("rpa", "RPA Agent", "控制浏览器和桌面应用")
agent_registry.register("tool", "工具 Agent", "调用 MCP 和外部 API")


class AgentDispatchInput(BaseModel):
    task: str = Field(description="Task description to dispatch")
    agent_type: Optional[str] = Field(default=None, description="Target agent type (auto-routed if empty)")


class AgentDispatchTool(BaseTool):
    name = "agent_dispatch"
    description = "Dispatch a task to the most suitable agent. Auto-routes based on task description."
    input_schema = AgentDispatchInput
    permission_level = PermissionLevel.WRITE
    category = ToolCategory.AGENT

    async def execute(self, input_data: AgentDispatchInput, context: ToolUseContext | None = None) -> ToolResult:
        atype = input_data.agent_type or agent_registry.route(input_data.task)
        agent_info = agent_registry.get(atype)
        if not agent_info:
            return ToolResult(tool_call_id="", output=f"[router] No agent found for: {atype}", is_error=True)

        return ToolResult(
            tool_call_id="",
            output=f"[router] Dispatched to '{agent_info['name']}'\n"
                   f"  Agent: {atype}\n  Task: {input_data.task}\n  (execute via sub-agent)",
            metadata={"agent_type": atype, "task": input_data.task},
        )


class AgentListTool(BaseTool):
    name = "agent_list"
    description = "List all available agents and their capabilities."
    input_schema = _Empty
    permission_level = PermissionLevel.READ
    category = ToolCategory.AGENT

    async def execute(self, input_data: BaseModel, context: ToolUseContext | None = None) -> ToolResult:
        agents = agent_registry.list_agents()
        lines = ["[router] Available agents:"]
        for a in agents:
            lines.append(f"  • {a['name']} ({a['type']}) — {a['description']}")
        return ToolResult(tool_call_id="", output="\n".join(lines))


def register_agent_cluster_tools(registry) -> None:
    registry.register(AgentDispatchTool())
    registry.register(AgentListTool())
