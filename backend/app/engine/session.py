"""SessionManager — 统一会话状态管理。

组合 Redis（热数据）和 SQLite（冷数据），提供会话恢复、消息持久化。
"""

from __future__ import annotations

import logging
from datetime import datetime, timezone
from typing import Optional

from app.models.chat import Message
from app.models.session import SessionState
from app.utils.redis_client import RedisClient
from app.utils.sqlite_store import SQLiteStore

logger = logging.getLogger("minicc.session")


class SessionManager:
    """会话状态管理层。统一管理热数据（Redis）和冷数据（SQLite）的读写。"""

    def __init__(self, redis_client: RedisClient, sqlite_store: SQLiteStore) -> None:
        self._redis = redis_client
        self._sqlite = sqlite_store

    async def save_message(self, session_id: str, message: Message) -> None:
        """同时写入 Redis（热）和 SQLite（冷）。"""
        await self._redis.append_message(session_id, message)
        await self._sqlite.save_message(session_id, message)

    async def save_session(self, state: SessionState) -> None:
        """持久化会话状态。"""
        now = datetime.now(timezone.utc)
        updated = SessionState(
            session_id=state.session_id,
            created_at=state.created_at,
            updated_at=now,
            messages=state.messages,
            pending_tool_calls=state.pending_tool_calls,
            approved_tool_calls=state.approved_tool_calls,
            metadata=state.metadata,
        )
        await self._redis.set_session_state(state.session_id, updated)
        await self._sqlite.save_session(updated)

    async def resume_session(self, session_id: str) -> Optional[SessionState]:
        """恢复会话。

        1. 尝试从 Redis 获取完整状态（用户刚断开的会话）
        2. 如果 Redis 没有，从 SQLite 重建基础状态
        3. 加载最近 5 条消息
        """
        # 1. 从 Redis 恢复
        state = await self._redis.get_session_state(session_id)
        if state:
            logger.info("Session restored from Redis: %s", session_id)
            return state

        # 2. 从 SQLite 重建
        base = await self._sqlite.get_session(session_id)
        if base is None:
            return None

        messages = await self._sqlite.get_messages(session_id)
        state = SessionState(
            session_id=session_id,
            created_at=base.created_at,
            updated_at=base.updated_at,
            messages=messages,
            metadata=base.metadata,
        )
        logger.info("Session restored from SQLite: %s (%d messages)", session_id, len(messages))
        return state

    async def list_sessions(self, limit: int = 50) -> list[SessionState]:
        """列出最近会话。"""
        return await self._sqlite.list_sessions(limit=limit)
