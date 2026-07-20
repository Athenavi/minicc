# 死信队列管理
from __future__ import annotations

import json
import logging

import redis.asyncio as aioredis

logger = logging.getLogger(__name__)

DLQ_STREAM = "engine:tasks:dlq"


class DeadLetterQueue:
    """死信队列管理 — 查看、重试、清理"""

    def __init__(self, redis: aioredis.Redis):
        self._redis = redis

    async def list(self, count: int = 50) -> list[dict]:
        """列出死信消息"""
        results = await self._redis.xrevrange(DLQ_STREAM, count=count)
        messages = []
        for stream_id, fields in results:
            msg = {}
            for k, v in fields.items():
                key = k.decode() if isinstance(k, bytes) else k
                val = v.decode() if isinstance(v, bytes) else v
                msg[key] = val
            msg["stream_id"] = stream_id
            messages.append(msg)
        return messages

    async def retry(self, stream_id: str) -> bool:
        """将死信消息重新入队"""
        # 读取消息
        results = await self._redis.xrange(DLQ_STREAM, min=stream_id, max=stream_id)
        if not results:
            return False

        _, fields = results[0]
        # 重建消息
        message = {}
        for k, v in fields.items():
            key = k.decode() if isinstance(k, bytes) else k
            val = v.decode() if isinstance(v, bytes) else v
            if key not in ("error", "stream_id"):
                message[key] = val
        message["retry_count"] = "0"

        # 重新入队
        await self._redis.xadd("engine:tasks", message)
        # 从 DLQ 删除
        await self._redis.xdel(DLQ_STREAM, stream_id)
        logger.info("DLQ message requeued: %s", stream_id)
        return True

    async def retry_all(self) -> int:
        """重试所有死信消息"""
        messages = await self.list(count=1000)
        count = 0
        for msg in messages:
            if await self.retry(msg["stream_id"]):
                count += 1
        return count

    async def clear(self) -> int:
        """清空死信队列"""
        messages = await self.list(count=10000)
        count = 0
        for msg in messages:
            await self._redis.xdel(DLQ_STREAM, msg["stream_id"])
            count += 1
        return count

    async def depth(self) -> int:
        """死信队列深度"""
        try:
            info = await self._redis.xinfo_stream(DLQ_STREAM)
            return info.get("length", 0)
        except Exception:
            return 0
