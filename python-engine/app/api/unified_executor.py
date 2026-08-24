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
from dataclasses import dataclass, field
from datetime import datetime
from typing import Any, Optional

from app.core.context_bus import publish_result
from app.core.task_router import TaskPriority, TaskRouter
from app.db import get_pool

logger = logging.getLogger(__name__)


@dataclass
class ChatSession:
    """对话会话"""
    session_id: str
    tenant_id: str
    title: str = ""
    mode: str = "auto"
    messages: list[dict] = field(default_factory=list)
    created_at: float = field(default_factory=time.time)
    updated_at: float = 0
    
    # 共享上下文 (跨工作台状态)
    shared_context: dict[str, Any] = field(default_factory=dict)


def _dt_to_ts(value: Any) -> float:
    """timestamptz → Unix 秒 (datetime / None / 数值兼容)。"""
    if value is None:
        return time.time()
    if isinstance(value, datetime):
        return value.timestamp()
    return float(value)


class UnifiedChatHandler:
    """统一聊天处理器 (整合所有工作台能力)
    
    功能:
    1. 接收用户自然语言输入
    2. 调用 TaskRouter 自动编排执行
    3. 发布结果到 ContextBus
    4. 维护会话历史和共享上下文
    """
    
    def __init__(self):
        self.sessions: dict[str, ChatSession] = {}  # session_id -> Session (L1 缓存,有界)
        self.router = TaskRouter()
        self._db_warned = False  # DB 降级仅告警一次

    # L1 会话缓存上限(S 资源修复: 原 self.sessions 无界,消息无限 append → 内存线性增长)
    _MAX_L1_SESSIONS = 200

    def _evict_l1_sessions(self) -> None:
        """超出上限时淘汰最久未更新的会话,防止内存无限增长。"""
        while len(self.sessions) > self._MAX_L1_SESSIONS:
            oldest_id = min(self.sessions, key=lambda k: self.sessions[k].updated_at)
            self.sessions.pop(oldest_id, None)

    # ── PostgreSQL 持久化层 (统一任务会话) ──────────────────────────
    # 写策略: 先写库,再更新内存 L1 缓存 (写穿透)。
    # 读策略: 读库为准 (多实例一致),内存仅作 DB 不可用时的兜底。
    # 所有 DB 操作独立 try/except: 池未初始化 / 表缺失 / 连接异常均
    # 降级为纯内存模式并 warn 一次,绝不阻断主任务执行。

    def _warn_db(self, exc: Exception) -> None:
        """DB 不可用时仅告警一次,避免日志刷屏。"""
        if not self._db_warned:
            self._db_warned = True
            logger.warning(
                "Unified session DB unavailable, falling back to in-memory mode: %s", exc
            )

    async def _db_get_session(self, session_id: str) -> dict | None:
        """按 id 读取会话行;DB 不可用或会话不存在返回 None。"""
        try:
            pool = get_pool()
            row = await pool.fetchrow(
                "SELECT id, tenant_id, user_id, title, mode, shared_context, "
                "created_at, updated_at FROM unified_sessions WHERE id = $1",
                session_id,
            )
            return dict(row) if row else None
        except Exception as e:  # noqa: BLE001
            self._warn_db(e)
            return None

    async def _db_ensure_session(
        self,
        session_id: str,
        tenant_id: str,
        user_id: str,
        title: str,
        mode: str,
        shared_context: dict,
    ) -> bool:
        """确保会话行存在 (INSERT ... ON CONFLICT DO NOTHING,幂等)。

        多实例并发创建同一会话时,后到者冲突被忽略,行必然存在。
        返回 True 表示 DB 可用;False 表示降级为内存模式。
        """
        try:
            pool = get_pool()
            await pool.execute(
                "INSERT INTO unified_sessions (id, tenant_id, user_id, title, mode, shared_context) "
                "VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO NOTHING",
                session_id, tenant_id, user_id, title, mode, dict(shared_context),
            )
            return True
        except Exception as e:  # noqa: BLE001
            self._warn_db(e)
            return False

    async def _db_append_message(
        self,
        session_id: str,
        role: str,
        content: str,
        metadata: dict,
        error: str = "",
    ) -> None:
        """追加一条消息到 unified_messages (写库失败不阻断主流程)。"""
        try:
            pool = get_pool()
            await pool.execute(
                "INSERT INTO unified_messages (session_id, role, content, metadata, error) "
                "VALUES ($1, $2, $3, $4, $5)",
                session_id, role, content, dict(metadata), error,
            )
        except Exception as e:  # noqa: BLE001
            self._warn_db(e)

    async def _db_touch_session(
        self,
        session_id: str,
        mode: str,
        shared_context: dict,
    ) -> None:
        """更新会话 mode / shared_context (jsonb 顶层键合并) / updated_at。"""
        try:
            pool = get_pool()
            await pool.execute(
                "UPDATE unified_sessions "
                "SET mode = $2, shared_context = shared_context || $3::jsonb, "
                "updated_at = NOW() WHERE id = $1",
                session_id, mode, dict(shared_context),
            )
        except Exception as e:  # noqa: BLE001
            self._warn_db(e)
    
    async def submit_task(
        self,
        user_input: str,
        tenant_id: str,
        session_id: Optional[str] = None,
        mode: str = "auto",  # "auto" / "agent" / "workflow"
        trace_id: str = "",
        context: Optional[dict] = None,
        user_id: str = "",
    ) -> dict[str, Any]:
        """提交任务 (自动编排)
        
        流程:
        1. 创建/获取会话
        2. 调用 TaskRouter 编排执行
        3. 将结果写入共享上下文
        4. 发布结果到 ContextBus
        5. 返回给用户
        
        Args:
            context: 可选会话上下文 (跨工作台注入):
                - kb_id (str): 知识库 ID,执行前做 RAG 检索,片段拼接到 user_input 前
                - agent (dict): mode=agent 时的 SubAgent 配置覆盖
                  (system_prompt / model / max_turns / name,缺省保持现有默认值)
                - skill_names (list[str]): 透传给 TaskRouter.route_task 的 context,
                  供能力匹配阶段作提示 (不改变现有匹配逻辑)
                - workflow_id (str): mode=workflow 时透传记录到结果 metadata
            user_id: 会话归属用户标识 (Go 网关经 ?user_id=<claims.UserID> 注入;
                缺省以 tenant_id 兜底),持久化到 unified_sessions.user_id。

        持久化: 会话与每轮消息写 PostgreSQL (unified_sessions / unified_messages),
        DB 不可用时降级为内存模式 (见 get_session_messages 同款策略)。
        """
        import uuid
        
        if not trace_id:
            trace_id = uuid.uuid4().hex[:12]
        
        context = context or {}
        ctx_agent = context.get("agent") if isinstance(context.get("agent"), dict) else None
        ctx_kb_id = str(context.get("kb_id") or "")
        ctx_workflow_id = str(context.get("workflow_id") or "")
        
        # ── 创建/获取会话 (内存 L1 缓存 + PostgreSQL 写穿透持久化) ──
        if not session_id:
            session_id = f"session_{int(time.time())}_{tenant_id[:8]}"

        session = self.sessions.get(session_id)
        if session is None:
            # 先尝试从 DB 恢复 (多实例场景: 其他实例可能已创建该会话)
            db_row = await self._db_get_session(session_id)
            if db_row is not None:
                session = ChatSession(
                    session_id=session_id,
                    tenant_id=db_row["tenant_id"],
                    title=db_row["title"] or "",
                    mode=db_row["mode"] or mode,
                    created_at=_dt_to_ts(db_row.get("created_at")),
                    updated_at=_dt_to_ts(db_row.get("updated_at")),
                    shared_context=dict(db_row.get("shared_context") or {}),
                )
            else:
                session = ChatSession(
                    session_id=session_id,
                    tenant_id=tenant_id,
                    title=user_input[:50],
                    mode=mode,
                    # 初始 shared_context = 请求注入的跨工作台上下文
                    shared_context=dict(context),
                )
            self.sessions[session_id] = session
            self._evict_l1_sessions()

        session.updated_at = time.time()
        session.mode = mode

        # 写穿透: 确保会话行存在 (幂等; DB 不可用时静默降级为内存模式)
        db_ok = await self._db_ensure_session(
            session_id=session_id,
            tenant_id=tenant_id,
            user_id=user_id or tenant_id,
            title=session.title,
            mode=mode,
            shared_context=session.shared_context,
        )

        # 添加用户消息到历史 (先写库,再更新内存缓存)
        session.messages.append({
            "role": "user",
            "content": user_input,
            "timestamp": time.time(),
        })
        if db_ok:
            await self._db_append_message(session_id, "user", user_input, {})
        
        logger.info(
            "User submitted task (session=%s, tenant=%s, mode=%s, trace_id=%s)",
            session_id, tenant_id, mode, trace_id,
        )
        
        try:
            # 会话上下文注入: context.kb_id → 执行前 RAG 检索,片段拼接到 user_input 前
            kb_hits = 0
            effective_input = user_input
            if ctx_kb_id:
                kb_block, kb_hits = await self._retrieve_kb_context(
                    kb_id=ctx_kb_id, query=user_input, tenant_id=tenant_id, trace_id=trace_id,
                )
                if kb_block:
                    effective_input = f"{kb_block}\n{user_input}"
                logger.info(
                    "KB context injected (kb_id=%s, hits=%d, session=%s)",
                    ctx_kb_id, kb_hits, session_id,
                )

            # 调用 TaskRouter (带模式选择)
            if mode == "agent":
                # 强制使用 Agent 工作台 (context.agent 覆盖 SubAgent 配置)
                result = await self._execute_via_agent(
                    effective_input, tenant_id, trace_id, agent_config=ctx_agent,
                )
            elif mode == "workflow":
                # 强制使用工作流工作台 (context.workflow_id 仅透传记录)
                result = await self._execute_via_workflow(
                    effective_input, tenant_id, trace_id, workflow_id=ctx_workflow_id,
                )
            else:
                # 自动模式 (默认)
                if context:
                    # context (含 skill_names) 透传给 route_task,供意图/能力匹配阶段参考
                    result = await self.router.route_task(
                        user_input=effective_input,
                        tenant_id=tenant_id,
                        priority=TaskPriority.NORMAL,
                        trace_id=trace_id,
                        context=context,
                    )
                else:
                    result = await self.router.route_task(
                        user_input=effective_input,
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
            
            # 结果元数据: 基础字段 + 上下文注入的可选字段 (kb_hits/kb_id/workflow_id/agent_name)
            meta: dict[str, Any] = {
                "task_id": result.get("task_id"),
                "duration_ms": result.get("total_duration_ms"),
                "subtasks": result.get("subtasks", []),
            }
            if ctx_kb_id:
                meta["kb_hits"] = kb_hits
                meta["kb_id"] = ctx_kb_id
            if ctx_workflow_id or result.get("workflow_id"):
                meta["workflow_id"] = result.get("workflow_id") or ctx_workflow_id
            if mode == "agent":
                meta["agent_name"] = (ctx_agent or {}).get("name") or "unified_chat_agent"
            
            # 添加到会话历史 (assistant 消息 metadata 同带引用/来源字段,供前端展示)
            session.messages.append({
                "role": "assistant",
                "content": final_output,
                "timestamp": time.time(),
                "metadata": dict(meta),
            })
            
            # 更新共享上下文 (供后续任务使用)
            session.shared_context["last_output"] = final_output
            session.shared_context["last_trace_id"] = trace_id
            
            # 写穿透: 落库 assistant 消息 + 更新会话共享上下文
            if db_ok:
                await self._db_append_message(session_id, "assistant", final_output, dict(meta))
                await self._db_touch_session(session_id, mode, session.shared_context)
            
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
                    **meta,
                    "subtasks_completed": result.get("output", {}).get("subtasks_completed", 0),
                }
            }
            
        except Exception as e:
            logger.error(f"Task execution failed: {e}", exc_info=True)
            
            # 记录错误到会话历史 (失败同样落库,便于跨实例恢复)
            session.messages.append({
                "role": "assistant",
                "content": f"抱歉,执行失败: {str(e)}",
                "timestamp": time.time(),
                "error": str(e),
            })
            if db_ok:
                await self._db_append_message(
                    session_id, "assistant", f"抱歉,执行失败: {str(e)}", {}, error=str(e)
                )
                await self._db_touch_session(session_id, mode, session.shared_context)
            
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
        agent_config: Optional[dict] = None,
    ) -> dict:
        """通过 Agent 工作台执行

        agent_config (来自 context.agent) 可覆盖 SubAgent 的
        name / system_prompt / model / max_turns,缺省时保持现有默认值。
        """
        from app.agent.multi_agent import SubAgent
        from app.main import get_gateway
        
        try:
            gateway = await get_gateway()
        except RuntimeError:
            return {"status": "error", "output": {"error": "LLM gateway not initialized"}}
        
        agent_config = agent_config or {}
        
        # name 覆盖
        agent_name = str(agent_config.get("name") or "unified_chat_agent")
        
        # system_prompt 覆盖 (缺省保持现有默认)
        default_prompt = """你是一个多功能 AI 助手。请帮助用户完成各种任务,包括:
1. 数据分析和问题解答
2. 代码生成和调试
3. 文档编写和翻译
4. 知识库检索和信息查询

如果用户需要执行具体操作(如运行 Python 代码),请使用相应的工具。"""
        system_prompt = str(agent_config.get("system_prompt") or default_prompt)
        
        # model 覆盖 (缺省保持现有默认 gpt-4o-mini)
        model = str(agent_config.get("model") or "gpt-4o-mini")
        
        # max_turns 覆盖 (缺省保持现有默认 10)
        try:
            max_turns = int(agent_config.get("max_turns", 10) or 10)
        except (TypeError, ValueError):
            max_turns = 10
        if max_turns <= 0:
            max_turns = 10
        
        agent = SubAgent(
            name=agent_name,
            description="通用助手 Agent",
            system_prompt=system_prompt,
            gateway=gateway,
            model=model,
            max_turns=max_turns,
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
        workflow_id: str = "",
    ) -> dict:
        """通过工作流工作台执行

        构建单节点 LLM 工作流（input → llm → output）并执行，
        复用 workflow/engine.run_workflow 的 DAG 执行能力。
        workflow_id 仅透传记录到结果,不改变工作流构建逻辑。
        """
        from app.main import get_gateway
        from app.workflow.engine import run_workflow

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
            "workflow_id": workflow_id,
            "total_duration_ms": int((instance.finished_at - instance.started_at) * 1000) if instance.finished_at else 0,
        }
    
    async def _retrieve_kb_context(
        self,
        kb_id: str,
        query: str,
        tenant_id: str,
        trace_id: str,
        top_k: int = 5,
    ) -> tuple[str, int]:
        """基于 context.kb_id 做 RAG 检索 (复用 knowledge:kb_search 的 kb_search 工具)

        返回 (引用块, 命中数);任何失败 (DB 不可用 / kb 不存在 / 无命中) 都降级为
        空块 + 0,不阻断主任务执行 (与"不传 context 行为一致"的向后兼容原则一致)。
        """
        try:
            from app.tools.context import set_tool_context
            from app.tools.kb import kb_search

            # kb_search 依赖工具上下文做归属校验 (与 TaskRouter._execute_single_task 同口径)
            set_tool_context(
                session_id=f"task_{trace_id}" if trace_id else "chat_context_rag",
                user_id=tenant_id,
                tenant_id=tenant_id,
            )

            result = await kb_search(kb_id=kb_id, query=query, top_k=top_k)
            if not isinstance(result, dict) or "error" in result:
                error = result.get("error") if isinstance(result, dict) else result
                logger.warning("KB context retrieval skipped (kb_id=%s): %s", kb_id, error)
                return "", 0

            hits = result.get("results", []) or []
            if not hits:
                return "", 0

            snippets: list[str] = []
            for hit in hits[:top_k]:
                content = (hit.get("content") or hit.get("name") or "").strip()
                if content:
                    snippets.append(content[:300])
            if not snippets:
                return "", 0

            block = "【知识库引用】\n" + "\n".join(f"- {s}" for s in snippets)
            return block, len(hits)

        except Exception as e:  # noqa: BLE001 — 检索失败不阻断主任务
            logger.warning("KB context retrieval failed (kb_id=%s): %s", kb_id, e)
            return "", 0
    
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
        tenant_id: str | None = None,
        limit: int = 50,
    ) -> dict[str, Any]:
        """获取会话消息历史 (DB 优先,内存兜底)

        S 安全修复:必须提供 tenant_id 并校验会话归属,杜绝跨租户读取消息。

        返回结构保持兼容:
        {success, session_id, messages: [{role, content, timestamp, metadata?, error?}], shared_context}
        timestamp 为 Unix 秒 (由 DB created_at 转换)。
        """
        if not tenant_id:
            return {"success": False, "error": "tenant_id is required"}

        # 1) DB 优先: 读库为准,保证多实例一致
        try:
            pool = get_pool()
        except RuntimeError:
            pool = None

        if pool is not None:
            try:
                row = await pool.fetchrow(
                    "SELECT tenant_id, title, mode, shared_context "
                    "FROM unified_sessions WHERE id = $1 AND tenant_id = $2",
                    session_id, tenant_id,
                )
                if row is None:
                    return {"success": False, "error": "Session not found"}
                rows = await pool.fetch(
                    "SELECT role, content, metadata, error, created_at "
                    "FROM unified_messages WHERE session_id = $1 "
                    "ORDER BY created_at, id LIMIT $2",
                    session_id, limit,
                )
                messages: list[dict] = []
                for m in rows:
                    msg: dict[str, Any] = {
                        "role": m["role"],
                        "content": m["content"],
                        "timestamp": _dt_to_ts(m["created_at"]),
                    }
                    if m.get("metadata"):
                        msg["metadata"] = m["metadata"]
                    if m.get("error"):
                        msg["error"] = m["error"]
                    messages.append(msg)
                shared_context = dict(row["shared_context"] or {})
                # 回填 L1 缓存 (DB 不可用时内存兜底仍可读到最新)
                cache = self.sessions.get(session_id)
                if cache is not None:
                    cache.messages = messages
                    cache.shared_context = shared_context
                return {
                    "success": True,
                    "session_id": session_id,
                    "messages": messages,
                    "shared_context": shared_context,
                }
            except Exception as e:  # noqa: BLE001 — 表缺失/连接异常 → 降级内存
                self._warn_db(e)

        # 2) 内存兜底 (DB 不可用,维持旧版行为)
        session = self.sessions.get(session_id)
        if not session or getattr(session, "tenant_id", None) != tenant_id:
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

    Body: {message/user_input, tenant_id?, session_id?, mode?, context?}
    context (可选 dict): 跨工作台会话上下文注入 —
        kb_id (str): 执行前 RAG 检索,片段以【知识库引用】拼接到输入前
        agent (dict): mode=agent 时覆盖 SubAgent 配置 (system_prompt/model/max_turns/name)
        skill_names (list[str]): 透传 TaskRouter 作能力匹配提示
        workflow_id (str): mode=workflow 时记录到结果 metadata
    Go 网关代理时追加 ?user_id=<claims.UserID>，作为 tenant_id 兜底。
    """
    body = await request.json()
    user_input = str(body.get("message") or body.get("user_input") or "")
    if not user_input.strip():
        return {"success": False, "error": "message is required"}

    # 多租户隔离:query 的 tenant_id 由 Go 网关从已验证 JWT claims 可信注入,
    # 必须优先于 body 中客户端可伪造的 tenant_id(S 安全修复,防止跨租户)。
    # 两者都存在且不一致时直接拒绝,绝不允许以客户端 body 覆盖可信身份。
    query_tid = str(request.query_params.get("tenant_id") or "")
    body_tid = str(body.get("tenant_id") or "")
    if query_tid and body_tid and query_tid != body_tid:
        return {"success": False, "error": "tenant_id mismatch"}
    tenant_id = query_tid or body_tid
    if not tenant_id:
        return {"success": False, "error": "tenant_id is required"}

    handler = get_chat_handler()
    context = body.get("context")
    if context is not None and not isinstance(context, dict):
        return {"success": False, "error": "context must be an object"}
    # user_id 持久化标识 (Go 网关代理时追加 ?user_id=<claims.UserID>;缺省以 tenant_id 兜底)。
    # 同 tenant_id 逻辑:query 可信优先,防止客户端伪造 user_id 冒充他人。
    query_uid = str(request.query_params.get("user_id") or "")
    body_uid = str(body.get("user_id") or "")
    user_id = query_uid or body_uid
    return await handler.submit_task(
        user_input=user_input,
        tenant_id=tenant_id,
        session_id=body.get("session_id"),
        mode=str(body.get("mode", "auto")),
        context=context,
        user_id=user_id,
    )


@router.get("/v1/chat/sessions/{session_id}/messages")
async def get_messages(request: Request, session_id: str, limit: int = 50):
    """获取会话消息历史（含跨工作台共享上下文）

    S 安全修复:强制租户校验。tenant_id 优先取 Go 网关可信注入的
    ?tenant_id= 查询参数,缺省尝试 request.state.tenant_id(中间件设置时)。
    """
    tenant_id = str(request.query_params.get("tenant_id") or "") or \
        str(getattr(request.state, "tenant_id", "") or "")
    handler = get_chat_handler()
    return await handler.get_session_messages(session_id, tenant_id=tenant_id, limit=limit)
