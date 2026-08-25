"""Trace Writer 鈥?灏?Agent/Workflow 鎵ц閾捐矾鍐欏叆 Redis Stream (鏃犵姸鎬佹墿灞?.

鑱岃矗:
1. 鎺ユ敹 AgentEvent(type="trace_span"),鎻愬彇 trace_id/span/duration
2. 鍐欏叆 Redis Stream `chiron:traces`
3. Go 渚у彲璁㈤槄璇?stream,鑱氬悎瀹屾暣璋冪敤閾?
澶氬疄渚嬮儴缃?
- 鎵€鏈?Python 瀹炰緥鍏变韩鍚屼竴 Redis Stream
- Go Gateway 缁熶竴璇诲彇 stream,涓嶄緷璧栬繘绋嬪唴瀛?"""
from __future__ import annotations

import asyncio
import json
import logging
import time
from typing import Any, Optional

from app.middleware.privacy_middleware import is_no_retention

logger = logging.getLogger(__name__)

# Redis Stream 閿悕 (SaaS: 鎸?tenant_id 闅旂)
TRACES_STREAM_TPL = "chiron:traces:{}"  # {} 灏嗚 tenant_id 鏇挎崲


def get_tenant_stream(tenant_id: str) -> str:
    """鏍规嵁 tenant_id 鐢熸垚闅旂鐨?Redis Stream 閿悕."""
    if not tenant_id:
        return TRACES_STREAM_TPL.format("anonymous")  # 鍖垮悕绉熸埛榛樿 key
    return TRACES_STREAM_TPL.format(tenant_id)


class TraceWriter:
    """寮傛 trace writer (鍗曚緥妯″紡,閬垮厤閲嶅杩炴帴 Redis)."""
    
    _instance: Optional["TraceWriter"] = None
    _redis: Optional[Any] = None
    
    @classmethod
    async def get_instance(cls, redis_url: Optional[str] = None) -> "TraceWriter":
        """鑾峰彇鍏ㄥ眬鍞竴 TraceWriter 瀹炰緥."""
        if cls._instance is None:
            cls._instance = cls()
            # 寤惰繜鍒濆鍖?Redis 杩炴帴 (閬垮厤鍚姩鏈熼樆濉?
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
        """鍏抽棴 Redis 杩炴帴."""
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
        tenant_id: Optional[str] = None,  # SaaS: 绉熸埛闅旂
    ) -> None:
        """鍐欏叆鍗曚釜 span 浜嬩欢鍒?Redis Stream (鎸夌鎴烽殧绂?.
        
        Args:
            trace_id: 鐢ㄦ埛璇锋眰鐨勫敮涓€鏍囪瘑
            span_name: span 鍚嶇О (llm_call / tool_execution / workflow_node)
            duration_ms: span 鑰楁椂 (姣)
            metadata: 棰濆涓婁笅鏂?(model, input_tokens, tool_name 绛?
            tenant_id: 绉熸埛 ID (鐢ㄤ簬娴侀殧绂?鍙€?
        """
        # 闅愮妯″紡: no_retention 鏃惰烦杩?trace 鍐欏叆 (涓嶈惤鐩?
        if is_no_retention():
            logger.debug("Privacy no_retention: skip trace span write: %s", span_name)
            return

        if self._redis is None:
            logger.debug("TraceWriter disabled (Redis not available), skipping span: %s", span_name)
            return
        
        try:
            # SaaS 瀹夊叏: 鎸?tenant_id 鍒?stream
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
        """鎵归噺鍐欏叆澶氫釜 span (鍑忓皯 Redis 寰€杩?.
        
        Args:
            spans: 姣忎釜 dict 鍖呭惈 trace_id, span_name, duration_ms, metadata
        """
        if not spans:
            return

        # 闅愮妯″紡: no_retention 鏃惰烦杩?trace 鍐欏叆 (涓嶈惤鐩?
        if is_no_retention():
            logger.debug("Privacy no_retention: skip trace batch write (%d spans)", len(spans))
            return

        if self._redis is None:
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
            for span, entry in zip(spans, entries):
                # SaaS 瀹夊叏: 鎸?tenant_id 鍒?stream锛堜笌 write_span 涓€鑷达級
                stream = get_tenant_stream(span.get("tenant_id") or "anonymous")
                pipeline.xadd(stream, entry, maxlen=10000, approximate=True)
            await pipeline.execute()
            
            logger.debug("TraceWriter wrote %d spans to Redis", len(entries))
        except Exception as e:
            logger.warning("TraceWriter batch write failed: %s", e)


# 鈹€鈹€ 渚挎嵎鍑芥暟 (渚?AgentRuntime 鐩存帴璋冪敤) 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

_trace_writer: Optional[TraceWriter] = None
_write_lock = asyncio.Lock()


async def record_span(
    trace_id: str,
    span_name: str,
    duration_ms: int,
    metadata: Optional[dict] = None,
    tenant_id: Optional[str] = None,  # SaaS: 绉熸埛闅旂
    redis_url: Optional[str] = None,
) -> None:
    """渚挎嵎鍑芥暟: 璁板綍鍗曚釜 span 鍒?Redis Stream (甯︾鎴烽殧绂?.
    
    鑷姩鍒濆鍖?TraceWriter (鍗曚緥),纭繚棣栨璋冪敤鏃跺缓绔?Redis 杩炴帴.
    
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

