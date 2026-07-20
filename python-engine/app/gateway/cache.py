# 三级语义缓存: L1 进程内 LRU + L2 Redis 精确 + L3 Redis 语义
from __future__ import annotations

import hashlib
import json
import logging
import time
from collections import OrderedDict
from typing import Optional

import redis.asyncio as aioredis

from app.gateway.provider import ChatMessage, ChatResponse

logger = logging.getLogger(__name__)


class SemanticCache:
    """
    L1: 进程内 LRU (2048 条, <0.1ms)
    L2: Redis 精确匹配 (TTL 3600s, ~1ms)
    L3: Redis 语义匹配 (embedding cosine > threshold, ~5ms)

    仅对「无工具调用」的纯文本请求启用。
    """

    def __init__(
        self,
        redis: aioredis.Redis,
        embed_fn,  # Callable[[str], Awaitable[list[float]]]
        l1_capacity: int = 2048,
        l2_ttl: int = 3600,
        semantic_threshold: float = 0.95,
        semantic_prefix_dims: int = 64,
    ):
        self._redis = redis
        self._embed_fn = embed_fn
        self._l2_ttl = l2_ttl
        self._semantic_threshold = semantic_threshold
        self._semantic_prefix_dims = semantic_prefix_dims

        # L1: LRU dict
        self._l1: OrderedDict[str, dict] = OrderedDict()
        self._l1_capacity = l1_capacity

        # 统计
        self._hits_l1 = 0
        self._hits_l2 = 0
        self._hits_l3 = 0
        self._misses = 0

    # ── key 计算 ──

    @staticmethod
    def _exact_key(model: str, messages: list[ChatMessage], tools: list[dict] | None, temperature: float) -> str:
        """精确缓存 key: hash(model + messages + tools + temperature)"""
        payload = {
            "model": model,
            "messages": [m.to_dict() for m in messages],
            "tools": tools,
            "temperature": temperature,
        }
        raw = json.dumps(payload, sort_keys=True, ensure_ascii=False)
        return hashlib.sha256(raw.encode()).hexdigest()[:32]

    async def _semantic_key(self, messages: list[ChatMessage], model: str) -> str | None:
        """语义缓存 key: embedding 前 64 维量化 → hash"""
        try:
            # 取最后 3 轮对话作为语义摘要
            recent = messages[-6:] if len(messages) > 6 else messages
            text = "\n".join(f"{m.role}:{m.content}" for m in recent if m.content)
            if len(text) < 10:
                return None

            embedding = await self._embed_fn(text)
            if not embedding:
                return None

            # 取前 N 维，量化为 int8
            prefix = embedding[: self._semantic_prefix_dims]
            quantized = bytes(
                max(0, min(255, int((v + 1) * 127.5))) for v in prefix
            )
            h = hashlib.sha256(quantized + model.encode()).hexdigest()[:16]
            return h
        except Exception as e:
            logger.debug("Semantic key computation failed: %s", e)
            return None

    # ── 查找 ──

    async def lookup(
        self,
        model: str,
        messages: list[ChatMessage],
        tools: list[dict] | None,
        temperature: float,
    ) -> Optional[ChatResponse]:
        """按 L1 → L2 → L3 顺序查找缓存"""
        # 不缓存带工具调用的请求
        if tools:
            return None

        exact = self._exact_key(model, messages, tools, temperature)

        # L1
        if exact in self._l1:
            self._l1.move_to_end(exact)
            self._hits_l1 += 1
            data = self._l1[exact]
            return self._decode(data, model)

        # L2
        raw = await self._redis.get(f"llm:cache:{exact}")
        if raw:
            data = json.loads(raw)
            self._l1_set(exact, data)
            self._hits_l2 += 1
            return self._decode(data, model)

        # L3
        sem_key = await self._semantic_key(messages, model)
        if sem_key:
            bucket_key = f"llm:semantic:{sem_key[:8]}"
            cached = await self._redis.hget(bucket_key, sem_key)
            if cached:
                data = json.loads(cached)
                self._l1_set(exact, data)
                self._hits_l3 += 1
                return self._decode(data, model)

        self._misses += 1
        return None

    # ── 存储 ──

    async def store(
        self,
        model: str,
        messages: list[ChatMessage],
        tools: list[dict] | None,
        temperature: float,
        response: ChatResponse,
    ) -> None:
        """存储到 L1 + L2 + L3"""
        if tools:
            return

        exact = self._exact_key(model, messages, tools, temperature)
        data = self._encode(response)

        # L1
        self._l1_set(exact, data)

        # L2
        await self._redis.set(
            f"llm:cache:{exact}",
            json.dumps(data, ensure_ascii=False),
            ex=self._l2_ttl,
        )

        # L3 (语义)
        sem_key = await self._semantic_key(messages, model)
        if sem_key:
            bucket_key = f"llm:semantic:{sem_key[:8]}"
            await self._redis.hset(
                bucket_key, sem_key, json.dumps(data, ensure_ascii=False)
            )
            await self._redis.expire(bucket_key, self._l2_ttl)

    # ── 内部 ──

    def _l1_set(self, key: str, data: dict) -> None:
        self._l1[key] = data
        self._l1.move_to_end(key)
        while len(self._l1) > self._l1_capacity:
            self._l1.popitem(last=False)

    @staticmethod
    def _encode(resp: ChatResponse) -> dict:
        return {
            "content": resp.content,
            "tool_calls": [
                {"id": tc.id, "name": tc.name, "arguments": tc.arguments}
                for tc in resp.tool_calls
            ],
            "finish_reason": resp.finish_reason,
            "input_tokens": resp.input_tokens,
            "output_tokens": resp.output_tokens,
            "cached_at": time.time(),
        }

    @staticmethod
    def _decode(data: dict, model: str) -> ChatResponse:
        from app.gateway.provider import ToolCall

        return ChatResponse(
            content=data.get("content", ""),
            tool_calls=[
                ToolCall(id=tc["id"], name=tc["name"], arguments=tc["arguments"])
                for tc in data.get("tool_calls", [])
            ],
            finish_reason=data.get("finish_reason", ""),
            input_tokens=data.get("input_tokens", 0),
            output_tokens=data.get("output_tokens", 0),
            model=model,
            provider="cache",
        )

    def stats(self) -> dict:
        total = self._hits_l1 + self._hits_l2 + self._hits_l3 + self._misses
        return {
            "l1_hits": self._hits_l1,
            "l2_hits": self._hits_l2,
            "l3_hits": self._hits_l3,
            "misses": self._misses,
            "hit_rate": (self._hits_l1 + self._hits_l2 + self._hits_l3) / max(total, 1),
        }
