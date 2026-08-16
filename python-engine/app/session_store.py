"""
Session Store — Redis-backed 会话消息缓存（支持降级到内存）

职责：
1. 按 session_id 维护累积消息列表（system + 历史 + 工具调用）
2. 跨请求保持前缀稳定（append-only），让 DeepSeek prefix cache 命中
3. 多实例共享 Redis 后端，实现无状态扩展
4. Redis 不可用时降级到内存 LRU

使用方式：
    store = SessionStore(redis_client=redis_instance)
    messages = await store.get_or_init(session_id, history_from_go)
    await store.append(session_id, new_messages)
"""
from __future__ import annotations

import json
import logging
from collections import OrderedDict
from typing import Optional

logger = logging.getLogger(__name__)

REDIS_KEY_PREFIX = "session_cache:"
REDIS_TTL_SECONDS = 7200  # 2 小时


class SessionStore:
    """会话消息缓存，优先使用 Redis，不可用时降级到内存 LRU"""

    def __init__(self, redis_client=None, max_sessions: int = 200):
        self._redis = redis_client
        self._max_sessions = max_sessions
        # 内存降级后端（仅 Redis 不可用时使用）
        self._local: OrderedDict[str, list[dict]] = OrderedDict()

    # ── 公共接口 ──

    async def get_or_init(self, session_id: str, history: list[dict]) -> list[dict]:
        if not session_id:
            return list(history)

        # 尝试 Redis 后端
        if self._redis is not None:
            try:
                return await self._redis_get_or_init(session_id, history)
            except Exception as e:
                logger.warning("Redis session get failed, fallback to local: %s", e)
                self._redis = None  # 降级

        # 内存降级
        return self._local_get_or_init(session_id, history)

    async def append(self, session_id: str, messages: list[dict]) -> None:
        if not session_id:
            return

        if self._redis is not None:
            try:
                await self._redis_set(session_id, messages)
                return
            except Exception:
                self._redis = None

        self._local_set(session_id, messages)

    async def get(self, session_id: str) -> Optional[list[dict]]:
        if self._redis is not None:
            try:
                return await self._redis_get(session_id)
            except Exception:
                self._redis = None
        return self._local.get(session_id)

    async def remove(self, session_id: str) -> None:
        if self._redis is not None:
            try:
                await self._redis.delete(REDIS_KEY_PREFIX + session_id)
            except Exception:
                pass
        self._local.pop(session_id, None)

    async def clear(self) -> None:
        self._local.clear()
        if self._redis is not None:
            try:
                cursor = 0
                while True:
                    cursor, keys = await self._redis.scan(cursor, match=REDIS_KEY_PREFIX + "*", count=100)
                    if keys:
                        await self._redis.delete(*keys)
                    if cursor == 0:
                        break
            except Exception:
                pass

    @property
    def size(self) -> int:
        return len(self._local)

    # ── Redis 后端 ──

    async def _redis_get_or_init(self, session_id: str, history: list[dict]) -> list[dict]:
        data = await self._redis.get(REDIS_KEY_PREFIX + session_id)
        if data:
            # 延长 TTL
            await self._redis.expire(REDIS_KEY_PREFIX + session_id, REDIS_TTL_SECONDS)
            result = json.loads(data)
            logger.debug("Redis cache HIT: %s (%d messages)", session_id, len(result))
            return result

        logger.info("Redis cache MISS: %s (init from history: %d msgs)", session_id, len(history))
        messages = list(history)
        await self._redis_set(session_id, messages)
        return messages

    async def _redis_get(self, session_id: str) -> Optional[list[dict]]:
        data = await self._redis.get(REDIS_KEY_PREFIX + session_id)
        if data:
            return json.loads(data)
        return None

    async def _redis_set(self, session_id: str, messages: list[dict]) -> None:
        await self._redis.setex(
            REDIS_KEY_PREFIX + session_id,
            REDIS_TTL_SECONDS,
            json.dumps(messages, ensure_ascii=False, default=str),
        )

    # ── 内存降级后端 ──

    def _local_get_or_init(self, session_id: str, history: list[dict]) -> list[dict]:
        if session_id in self._local:
            self._local.move_to_end(session_id)
            cached = self._local[session_id]
            logger.debug("Local cache HIT: %s (%d messages)", session_id, len(cached))
            # 返回拷贝：调用方（runtime）会 append/修改返回列表，
            # 若直接返回缓存引用会污染共享状态，导致并发/多轮上下文错乱
            return list(cached)

        logger.info("Local cache MISS: %s (init from history: %d msgs)", session_id, len(history))
        messages = list(history)
        self._local[session_id] = messages
        self._evict_if_needed()
        return messages

    def _local_set(self, session_id: str, messages: list[dict]) -> None:
        self._local[session_id] = messages
        self._local.move_to_end(session_id)

    def _evict_if_needed(self) -> None:
        while len(self._local) > self._max_sessions:
            evicted_id, evicted = self._local.popitem(last=False)
            logger.info("Local cache EVICT: %s (%d messages)", evicted_id, len(evicted))
