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
from app.core.context_bus import publish_result

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
            # Fail loud: 执行器返回 status=error (如 gateway 未初始化) 时,
            # 不得包装成 success=True 响应,必须转入异常路径返回明确错误。
            if result.get("status") == "error":
                output_data = result.get("output")
                error_msg = (
                    output_data.get("error", "execution failed")
                    if isinstance(output_data, dict) else "execution failed"
                )
                raise RuntimeError(str(error_msg))

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
            
            # 发布结果到 ContextBus (publish_result 默认即 RESULT_PUBLISH)
            await publish_result(
                topic=f"chat.sessions.{session_id}",
                data={
                    "output": final_output,
                    "task_id": result.get("task_id"),
                    "trace_id": trace_id,
                },
                tenant_id=tenant_id,
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
                    "subtasks": result.get("subtasks", []),
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
            gateway = await get_gateway()
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
        
        # SubAgent.run 已实现: 真实调用并透传结果 (绝不返回伪造输出)
        agent_result = await agent.run(task=user_input, tenant_id=tenant_id)
        if not agent_result.success:
            # Fail loud: Agent 执行失败必须向上抛出,由 submit_task 返回 success=False
            raise RuntimeError(f"Agent execution failed: {agent_result.error or 'unknown error'}")
        
        return {
            "status": "completed",
            "output": {
                "result": agent_result.output,
                "tool_calls": agent_result.tool_calls,
            },
            "total_duration_ms": int(agent_result.duration * 1000),
        }
    
    async def _execute_via_workflow(
        self,
        user_input: str,
        tenant_id: str,
        trace_id: str,
    ) -> dict:
        """通过工作流工作台执行

        构建单节点 LLM 工作流（input → llm → output）并执行，
        复用 workflow/engine.run_workflow 的 DAG 执行能力。
        """
        from app.workflow.engine import run_workflow
        from app.main import get_gateway

        try:
            gateway = await get_gateway()
        except RuntimeError:
            return {"status": "error", "output": {"error": "LLM gateway not initialized"}}

        # 构建简单工作流图: input → llm → output
        graph_json = {
            "name": f"unified_chat_{trace_id}",
            "nodes": [
                {"id": "input_1", "node_type": "input", "label": "用户输入"},
                {
                    "id": "llm_1",
                    "node_type": "llm",
                    "label": "LLM 处理",
                    "config": {
                        "system_prompt": "你是一个多功能 AI 助手,请根据用户输入给出清晰、准确的回答。",
                        "user_message": user_input,
                    },
                },
                {"id": "output_1", "node_type": "output", "label": "输出"},
            ],
            "edges": [
                {"source_id": "input_1", "target_id": "llm_1"},
                {"source_id": "llm_1", "target_id": "output_1"},
            ],
        }

        instance = await run_workflow(
            graph_json=graph_json,
            gateway=gateway,
            initial_state={"input": user_input},
            instance_id=f"wf_{trace_id}",
        )

        if instance.status == "error":
            raise RuntimeError(f"Workflow execution failed: {instance.error}")

        # 提取最终输出（output 节点的结果）
        final_output = instance.state.get("__out_output_1__", "") or instance.state.get("__out_llm_1__", "")

        return {
            "status": "completed",
            "output": {
                "result": final_output,
                "workflow_instance_id": instance.instance_id,
            },
            "total_duration_ms": int((instance.finished_at - instance.started_at) * 1000) if instance.finished_at else 0,
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


# ── FastAPI 路由（六大工作台统一对话入口） ──────────────────────────
from fastapi import APIRouter, Request  # noqa: E402

router = APIRouter(tags=["chat"])


@router.post("/v1/chat/submit")
async def submit_chat(request: Request):
    """统一任务提交入口：TaskRouter 自动编排六大工作台能力

    Body: {message/user_input, tenant_id?, session_id?, mode?}
    Go 网关代理时追加 ?user_id=<claims.UserID>，作为 tenant_id 兜底。
    """
    body = await request.json()
    user_input = str(body.get("message") or body.get("user_input") or "")
    if not user_input.strip():
        return {"success": False, "error": "message is required"}

    # S 多租户隔离:优先 body → tenant_id query param(Go 网关注入 claims.TenantID)→ 拒绝
    # 历史遗留:曾用 user_id 兜底,但 user_id 不是租户标识,会导致跨租户数据访问
    tenant_id = str(body.get("tenant_id") or request.query_params.get("tenant_id") or "")
    if not tenant_id:
        return {"success": False, "error": "tenant_id is required"}

    handler = get_chat_handler()
    return await handler.submit_task(
        user_input=user_input,
        tenant_id=tenant_id,
        session_id=body.get("session_id"),
        mode=str(body.get("mode", "auto")),
    )


@router.get("/v1/chat/sessions/{session_id}/messages")
async def get_messages(session_id: str, limit: int = 50):
    """获取会话消息历史（含跨工作台共享上下文）"""
    handler = get_chat_handler()
    return await handler.get_session_messages(session_id, limit=limit)
