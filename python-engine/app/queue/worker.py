# 队列消费者 — Redis Streams Consumer Group
from __future__ import annotations

import asyncio
import json
import logging
import time

import redis.asyncio as aioredis

from app.config import settings
from app.observability.metrics import (
    QUEUE_DEPTH,
    QUEUE_PROCESSING_DURATION,
    QUEUE_DLQ_TOTAL,
    QUEUE_RETRY_TOTAL,
)

logger = logging.getLogger(__name__)

TASK_STREAM = "engine:tasks"
DLQ_STREAM = "engine:tasks:dlq"
GROUP_NAME = "engine-workers"
CONSUMER_PREFIX = "worker"
MAX_RETRIES = 3


class QueueWorker:
    """
    Redis Streams 消费者

    生命周期:
      - 启动: XREADGROUP BLOCK 5s
      - 处理: ACK 成功 / NACK 失败
      - 重试: NACK 后超时 30s 可被 XCLAIM
      - 死信: retry_count >= 3 → XADD 到 DLQ
      - 关闭: 停止消费 → 等待 in-flight → 退出
    """

    def __init__(self, redis: aioredis.Redis, concurrency: int = 10):
        self._redis = redis
        self._concurrency = concurrency
        self._running = False
        self._semaphore = asyncio.Semaphore(concurrency)
        self._in_flight: set[asyncio.Task] = set()
        self._consumer_name = f"{CONSUMER_PREFIX}-{id(self):x}"

    async def start(self) -> None:
        """启动消费者"""
        self._running = True

        # 确保 Consumer Group 存在
        try:
            await self._redis.xgroup_create(TASK_STREAM, GROUP_NAME, id="0", mkstream=True)
            logger.info("Consumer group '%s' created", GROUP_NAME)
        except aioredis.ResponseError as e:
            if "BUSYGROUP" not in str(e):
                raise
            logger.debug("Consumer group '%s' already exists", GROUP_NAME)

        logger.info(
            "Queue worker started: consumer=%s concurrency=%d",
            self._consumer_name, self._concurrency,
        )

        while self._running:
            try:
                await self._consume_batch()
            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.warning("Queue worker error: %s", e)
                await asyncio.sleep(1)

    async def stop(self) -> None:
        """优雅停止"""
        self._running = False
        logger.info("Queue worker stopping, waiting for %d in-flight tasks...", len(self._in_flight))
        if self._in_flight:
            await asyncio.gather(*self._in_flight, return_exceptions=True)
        logger.info("Queue worker stopped")

    async def _consume_batch(self) -> None:
        """批量消费一批消息"""
        try:
            results = await self._redis.xreadgroup(
                GROUP_NAME,
                self._consumer_name,
                {TASK_STREAM: ">"},
                count=self._concurrency,
                block=5000,  # 5 秒超时
            )
        except asyncio.CancelledError:
            raise
        except Exception as e:
            logger.debug("XREADGROUP error: %s", e)
            return

        if not results:
            return

        for stream, messages in results:
            for stream_id, fields in messages:
                # 等待信号量
                await self._semaphore.acquire()
                task = asyncio.create_task(
                    self._process_message(stream_id, fields)
                )
                # 设置超时保护，防止 task 永久挂起
                timeout_task = asyncio.create_task(
                    asyncio.wait_for(task, timeout=3600)
                )
                self._in_flight.add(timeout_task)
                timeout_task.add_done_callback(self._task_done)

    def _task_done(self, task: asyncio.Task) -> None:
        self._in_flight.discard(task)
        self._semaphore.release()
        if task.exception():
            logger.error("Task exception: %s", task.exception())

    async def _process_message(self, stream_id: str, fields: dict) -> None:
        """处理单条消息"""
        task_type = fields.get(b"task_type", b"").decode() if isinstance(fields.get(b"task_type"), bytes) else fields.get("task_type", "")
        task_id = fields.get(b"task_id", b"").decode() if isinstance(fields.get(b"task_id"), bytes) else fields.get("task_id", "")
        payload_raw = fields.get(b"payload", b"{}").decode() if isinstance(fields.get(b"payload"), bytes) else fields.get("payload", "{}")
        retry_count = int(fields.get(b"retry_count", 0) if isinstance(fields.get(b"retry_count"), bytes) else fields.get("retry_count", 0))

        start = time.monotonic()
        try:
            payload = json.loads(payload_raw)
            await self._dispatch(task_type, payload)

            # 成功 → ACK
            await self._redis.xack(TASK_STREAM, GROUP_NAME, stream_id)
            elapsed = time.monotonic() - start
            QUEUE_PROCESSING_DURATION.labels(task_type=task_type).observe(elapsed)
            logger.info("Task completed: id=%s type=%s (%.2fs)", task_id, task_type, elapsed)

        except Exception as e:
            logger.error("Task failed: id=%s type=%s error=%s", task_id, task_type, e)

            if retry_count >= MAX_RETRIES:
                # 移入死信队列
                await self._redis.xadd(DLQ_STREAM, {
                    "task_id": task_id,
                    "task_type": task_type,
                    "payload": payload_raw,
                    "error": str(e),
                    "retry_count": str(retry_count),
                })
                await self._redis.xack(TASK_STREAM, GROUP_NAME, stream_id)
                QUEUE_DLQ_TOTAL.labels(task_type=task_type).inc()
                logger.warning("Task moved to DLQ: id=%s (retries=%d)", task_id, retry_count)
            else:
                # 重试：重新投递消息并递增 retry_count
                retry_count += 1
                await self._redis.xadd(TASK_STREAM, {
                    "task_type": task_type,
                    "task_id": task_id,
                    "payload": payload_raw,
                    "retry_count": str(retry_count),
                }, maxlen=10000)
                await self._redis.xack(TASK_STREAM, GROUP_NAME, stream_id)
                QUEUE_RETRY_TOTAL.labels(task_type=task_type).inc()
                logger.info("Task re-queued for retry: id=%s (retry=%d/%d)", task_id, retry_count, MAX_RETRIES)

    async def _dispatch(self, task_type: str, payload: dict) -> None:
        """分发任务到具体处理器"""
        if task_type == "rag_index":
            await self._handle_rag_index(payload)
        elif task_type == "memory_save":
            await self._handle_memory_save(payload)
        elif task_type == "embed_batch":
            await self._handle_embed_batch(payload)
        else:
            logger.warning("Unknown task type: %s", task_type)

    async def _handle_rag_index(self, payload: dict) -> None:
        """处理 RAG 文档索引任务"""
        # 实际实现会调用 RAGBuilder.build_document
        logger.info("Processing rag_index: doc_id=%s", payload.get("doc_id"))
        # TODO: 在此实现异步 RAG 索引

    async def _handle_memory_save(self, payload: dict) -> None:
        """处理记忆持久化任务"""
        logger.info("Processing memory_save: memory_id=%s", payload.get("memory_id"))
        # TODO: 在此实现异步记忆保存

    async def _handle_embed_batch(self, payload: dict) -> None:
        """处理批量嵌入任务"""
        logger.info("Processing embed_batch: count=%d", len(payload.get("texts", [])))
        # TODO: 在此实现批量嵌入

    async def update_queue_depth(self) -> None:
        """更新队列深度指标"""
        try:
            info = await self._redis.xinfo_stream(TASK_STREAM)
            QUEUE_DEPTH.labels(stream="engine:tasks").set(info.get("length", 0))
        except Exception:
            logger.warning("Failed to update queue depth metric")
