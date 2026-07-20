# 队列生产者 — 发布任务到 Redis Streams
from __future__ import annotations

import json
import logging
import time
import uuid

import redis.asyncio as aioredis

from app.observability.logging import trace_id_var

logger = logging.getLogger(__name__)

# Stream 名称
TASK_STREAM = "engine:tasks"
DLQ_STREAM = "engine:tasks:dlq"


class QueueProducer:
    """Redis Streams 任务发布者"""

    def __init__(self, redis: aioredis.Redis):
        self._redis = redis

    async def enqueue(
        self,
        task_type: str,
        tenant_id: str,
        payload: dict,
        priority: int = 0,
    ) -> str:
        """
        发布任务到 Redis Streams

        Args:
            task_type: "rag_index" | "memory_save" | "embed_batch"
            tenant_id: 租户 ID
            payload: 任务数据
            priority: 优先级（0=普通，1=高）

        Returns:
            task_id
        """
        task_id = uuid.uuid4().hex[:16]
        trace_id = trace_id_var.get("")

        message = {
            "task_id": task_id,
            "task_type": task_type,
            "tenant_id": tenant_id,
            "payload": json.dumps(payload, ensure_ascii=False),
            "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "retry_count": "0",
            "trace_id": trace_id,
            "priority": str(priority),
        }

        stream_id = await self._redis.xadd(TASK_STREAM, message, maxlen=100000)
        logger.info("Task enqueued: id=%s type=%s stream_id=%s", task_id, task_type, stream_id)
        return task_id

    async def get_depth(self) -> int:
        """查询队列深度"""
        info = await self._redis.xinfo_stream(TASK_STREAM)
        return info.get("length", 0)
