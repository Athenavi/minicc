"""MiniCC FastAPI 入口 — 路由注册与应用生命周期。

WebSocket 端点处理：
- 用户消息 → QueryEngine.submit_message()
- cancel 信号 → QueryEngine.cancel() + PermissionHandler.cancel_all()
- 会话恢复 → SessionManager.resume_session()
"""

from __future__ import annotations

import asyncio
import json
from contextlib import asynccontextmanager

from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware

from app.core.context_builder import ContextBuilder
from app.core.permission import PermissionHandler
from app.engine.query_engine import QueryEngine, QueryEngineConfig
from app.engine.session import SessionManager
from app.engine.task_manager import TaskManager
from app.tools.base import ToolRegistry
from app.tools.file_system import register_file_tools
from app.tools.search_tools import GlobTool, GrepTool
from app.tools.session_tools import AskUserQuestionTool, TodoWriteTool
from app.tools.tool_search import ToolSearchTool
from app.tools.web_tools import WebFetchTool
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

# 命令系统
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

# 活跃的 QueryEngine 实例（session_id → engine）
_active_engines: dict[str, QueryEngine] = {}


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期：初始化和清理。"""
    logger.info("MiniCC starting — provider=%s model=%s", settings.llm_provider, settings.llm_model)
    await redis_client.connect()
    await sqlite_store.connect()
    yield
    logger.info("MiniCC shutting down")
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


# -- REST 端点 --


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.get("/api/tools")
async def list_tools():
    tools = tool_registry.to_anthropic_tools() if settings.llm_provider == "anthropic" else tool_registry.to_openai_tools()
    return {"tools": tools, "count": len(tools)}


@app.get("/api/sessions")
async def list_sessions():
    sessions = await session_manager.list_sessions(limit=20)
    return {
        "sessions": [
            {"session_id": s.session_id, "created_at": s.created_at.isoformat(), "message_count": len(s.messages)}
            for s in sessions
        ]
    }


# -- WebSocket 端点 --


@app.websocket("/ws/{session_id}")
async def agent_websocket(websocket: WebSocket, session_id: str):
    await websocket.accept()
    logger.info("WS connect: session=%s", session_id)

    # 初始化或恢复会话
    context_builder = ContextBuilder(settings.workspace_dir)
    permission_handler = PermissionHandler()

    engine = _active_engines.get(session_id)
    if engine is None:
        # 从持久层恢复
        saved = await session_manager.resume_session(session_id)
        engine = QueryEngine(QueryEngineConfig(
            session_id=session_id,
            provider=None,  # Phase 1 注入 LLM provider
            tool_registry=tool_registry,
            context_builder=context_builder,
            permission_handler=permission_handler,
        ))
        _active_engines[session_id] = engine

    # 发送会话信息
    await websocket.send_json({
        "type": "session_info",
        "payload": {
            "session_id": session_id,
            "message_count": len(engine.mutable_messages),
        },
    })

    output_queue: asyncio.Queue = asyncio.Queue()

    async def writer():
        """从队列取消息并发送到 WebSocket。"""
        while True:
            try:
                msg = await asyncio.wait_for(output_queue.get(), timeout=0.5)
                await websocket.send_json(msg)
            except asyncio.TimeoutError:
                continue
            except Exception:
                break

    writer_task = asyncio.create_task(writer())

    try:
        async for raw in websocket.iter_text():
            try:
                data = json.loads(raw)
            except json.JSONDecodeError:
                continue

            msg_type = data.get("type", "")
            payload = data.get("payload", {})

            if msg_type == "cancel":
                engine.cancel()
                permission_handler.cancel_all_pending()
                logger.info("Cancel signal: session=%s", session_id)
                continue

            if msg_type == "user_message":
                content = payload.get("content", "")

                # 检测 slash command
                cmd_context = {
                    "session_id": session_id,
                    "message_count": len(engine.mutable_messages),
                    "messages": engine.mutable_messages,
                    "usage": engine.total_usage.model_dump() if hasattr(engine, "total_usage") else {},
                    "provider": settings.llm_provider,
                    "model": settings.llm_model,
                }
                cmd_result = await command_dispatcher.dispatch(content, cmd_context)
                if cmd_result is not None:
                    # 是命令，直接返回结果
                    await websocket.send_json({
                        "type": "command_result",
                        "payload": {"output": cmd_result},
                    })
                    continue

                # 普通用户消息
                await _handle_user_message(engine, output_queue, session_id, content)
                continue

            if msg_type == "approval_action":
                req_id = payload.get("request_id", "")
                action = payload.get("action", "")
                permission_handler.handle_user_response(req_id, action)
                continue

            if msg_type == "ping":
                await websocket.send_json({"type": "pong"})
                continue

    except WebSocketDisconnect:
        logger.info("WS disconnect: session=%s", session_id)
    except Exception as exc:
        logger.warning("WS error: session=%s reason=%s", session_id, exc)
    finally:
        writer_task.cancel()
        try:
            await asyncio.wait_for(writer_task, timeout=2)
        except (asyncio.TimeoutError, asyncio.CancelledError):
            pass
        # 保留 engine 在内存中等待重连（30分钟超时由 TaskManager 处理）
        logger.info("WS cleanup: session=%s", session_id)


async def _handle_user_message(engine, queue, session_id: str, content: str) -> None:
    """处理用户消息：驱动 QueryEngine 主循环并转发事件到 WebSocket。"""
    async for event in engine.submit_message(content):
        await queue.put(event)

        # 持久化消息
        if event["type"] in ("message_complete",):
            await session_manager.save_session(
                engine._build_state() if hasattr(engine, "_build_state") else _build_state_fallback(engine)
            )


def _build_state_fallback(engine: QueryEngine):
    """Fallback: 构建 SessionState 用于持久化。"""
    from app.models.session import SessionState
    return SessionState(
        session_id=engine.session_id,
        messages=engine.mutable_messages,
    )
