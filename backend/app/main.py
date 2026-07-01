"""MiniCC FastAPI 入口 — SSE 事件流 + HTTP 命令端点。

参照 Reasonix 架构：
- SSE (GET /events)：
  Server-Sent Events 单向推送，服务端→客户端
  包含所有事件类型：text, reasoning, tool_dispatch, tool_progress, usage, message, turn_done, error 等
- HTTP POST 命令端点：
  客户端通过 POST 发送命令：/submit, /cancel, /approve
  命令结果也通过 SSE 事件流推送
"""

from __future__ import annotations

import asyncio
import json
import os
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse, StreamingResponse

from app.core.context_builder import ContextBuilder
from app.core.events import (
    APPROVAL_REQUEST, ERROR, MESSAGE, NOTICE, TEXT,
    TOOL_DISPATCH, TOOL_RESULT, TURN_DONE,
    TURN_STARTED, MiniCCEvent, broadcaster,
)
from app.core.extensions import ExtensionLoader
from app.core.permission import PermissionHandler
from app.engine.compactor import BudgetManager, CompactPipeline, SNIP_THRESHOLD
from app.engine.llm_provider import create_provider
from app.engine.query_engine import QueryEngine, QueryEngineConfig
from app.engine.session import SessionManager
from app.engine.task_manager import TaskManager
from app.tools.base import ToolRegistry
from app.tools.file_system import register_file_tools
from app.tools.search_tools import GlobTool, GrepTool
from app.tools.session_tools import AskUserQuestionTool, TodoWriteTool
from app.tools.tool_search import ToolSearchTool
from app.tools.web_tools import WebFetchTool
from app.tools.extras import (
    EnterPlanModeTool, ExitPlanModeTool,
    NotebookEditTool, WebSearchTool,
)
from app.tools.agent_tools import (
    AgentTool, SendMessageTool, SkillTool,
    TaskCreateTool, TaskGetTool, TaskListTool,
    TaskOutputTool, TaskStopTool, TaskUpdateTool,
)
from app.tools.shell_executor import ShellExecutorTool
from app.tools.codegraph import register_codegraph_tools
from app.tools.web import register_web_tools
from app.tools.web.scraper import register_scraper_tools
from app.tools.web.form_filler import register_form_tools
from app.tools.web.session import register_session_tools
from app.tools.office.excel import register_excel_tools
from app.tools.office.word import register_word_tools
from app.tools.office.email import register_email_tools
from app.tools.desktop.mouse_keyboard import register_mouse_keyboard_tools
from app.tools.desktop.ocr import register_ocr_tools
from app.tools.desktop.window import register_window_tools
from app.tools.desktop.clipboard import register_clipboard_tools
from app.automator import register_workflow_tools
from app.automator.scheduler import register_scheduler_tools
from app.tools.database.db_connector import register_db_tools
from app.tools.api.rest_client import register_api_tools
from app.automator.variables import register_var_tools
from app.automator.recorder import register_recorder_tools
from app.agents.router import register_agent_cluster_tools
from app.tools.editor_sync import router as editor_router
from app.tools.editor_api import router as editor_api_router
from app.tools.editor_actions import register_editor_tools
from app.agents.code_agent import CodeAgentTool
from app.graph.tools import register_graph_tools
from app.trace import TraceMiddleware, tracer
from app.tools.web.ai_enhance import register_ai_tools
from app.utils.config import settings
from app.utils.logger import logger
from app.utils.redis_client import RedisClient
from app.utils.sqlite_store import SQLiteStore

from app.commands import CommandDispatcher
from app.commands.core import ClearCommand, ConfigCommand, HelpCommand, StatusCommand, ToolsCommand


# -- 全局单例 --

tool_registry = ToolRegistry()
register_file_tools(tool_registry)
tool_registry.register(AskUserQuestionTool())
tool_registry.register(TodoWriteTool())
tool_registry.register(GlobTool())
tool_registry.register(GrepTool())
tool_registry.register(WebFetchTool())
tool_registry.register(ToolSearchTool(tool_registry))
tool_registry.register(AgentTool())
tool_registry.register(TaskCreateTool())
tool_registry.register(TaskGetTool())
tool_registry.register(TaskUpdateTool())
tool_registry.register(TaskListTool())
tool_registry.register(TaskStopTool())
tool_registry.register(TaskOutputTool())
tool_registry.register(SendMessageTool())
tool_registry.register(SkillTool())
tool_registry.register(NotebookEditTool())
tool_registry.register(WebSearchTool())
tool_registry.register(EnterPlanModeTool())
tool_registry.register(ExitPlanModeTool())
tool_registry.register(ShellExecutorTool())
register_codegraph_tools(tool_registry)
register_web_tools(tool_registry)
register_scraper_tools(tool_registry)
register_form_tools(tool_registry)
register_session_tools(tool_registry)
register_excel_tools(tool_registry)
register_word_tools(tool_registry)
register_email_tools(tool_registry)
register_mouse_keyboard_tools(tool_registry)
register_ocr_tools(tool_registry)
register_window_tools(tool_registry)
register_clipboard_tools(tool_registry)
register_workflow_tools(tool_registry)
register_scheduler_tools(tool_registry)
register_db_tools(tool_registry)
register_api_tools(tool_registry)
register_var_tools(tool_registry)
register_editor_tools(tool_registry)
register_ai_tools(tool_registry)
register_graph_tools(tool_registry)
register_agent_cluster_tools(tool_registry)
tool_registry.register(CodeAgentTool())

command_dispatcher = CommandDispatcher()
command_dispatcher.register(HelpCommand(command_dispatcher))
command_dispatcher.register(StatusCommand())
command_dispatcher.register(ToolsCommand(tool_registry))
command_dispatcher.register(ClearCommand())
command_dispatcher.register(ConfigCommand())

redis_client = RedisClient(settings.redis_url)
sqlite_store = SQLiteStore()
session_manager = SessionManager(redis_client, sqlite_store)
task_manager = TaskManager()

_active_engines: dict[str, QueryEngine] = {}
_active_permission: dict[str, PermissionHandler] = {}


@asynccontextmanager
async def lifespan(app: FastAPI):
    logger.info("MiniCC starting — provider=%s model=%s", settings.llm_provider, settings.llm_model)
    await redis_client.connect()
    await sqlite_store.connect()

    # 加载 MCP 扩展（如 codegraph）
    ext_loader = ExtensionLoader(tool_registry)
    await ext_loader.load_mcp_servers_from_config(".minicc/mcp.json")

    yield
    logger.info("MiniCC shutting down")
    await ext_loader.shutdown_all()
    await task_manager.cancel_all()
    await redis_client.disconnect()
    await sqlite_store.disconnect()


app = FastAPI(title="MiniCC API", version="0.1.0", lifespan=lifespan)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:3000", "http://127.0.0.1:3000"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.include_router(editor_router)
app.include_router(editor_api_router)


# ── SSE 事件流端点 ──
# 参照 Reasonix serve.go events 处理函数


@app.get("/events")
async def sse_events(request: Request):
    """SSE 事件流端点。"""

    async def event_stream():
        sub_id, queue = await broadcaster.subscribe()
        try:
            yield f"data: {json.dumps({'kind': 'connected'})}\n\n"
            while True:
                try:
                    event = await asyncio.wait_for(queue.get(), timeout=15)
                    data = json.dumps({"kind": event.kind, **event.data})
                    yield f"data: {data}\n\n"
                except asyncio.TimeoutError:
                    yield ": ping\n\n"
        except asyncio.CancelledError:
            pass
        finally:
            broadcaster.unsubscribe(sub_id)

    return StreamingResponse(
        event_stream(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
            "X-Accel-Buffering": "no",
        },
    )


# ── 基础端点 ──


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.get("/api/tools")
async def list_tools():
    tools = tool_registry.to_anthropic_tools() if settings.llm_provider == "anthropic" else tool_registry.to_openai_tools()
    return {"tools": tools, "count": len(tools)}


# ── HTTP 命令端点 ──
# 参照 Reasonix 的 POST /submit, /cancel, /approve


@app.post("/submit")
async def submit_message(request: Request):
    """提交用户消息。返回 202 Accepted，结果通过 SSE 推送。"""
    body = await request.json()
    content = body.get("content", "")
    session_id = body.get("session_id", "default")

    # 创建/获取 engine
    engine = _active_engines.get(session_id)
    if engine is None:
        context_builder = ContextBuilder(settings.workspace_dir)
        permission_handler = PermissionHandler()
        _active_permission[session_id] = permission_handler
        provider = create_provider(
            settings.llm_provider,
            settings.llm_api_key,
            settings.llm_model,
            base_url=settings.llm_base_url or None,
        )
        engine = QueryEngine(QueryEngineConfig(
            session_id=session_id,
            provider=provider,
            tool_registry=tool_registry,
            context_builder=context_builder,
            permission_handler=permission_handler,
            provider_type=settings.llm_provider,
        ))
        _active_engines[session_id] = engine

    # 在后台处理消息
    asyncio.create_task(_process_message(engine, session_id, content))

    return JSONResponse({"status": "accepted", "session_id": session_id}, status_code=202)


@app.post("/cancel")
async def cancel_message(request: Request):
    body = await request.json()
    session_id = body.get("session_id", "default")
    engine = _active_engines.get(session_id)
    if engine:
        engine.cancel()
        perm = _active_permission.get(session_id)
        if perm:
            perm.cancel_all_pending()
        await broadcaster.emit(MiniCCEvent(TURN_DONE, {"interrupted": True}))
    return JSONResponse({"status": "cancelled"})


@app.post("/mode")
async def set_mode(request: Request):
    """设置执行模式：ask / auto / yolo。"""
    body = await request.json()
    session_id = body.get("session_id", "default")
    mode = body.get("mode", "ask")
    perm = _active_permission.get(session_id)
    if perm:
        perm.set_mode(mode)
        await broadcaster.emit(MiniCCEvent(NOTICE, {"message": f"Mode changed to: {mode}"}))
    return JSONResponse({"status": "ok", "mode": mode})


@app.post("/approve")
async def approve_tool(request: Request):
    body = await request.json()
    session_id = body.get("session_id", "default")
    request_id = body.get("request_id", "")
    action = body.get("action", "approve")
    perm = _active_permission.get(session_id)
    if perm:
        perm.handle_user_response(request_id, action)
    return JSONResponse({"status": "ok"})


# ── 后台消息处理 ──


async def _process_message(engine: QueryEngine, session_id: str, content: str) -> None:
    """处理用户消息：驱动 QueryEngine 主循环并通过 Broadcaster 推送事件。"""
    await broadcaster.emit(MiniCCEvent(TURN_STARTED, {"session_id": session_id}))

    # 为 PermissionHandler 设置发送回调，使其能通过 Broadcaster 发送审批请求
    async def _permission_sender(data: dict) -> None:
        """PermissionHandler 通过此回调发送审批请求到前端。"""
        if data.get("type") == "permission_required":
            payload = data.get("payload", {})
            await broadcaster.emit(MiniCCEvent(APPROVAL_REQUEST, {
                "request_id": payload.get("id", ""),
                "tool_name": payload.get("tool_name", ""),
                "tool_input": payload.get("tool_input", {}),
                "level": payload.get("level", "write"),
                "diff_preview": payload.get("diff_preview", ""),
            }))

    # 将回调注入 PermissionHandler
    if hasattr(engine, "_permission_handler") and engine._permission_handler:
        engine._permission_handler._send = _permission_sender

    # 检查 slash command
    cmd_context = {
        "session_id": session_id,
        "messages": engine.mutable_messages,
        "usage": engine.total_usage.model_dump() if hasattr(engine, "total_usage") else {},
        "provider": settings.llm_provider,
        "model": settings.llm_model,
    }
    cmd_result = await command_dispatcher.dispatch(content, cmd_context)
    if cmd_result is not None:
        await broadcaster.emit(MiniCCEvent(TEXT, {"text": cmd_result}))
        await broadcaster.emit(MiniCCEvent(MESSAGE, {"text": cmd_result}))
        await broadcaster.emit(MiniCCEvent(TURN_DONE, {}))
        return

    try:
        async for event in engine.submit_message(content):
            etype = event["type"]
            payload = event.get("payload", {})

            if etype == "text_chunk":
                await broadcaster.emit(MiniCCEvent(TEXT, {"text": payload.get("text", "")}))

            elif etype == "tool_call_start":
                await broadcaster.emit(MiniCCEvent(TOOL_DISPATCH, {
                    "id": payload.get("call_id", ""),
                    "name": payload.get("name", ""),
                    "input": payload.get("input", {}),
                }))

            elif etype == "tool_call_result":
                await broadcaster.emit(MiniCCEvent(TOOL_RESULT, {
                    "id": payload.get("call_id", ""),
                    "output": payload.get("output", ""),
                    "is_error": payload.get("is_error", False),
                }))

            elif etype == "permission_required":
                await broadcaster.emit(MiniCCEvent(APPROVAL_REQUEST, {
                    "request_id": payload.get("request_id", ""),
                    "tool_name": payload.get("tool_name", ""),
                    "tool_input": payload.get("tool_input", {}),
                    "level": payload.get("level", "write"),
                    "diff_preview": payload.get("diff_preview", ""),
                }))

            elif etype == "message_complete":
                # 最终消息（含完整文本，供前端重新渲染）
                full_text = ""
                # Collect text from the last assistant message
                if engine.mutable_messages and hasattr(engine, "mutable_messages"):
                    for msg in reversed(engine.mutable_messages):
                        if hasattr(msg, "role") and str(msg.role) == "assistant":
                            if isinstance(msg.content, str):
                                full_text = msg.content
                            break
                await broadcaster.emit(MiniCCEvent(MESSAGE, {
                    "text": full_text,
                    "usage": payload.get("usage", {}),
                }))
                await broadcaster.emit(MiniCCEvent(TURN_DONE, {}))

            elif etype == "error":
                await broadcaster.emit(MiniCCEvent(ERROR, {"message": payload.get("message", "Unknown error")}))
                await broadcaster.emit(MiniCCEvent(TURN_DONE, {"error": payload.get("message", "")}))

            elif etype == "compaction":
                await broadcaster.emit(MiniCCEvent(NOTICE, {
                    "message": f"Context compressed (freed ~{payload.get('tokens_freed', 0)} tokens)",
                }))

    except Exception as exc:
        logger.exception("Message processing error")
        await broadcaster.emit(MiniCCEvent(ERROR, {"message": str(exc)}))
        await broadcaster.emit(MiniCCEvent(TURN_DONE, {"error": str(exc)}))
