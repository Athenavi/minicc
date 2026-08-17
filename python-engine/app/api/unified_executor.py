"""统一任务执行处理器 (Unified Task Executor)

整合对话、Agent、工作流的所有能力,通过 TaskRouter 自动编排执行

API 路由:
- POST /v1/chat/submit       提交自然语言任务 (自动编排)
- GET  /v1/chat/sessions     获取会话列表
- GET  /v1/chat/sessions/{id}/messages 获取消息历史
"""
from __future__ import annotations

import json
import logging
import time
from typing import Any, Optional
from dataclasses import dataclass, field

from app.core.task_router import TaskRouter, TaskPriority
from app.core.context_bus import publish_result, MessageType

logger = logging.getLogger(__name__)


@dataclass
class ChatSession:
    """对话会话"""
    session_id: str
    tenant_id: str
    title: str = ""
    messages: list[dict] = field(default_factory=list)
    created_at: float = field(default_factory=time.time)
    updated_at: float = 0
    
    # 共享上下文 (跨工作台状态)
    shared_context: dict[str, Any] = field(default_factory=dict)


class UnifiedChatHandler:
    """统一聊天处理器 (整合所有工作台能力)
    
    功能:
    1. 接收用户自然语言输入
    2. 调用 TaskRouter 自动编排执行
    3. 发布结果到 ContextBus
    4. 维护会话历史和共享上下文
    """
    
    def __init__(self):
        self.sessions: dict[str, ChatSession] = {}  # session_id -> Session
        self.router = TaskRouter()
    
    async def submit_task(
        self,
        user_input: str,
        tenant_id: str,
        session_id: Optional[str] = None,
        mode: str = "auto",  # "auto" / "agent" / "workflow"
        trace_id: str = "",
    ) -> dict[str, Any]:
        """提交任务 (自动编排)
        
        流程:
        1. 创建/获取会话
        2. 调用 TaskRouter 编排执行
        3. 将结果写入共享上下文
        4. 发布结果到 ContextBus
        5. 返回给用户
        """
        import uuid
        
        if not trace_id:
            trace_id = uuid.uuid4().hex[:12]
        
        # 创建/获取会话
        if not session_id:
            session_id = f"session_{int(time.time())}_{tenant_id[:8]}"
        
        if session_id not in self.sessions:
            self.sessions[session_id] = ChatSession(
                session_id=session_id,
                tenant_id=tenant_id,
                title=user_input[:50],
            )
        
        session = self.sessions[session_id]
        session.updated_at = time.time()
        
        # 添加用户消息到历史
        session.messages.append({
            "role": "user",
            "content": user_input,
            "timestamp": time.time(),
        })
        
        logger.info(
            "User submitted task (session=%s, tenant=%s, mode=%s, trace_id=%s)",
            session_id, tenant_id, mode, trace_id,
        )
        
        try:
            # 调用 TaskRouter (带模式选择)
            if mode == "agent":
                # 强制使用 Agent 工作台
                result = await self._execute_via_agent(user_input, tenant_id, trace_id)
            elif mode == "workflow":
                # 强制使用工作流工作台
                result = await self._execute_via_workflow(user_input, tenant_id, trace_id)
            else:
                # 自动模式 (默认)
                result = await self.router.route_task(
                    user_input=user_input,
                    tenant_id=tenant_id,
                    priority=TaskPriority.NORMAL,
                    trace_id=trace_id,
                )
            
            # 提取最终输出
            final_output = self._extract_output(result)
            
            # 添加到会话历史
            session.messages.append({
                "role": "assistant",
                "content": final_output,
                "timestamp": time.time(),
                "metadata": {
                    "task_id": result.get("task_id"),
                    "duration_ms": result.get("total_duration_ms"),
                    "subtasks": result.get("subtasks", []),
                }
            })
            
            # 更新共享上下文 (供后续任务使用)
            session.shared_context["last_output"] = final_output
            session.shared_context["last_trace_id"] = trace_id
            
            # 发布结果到 ContextBus
            await publish_result(
                topic=f"chat.sessions.{session_id}",
                data={
                    "output": final_output,
                    "task_id": result.get("task_id"),
                    "trace_id": trace_id,
                },
                tenant_id=tenant_id,
                message_type=MessageType.RESULT_PUBLISH,
            )
            
            logger.info(
                "Task completed (session=%s, task_id=%s, duration=%dms)",
                session_id, result.get("task_id"), result.get("total_duration_ms", 0),
            )
            
            return {
                "success": True,
                "session_id": session_id,
                "trace_id": trace_id,
                "output": final_output,
                "metadata": {
                    "task_id": result.get("task_id"),
                    "duration_ms": result.get("total_duration_ms"),
                    "subtasks_completed": result.get("output", {}).get("subtasks_completed", 0),
                }
            }
            
        except Exception as e:
            logger.error(f"Task execution failed: {e}", exc_info=True)
            
            # 记录错误到会话历史
            session.messages.append({
                "role": "assistant",
                "content": f"抱歉,执行失败: {str(e)}",
                "timestamp": time.time(),
                "error": str(e),
            })
            
            return {
                "success": False,
                "error": str(e),
                "session_id": session_id,
                "trace_id": trace_id,
            }
    
    async def _execute_via_agent(
        self,
        user_input: str,
        tenant_id: str,
        trace_id: str,
    ) -> dict:
        """通过 Agent 工作台执行"""
        from app.agent.multi_agent import SubAgent
        from app.main import get_gateway
        
        try:
            gateway = get_gateway()
        except RuntimeError:
            return {"status": "error", "output": {"error": "LLM gateway not initialized"}}
        
        agent = SubAgent(
            name="unified_chat_agent",
            description="通用助手 Agent",
            system_prompt="""你是一个多功能 AI 助手。请帮助用户完成各种任务,包括:
1. 数据分析和问题解答
2. 代码生成和调试
3. 文档编写和翻译
4. 知识库检索和信息查询

如果用户需要执行具体操作(如运行 Python 代码),请使用相应的工具。""",
            gateway=gateway,
            model="gpt-4o-mini",
            max_turns=10,
        )
        
        # TODO: 调用 agent.run()
        # result = await agent.run(task=user_input, tenant_id=tenant_id)
        
        return {
            "status": "completed",
            "output": {"result": "Agent 执行结果"},
            "total_duration_ms": 1000,
        }
    
    async def _execute_via_workflow(
        self,
        user_input: str,
        tenant_id: str,
        trace_id: str,
    ) -> dict:
        """通过工作流工作台执行"""
        from app.workflow.tracing_engine import TracingWorkflowEngine
        from app.main import get_gateway
        
        try:
            gateway = get_gateway()
        except RuntimeError:
            return {"status": "error", "output": {"error": "Gateway not available"}}
        
        # 构建简单的工作流: 理解 → 执行 → 返回
        graph_json = {
            "name": "chat_workflow",
            "nodes": [
                {
                    "id": "node_understand",
                    "type": "llm",
                    "label": "理解需求",
                    "config": {
                        "system_prompt": "分析用户需求,提取关键意图和实体",
                        "prompt": user_input,
                    }
                },
                {
                    "id": "node_execute",
                    "type": "tool",
                    "label": "执行任务",
                    "config": {
                        "tool_name": "analyze_and_execute",  # 需要注册
                    }
                },
                {
                    "id": "node_output",
                    "type": "output",
                    "label": "返回结果",
                },
            ],
            "edges": [
                {"source_id": "node_understand", "target_id": "node_execute"},
                {"source_id": "node_execute", "target_id": "node_output"},
            ],
        }
        
        engine = TracingWorkflowEngine(gateway)
        
        # TODO: 调用 engine.run_workflow_with_trace(...)
        
        return {
            "status": "completed",
            "output": {"result": "工作流执行结果"},
            "total_duration_ms": 2000,
        }
    
    def _extract_output(self, result: dict) -> str:
        """从执行结果中提取最终输出"""
        output_data = result.get("output", {})
        
        if isinstance(output_data, dict):
            # 查找最后一个成功的子任务输出
            outputs = output_data.get("outputs", [])
            for output_item in reversed(outputs):
                if output_item.get("output"):
                    return json.dumps(output_item["output"], ensure_ascii=False, indent=2)[:4000]
            
            # 降级: 返回摘要
            return json.dumps(output_data, ensure_ascii=False, indent=2)[:2000]
        
        elif isinstance(output_data, str):
            return output_data[:4000]
        
        else:
            return str(output_data)
    
    async def get_session_messages(
        self,
        session_id: str,
        limit: int = 50,
    ) -> dict[str, Any]:
        """获取会话消息历史"""
        session = self.sessions.get(session_id)
        if not session:
            return {"success": False, "error": "Session not found"}
        
        return {
            "success": True,
            "session_id": session_id,
            "messages": session.messages[-limit:],
            "shared_context": session.shared_context,
        }


# ── 全局单例 ───────────────────────────────────────────────────────
_global_chat_handler: Optional[UnifiedChatHandler] = None


def get_chat_handler() -> UnifiedChatHandler:
    """获取全局聊天处理器 (单例模式)"""
    global _global_chat_handler
    if _global_chat_handler is None:
        _global_chat_handler = UnifiedChatHandler()
    return _global_chat_handler


# ── FastAPI 路由示例 ────────────────────────────────────────────────
"""
from fastapi import APIRouter

router = APIRouter(tags=["chat"])

@router.post("/v1/chat/submit")
async def submit_chat(body: dict):
    handler = get_chat_handler()
    return await handler.submit_task(
        user_input=body.get("message", ""),
        tenant_id=body.get("tenant_id", ""),
        session_id=body.get("session_id"),
        mode=body.get("mode", "auto"),
    )

@router.get("/v1/chat/sessions/{session_id}/messages")
async def get_messages(session_id: str):
    handler = get_chat_handler()
    return await handler.get_session_messages(session_id)
"""
