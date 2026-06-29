"""Redis 存储层 — 会话热数据缓存，2h TTL。

存储结构：
- session:{id}:state → JSON (当前会话状态)
- session:{id}:messages → List (最近 N 条消息)
"""

from __future__ import annotations

import json
import logging
from typing import Optional

from app.models.chat import Message
from app.models.session import SessionState

logger = logging.getLogger("minicc.redis")


class RedisClient:
    """Redis 连接池管理器。可选依赖 — 无 Redis 时降级为内存模式。"""

    def __init__(self, redis_url: str) -> None:
        self._url = redis_url
        self._redis = None

    async def connect(self) -> bool:
        """连接到 Redis。返回是否连接成功。"""
        try:
            import redis.asyncio as aredis
            self._redis = await aredis.from_url(self._url, decode_responses=True)
            await self._redis.ping()
            logger.info("Redis connected: %s", self._url)
            return True
        except Exception as exc:
            logger.warning("Redis unavailable (running in memory-only mode): %s", exc)
            self._redis = None
            return False

    async def disconnect(self) -> None:
        if self._redis:
            await self._redis.close()

    @property
    def available(self) -> bool:
        return self._redis is not None

    # -- Session state --

    async def set_session_state(self, session_id: str, state: SessionState, ttl: int = 7200) -> None:
        if not self._redis:
            return
        key = f"session:{session_id}:state"
        await self._redis.setex(key, ttl, state.model_dump_json())

    async def get_session_state(self, session_id: str) -> Optional[SessionState]:
        if not self._redis:
            return None
        key = f"session:{session_id}:state"
        data = await self._redis.get(key)
        if data is None:
            return None
        return SessionState.model_validate_json(data)

    # -- Messages --

    async def append_message(self, session_id: str, message: Message) -> None:
        if not self._redis:
            return
        key = f"session:{session_id}:messages"
        await self._redis.rpush(key, message.model_dump_json())

    async def get_recent_messages(self, session_id: str, count: int = 5) -> list[Message]:
        if not self._redis:
            return []
        key = f"session:{session_id}:messages"
        items = await self._redis.lrange(key, -count, -1)
        return [Message.model_validate_json(item) for item in items]

    async def delete_session(self, session_id: str) -> None:
        if not self._redis:
            return
        await self._redis.delete(f"session:{session_id}:state", f"session:{session_id}:messages")
