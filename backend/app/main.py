"""MiniCC FastAPI 入口 — 路由注册与应用生命周期。"""

from __future__ import annotations

from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.routing import APIRoute

from app.tools.base import ToolRegistry
from app.utils.config import settings
from app.utils.logger import logger


# -- 全局工具注册中心（Phase 2 填充）
tool_registry = ToolRegistry()


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期：初始化和清理。"""
    logger.info("MiniCC starting up — provider=%s model=%s", settings.llm_provider, settings.llm_model)
    # Phase 3: 初始化 Redis 连接池
    yield
    logger.info("MiniCC shutting down")
    # Phase 3: 清理资源


app = FastAPI(
    title="MiniCC API",
    version="0.1.0",
    lifespan=lifespan,
)

# CORS — 允许 Next.js 开发服务器
app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:3000", "http://127.0.0.1:3000"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# -- 路由 --

@app.get("/health")
async def health():
    return {"status": "ok"}


@app.get("/api/tools")
async def list_tools():
    """返回所有已注册工具（前端动态渲染用）。"""
    tools = tool_registry.to_anthropic_tools() if settings.llm_provider == "anthropic" else tool_registry.to_openai_tools()
    return {"tools": tools, "count": len(tools)}


@app.websocket("/ws/{session_id}")
async def agent_websocket(websocket, session_id: str):
    """WebSocket 主端点 — 会话生命周期入口。

    Phase 1: 初始化 QueryEngine
    Phase 2: 挂载权限回调
    """
    await websocket.accept()
    logger.info("WebSocket connected: session=%s", session_id)

    try:
        async for message in websocket.iter_text():
            # Phase 1: 路由消息到 QueryEngine
            pass
    except Exception as exc:
        logger.warning("WebSocket disconnected: session=%s reason=%s", session_id, exc)
    finally:
        logger.info("WebSocket cleaned up: session=%s", session_id)
