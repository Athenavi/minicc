"""Trace Writer — 将 Agent/Workflow 执行链路写入 Redis Stream (无状态扩展).

职责:
1. 接收 AgentEvent(type="trace_span"),提取 trace_id/span/duration
2. 写入 Redis Stream `minicc:traces`
3. Go 侧可订阅该 stream,聚合完整调用链

多实例部署:
- 所有 Python 实例共享同一 Redis Stream
- Go Gateway 统一读取 stream,不依赖进程内存
"""
from __future__ import annotations

import asyncio
import json
import logging
import time
from typing import Optional

logger = logging.getLogger(__name__)

# Redis Stream 键名 (SaaS: 按 tenant_id 隔离)
TRACES_STREAM_TPL = "minicc:traces:{}"  # {} 将被 tenant_id 替换


def get_tenant_stream(tenant_id: str) -> str:
    """根据 tenant_id 生成隔离的 Redis Stream 键名."""
    if not tenant_id:
        return TRACES_STREAM_TPL.format("anonymous")  # 匿名租户默认 key
    return TRACES_STREAM_TPL.format(tenant_id)


class TraceWriter:
    """异步 trace writer (单例模式,避免重复连接 Redis)."""
    
    _instance: Optional["TraceWriter"] = None
    _redis: Optional[Any] = None
    
    @classmethod
    async def get_instance(cls, redis_url: Optional[str] = None) -> "TraceWriter":
        """获取全局唯一 TraceWriter 实例."""
        if cls._instance is None:
            cls._instance = cls()
            # 延迟初始化 Redis 连接 (避免启动期阻塞)
            if redis_url:
                import redis.asyncio as aioredis
                cls._redis = aioredis.from_url(redis_url, decode_responses=True)
                try:
                    await cls._redis.ping()
                    logger.info("TraceWriter connected to Redis: %s", redis_url)
                except Exception as e:
                    logger.warning("TraceWriter Redis ping failed: %s (traces will be lost)", e)
                    cls._redis = None
        return cls._instance
    
    @classmethod
    async def close(cls) -> None:
        """关闭 Redis 连接."""
        if cls._redis:
            await cls._redis.close()
            cls._redis = None
            cls._instance = None
    
    async def write_span(
        self,
        trace_id: str,
        span_name: str,
        duration_ms: int,
        metadata: Optional[dict] = None,
        tenant_id: Optional[str] = None,  # SaaS: 租户隔离
    ) -> None:
        """写入单个 span 事件到 Redis Stream (按租户隔离).
        
        Args:
            trace_id: 用户请求的唯一标识
            span_name: span 名称 (llm_call / tool_execution / workflow_node)
            duration_ms: span 耗时 (毫秒)
            metadata: 额外上下文 (model, input_tokens, tool_name 等)
            tenant_id: 租户 ID (用于流隔离,可选)
        """
        if self._redis is None:
            logger.debug("TraceWriter disabled (Redis not available), skipping span: %s", span_name)
            return
        
        try:
            # SaaS 安全: 按 tenant_id 分 stream
            stream = get_tenant_stream(tenant_id or "anonymous")
            entry = {
                "trace_id": trace_id,
                "span_name": span_name,
                "duration_ms": str(duration_ms),
                "timestamp": str(time.time()),
                "tenant_id": tenant_id or "",
                "metadata": json.dumps(metadata or {}, ensure_ascii=False),
            }
            await self._redis.xadd(stream, entry, maxlen=10000, approximate=True)
        except Exception as e:
            logger.warning("TraceWriter failed to write span (tenant=%s): %s", tenant_id, e)
    
    async def write_batch(self, spans: list[dict]) -> None:
        """批量写入多个 span (减少 Redis 往返).
        
        Args:
            spans: 每个 dict 包含 trace_id, span_name, duration_ms, metadata
        """
        if not spans or self._redis is None:
            return
        
        try:
            entries = []
            for s in spans:
                entry = {
                    "trace_id": s["trace_id"],
                    "span_name": s["span_name"],
                    "duration_ms": str(s["duration_ms"]),
                    "timestamp": str(time.time()),
                    "metadata": json.dumps(s.get("metadata", {}), ensure_ascii=False),
                }
                entries.append(entry)
            
            pipeline = self._redis.pipeline(transaction=False)
            for entry in entries:
                pipeline.xadd(TRACES_STREAM, entry, maxlen=10000, approximate=True)
            await pipeline.execute()
            
            logger.debug("TraceWriter wrote %d spans to Redis", len(entries))
        except Exception as e:
            logger.warning("TraceWriter batch write failed: %s", e)


# ── 便捷函数 (供 AgentRuntime 直接调用) ─────────────────────────────────────

_trace_writer: Optional[TraceWriter] = None
_write_lock = asyncio.Lock()


async def record_span(
    trace_id: str,
    span_name: str,
    duration_ms: int,
    metadata: Optional[dict] = None,
    tenant_id: Optional[str] = None,  # SaaS: 租户隔离
    redis_url: Optional[str] = None,
) -> None:
    """便捷函数: 记录单个 span 到 Redis Stream (带租户隔离).
    
    自动初始化 TraceWriter (单例),确保首次调用时建立 Redis 连接.
    
    Example:
        await record_span(
            trace_id="abc123",
            span_name="llm_call",
            duration_ms=1500,
            metadata={"model": "gpt-4", "input_tokens": 500},
            tenant_id="tenant_456",
        )
    """
    global _trace_writer
    
    if _trace_writer is None:
        async with _write_lock:
            if _trace_writer is None:
                _trace_writer = await TraceWriter.get_instance(redis_url)
    
    await _trace_writer.write_span(trace_id, span_name, duration_ms, metadata, tenant_id)
