# 队列消费者 — Redis Streams Consumer Group
# P1-2: 信号处理与优雅关闭 (生产安全检查 2026-08-17)
from __future__ import annotations

import asyncio
import json
import logging
import signal
import sys
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

    def __init__(self, redis: aioredis.Redis, concurrency: int = 10, gateway=None, memory_service=None):
        self._redis = redis
        self._concurrency = concurrency
        self._gateway = gateway  # GatewayRouter（用于 RAG 构建/嵌入），可为 None
        self._memory_service = memory_service  # MemoryService（用于记忆存储），可为 None
        self._running = False
        self._semaphore = asyncio.Semaphore(concurrency)
        self._in_flight: set[asyncio.Task] = set()
        self._consumer_name = f"{CONSUMER_PREFIX}-{id(self):x}"

    async def start(self) -> None:
        """启动消费者"""
        self._running = True
        
        # P1-2: 注册信号处理器（支持 SIGTERM/Kill -15）
        await self._register_signal_handlers()

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
        """P1-2: 优雅停止（带超时保护）"""
        self._running = False
        logger.info("Queue worker stopping, waiting for %d in-flight tasks...", len(self._in_flight))
        if self._in_flight:
            # P1-2: 设置超时，防止 task 永久挂起
            try:
                await asyncio.wait_for(
                    asyncio.gather(*self._in_flight, return_exceptions=True),
                    timeout=30.0  # 最多等待 30 秒
                )
            except asyncio.TimeoutError:
                logger.error("Queue worker shutdown timed out, cancelling remaining tasks")
                for task in self._in_flight:
                    task.cancel()
        logger.info("Queue worker stopped")
    
    async def _register_signal_handlers(self) -> None:
        """P1-2: 注册 UNIX 信号处理器（Kubernetes Docker Pod 兼容）"""
        loop = asyncio.get_event_loop()
        
        def _signal_handler():
            """信号回调：触发异步停止"""
            asyncio.create_task(self.stop())
        
        try:
            loop.add_signal_handler(signal.SIGTERM, _signal_handler)
            loop.add_signal_handler(signal.SIGINT, _signal_handler)
            logger.info("Registered signal handlers for SIGTERM/SIGINT")
        except NotImplementedError:
            # Windows 不支持 add_signal_handler
            logger.debug("Signal handling not supported on this platform")

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
        tenant_id = fields.get(b"tenant_id", b"").decode() if isinstance(fields.get(b"tenant_id"), bytes) else fields.get("tenant_id", "")

        start = time.monotonic()
        try:
            payload = json.loads(payload_raw)
            await self._dispatch(task_type, payload, tenant_id)

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
                # 重试：重新投递消息并递增 retry_count（保留租户标识以保持观测链路）
                retry_count += 1
                retry_msg = {
                    "task_type": task_type,
                    "task_id": task_id,
                    "payload": payload_raw,
                    "retry_count": str(retry_count),
                }
                if tenant_id:
                    retry_msg["tenant_id"] = tenant_id
                await self._redis.xadd(TASK_STREAM, retry_msg, maxlen=10000)
                await self._redis.xack(TASK_STREAM, GROUP_NAME, stream_id)
                QUEUE_RETRY_TOTAL.labels(task_type=task_type).inc()
                logger.info("Task re-queued for retry: id=%s (retry=%d/%d)", task_id, retry_count, MAX_RETRIES)

    async def _dispatch(self, task_type: str, payload: dict, tenant_id: str = "") -> None:
        """分发任务到具体处理器"""
        if task_type == "rag_index":
            await self._handle_rag_index(payload)
        elif task_type == "memory_save":
            await self._handle_memory_save(payload, tenant_id)
        elif task_type == "memory_consolidate":
            await self._handle_memory_consolidate(payload, tenant_id)
        elif task_type == "memory_rollup":
            await self._handle_memory_rollup(payload, tenant_id)
        elif task_type == "embed_batch":
            await self._handle_embed_batch(payload)
        else:
            logger.warning("Unknown task type: %s", task_type)

    async def _handle_rag_index(self, payload: dict) -> None:
        """处理 RAG 文档索引任务：读库取内容 → RAGBuilder 构建 → 更新文档/KB 状态与扣费

        payload: {kb_id, user_id, documents: [{doc_id, file_type, filename}], estimated_cost}
        """
        from app.db import get_pool
        from app.rag.builder import RAGBuilder

        kb_id = payload.get("kb_id")
        user_id = payload.get("user_id")
        documents = payload.get("documents") or []
        estimated_cost = payload.get("estimated_cost", 0)
        if not kb_id or not documents:
            raise ValueError(f"rag_index payload 缺失: {payload}")

        pool = get_pool()
        # 幂等：任务级重试/重复投递时 KB 已激活则跳过（避免重复构建与扣费）
        kb = await pool.fetchrow("SELECT status FROM knowledge_bases WHERE id = $1", kb_id)
        if kb is None:
            # KB 已被删除（文档应随 CASCADE 清除）——直接失败进 DLQ，不误扣费
            raise ValueError(f"知识库不存在: {kb_id}")
        if kb["status"] == "active":
            logger.info("rag_index 跳过（KB 已激活）: kb_id=%s", kb_id)
            return

        builder = RAGBuilder(llm_gateway=self._gateway)
        errors = []

        for doc in documents:
            doc_id = doc.get("doc_id")
            row = await pool.fetchrow(
                "SELECT content, file_type, name FROM knowledge_documents WHERE id = $1",
                doc_id,
            )
            if row is None or row["content"] is None:
                errors.append(f"{doc_id}: 无内容")
                await pool.execute(
                    "UPDATE knowledge_documents SET status='error', error_message=$1 WHERE id=$2",
                    "no content", doc_id,
                )
                continue

            await pool.execute(
                "UPDATE knowledge_documents SET status='processing' WHERE id=$1", doc_id,
            )
            chunk_count = 0
            error_msg = None
            try:
                async for event in builder.build_document(
                    kb_id=kb_id,
                    doc_id=doc_id,
                    content=bytes(row["content"]),
                    file_type=doc.get("file_type") or row["file_type"] or "txt",
                    filename=doc.get("filename") or row["name"] or doc_id,
                    tenant_id=user_id or "",
                ):
                    if event.get("type") == "complete":
                        chunk_count = event.get("chunk_count", 0)
                    elif event.get("type") == "error":
                        error_msg = event.get("message")
            except Exception as e:
                error_msg = str(e)

            if error_msg:
                errors.append(f"{doc_id}: {error_msg}")
                await pool.execute(
                    "UPDATE knowledge_documents SET status='error', error_message=$1 WHERE id=$2",
                    error_msg, doc_id,
                )
            else:
                await pool.execute(
                    "UPDATE knowledge_documents SET status='completed', chunk_count=$1 WHERE id=$2",
                    chunk_count, doc_id,
                )

        # 全部文档成功才扣费并激活；有失败则置 error（与 wiki 分支「成功才激活」语义一致）
        async with pool.acquire() as conn:
            async with conn.transaction():
                if errors:
                    await conn.execute(
                        "UPDATE knowledge_bases SET status='error', updated_at=NOW() WHERE id=$1",
                        kb_id,
                    )
                else:
                    # 防负扣费：余额不足时事务回滚并报错（任务进 DLQ，不重复扣费）
                    res = await conn.execute(
                        "UPDATE users SET credits = credits - $1 WHERE id = $2 AND credits >= $1",
                        estimated_cost, user_id,
                    )
                    if not res or res.startswith("UPDATE 0"):
                        raise RuntimeError(
                            f"用户 {user_id} 积分不足，无法扣费 {estimated_cost}"
                        )
                    await conn.execute(
                        """UPDATE knowledge_bases
                           SET status = 'active', credits_consumed = credits_consumed + $1, updated_at = NOW()
                           WHERE id = $2""",
                        estimated_cost, kb_id,
                    )
        if errors:
            logger.warning("rag_index 部分文档失败，KB 置 error: %s", errors)

    async def _handle_memory_save(self, payload: dict, tenant_id: str = "") -> None:
        """处理记忆持久化任务

        payload: {key, value, source, user_id?, slot?, confidence?}
        """
        key = payload.get("key")
        if not key:
            raise ValueError("memory_save payload 缺少 key")
        if not self._memory_service:
            raise RuntimeError("memory_save 需要 memory_service 支持")

        from app.memory.layers import SlotType, SourceType

        slot = SlotType(payload.get("slot", "fact"))
        source = SourceType(payload.get("source", "derived"))
        confidence = int(payload.get("confidence", 50))
        user_id = payload.get("user_id", "")

        await self._memory_service.update_profile(
            tenant_id=tenant_id,
            user_id=user_id,
            slot=slot,
            item_key=key,
            item_value=str(payload.get("value", "")),
            confidence=confidence,
            source=source,
        )
        logger.info("memory_save 完成: key=%s tenant=%s", key, tenant_id)

    async def _handle_memory_consolidate(self, payload: dict, tenant_id: str) -> None:
        """处理记忆巩固任务：将对话消息巩固为 L3 摘要。

        payload: {session_id, user_id, turn_count, trigger}
        """
        if not self._memory_service:
            raise RuntimeError("memory_consolidate 需要 memory_service 支持")

        session_id = payload.get("session_id")
        user_id = payload.get("user_id", "")
        if not session_id:
            raise ValueError("memory_consolidate payload 缺少 session_id")

        # 从会话历史获取消息（简化：后续集成 ContextManager）
        messages = await self._get_session_messages(tenant_id, user_id, session_id)
        if not messages:
            logger.warning("No messages to consolidate: session=%s", session_id)
            return

        # 调用 Consolidator 进行巩固
        from app.memory.consolidator import Consolidator
        from app.memory.summary_store import SummaryStore

        if self._memory_service._summary_store is None:
            logger.debug("SummaryStore not available, skip consolidate")
            return

        consolidator = Consolidator(store=self._memory_service._summary_store)
        turn_count = payload.get("turn_count", 0)
        result = await consolidator.consolidate(
            tenant_id=tenant_id,
            user_id=user_id,
            session_id=session_id,
            messages=messages,
            turn_start=max(0, turn_count - 10),
            turn_end=turn_count,
        )

        if result.error:
            logger.error("Consolidate failed: %s", result.error)
        elif result.deduplicated:
            logger.debug("Consolidate deduplicated: session=%s", session_id)
        else:
            logger.info(
                "Consolidate completed: session=%s, summary_id=%s",
                session_id,
                result.summary.id if result.summary else None,
            )

    async def _handle_memory_rollup(self, payload: dict, tenant_id: str) -> None:
        """处理记忆 rollup 任务：会话结束时的总结归档。

        payload: {session_id, user_id, trigger}
        """
        if not self._memory_service:
            raise RuntimeError("memory_rollup 需要 memory_service 支持")

        session_id = payload.get("session_id")
        user_id = payload.get("user_id", "")
        if not session_id:
            raise ValueError("memory_rollup payload 缺少 session_id")

        # 获取会话全部消息
        messages = await self._get_session_messages(tenant_id, user_id, session_id)
        if not messages:
            logger.debug("No messages to rollup: session=%s", session_id)
            return

        # 调用 Consolidator 进行完整 rollup
        from app.memory.consolidator import Consolidator

        if self._memory_service._summary_store is None:
            logger.debug("SummaryStore not available, skip rollup")
            return

        consolidator = Consolidator(store=self._memory_service._summary_store)
        result = await consolidator.consolidate(
            tenant_id=tenant_id,
            user_id=user_id,
            session_id=session_id,
            messages=messages,
            turn_start=0,
            turn_end=len(messages),
        )

        if result.error:
            logger.error("Rollup failed: %s", result.error)
        else:
            logger.info(
                "Rollup completed: session=%s, messages=%d, summary_id=%s",
                session_id,
                len(messages),
                result.summary.id if result.summary else None,
            )

    async def _get_session_messages(
        self, tenant_id: str, user_id: str, session_id: str
    ) -> list[dict]:
        """获取会话消息（从数据库）。"""
        from app.db import get_pool

        try:
            pool = get_pool()
            rows = await pool.fetch(
                """SELECT role, content, created_at
                   FROM unified_messages
                   WHERE session_id = $1
                   ORDER BY created_at ASC""",
                session_id,
            )
            return [
                {
                    "role": row["role"],
                    "content": row["content"],
                }
                for row in rows
                if row["content"] and isinstance(row["content"], str)
            ]
        except Exception as e:
            logger.warning("Failed to get session messages: %s", e)
            return []

    async def _handle_embed_batch(self, payload: dict) -> None:
        """处理批量嵌入任务：批量计算嵌入并存储向量

        payload: {texts, kb_id, doc_id, tenant_id}
        """
        from app.rag.builder import RAGBuilder

        texts = payload.get("texts") or []
        if not texts:
            raise ValueError("embed_batch payload 缺少 texts")
        builder = RAGBuilder(llm_gateway=self._gateway)
        chunks = [{"index": i, "content": t} for i, t in enumerate(texts)]
        embeddings = await builder._compute_embeddings(chunks)
        await builder._store_vectors(
            payload.get("kb_id", ""),
            payload.get("doc_id", ""),
            payload.get("tenant_id", ""),
            chunks, embeddings,
            builder._vector_db_type,
        )
        logger.info("embed_batch 完成: count=%d", len(texts))

    async def update_queue_depth(self) -> None:
        """更新队列深度指标"""
        try:
            info = await self._redis.xinfo_stream(TASK_STREAM)
            QUEUE_DEPTH.labels(stream="engine:tasks").set(info.get("length", 0))
        except Exception:
            logger.warning("Failed to update queue depth metric")
