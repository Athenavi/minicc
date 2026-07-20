# Gateway 路由器 — 多 Provider 加权选择 + 熔断 + 降级
from __future__ import annotations

import logging
import time
import random
from typing import AsyncIterator, Optional

from app.gateway.provider import (
    ChatMessage,
    ChatResponse,
    EmbeddingResponse,
    LLMProvider,
)
from app.gateway.circuit_breaker import CircuitBreaker
from app.gateway.cache import SemanticCache
from app.gateway.budget import TokenBudget
from app.gateway.ratelimit import TenantRateLimiter

logger = logging.getLogger(__name__)


class GatewayRouter:
    """
    核心路由器 — 所有 LLM 调用的统一入口。

    路由决策:
      1. provider_hint 指定 → 直接用（如果未熔断）
      2. 找到支持 model 的所有 providers
      3. 过滤已熔断的
      4. 加权随机选择: score = w_cost * (1/price) + w_latency * (1/latency) + w_quality
      5. 全部熔断 → 尝试 fallback provider

    缓存: chat 请求自动走 SemanticCache
    预算: chat 请求自动扣减 TokenBudget
    限流: 调用方在 middleware 层做，此处不重复
    """

    # Provider 默认定价 ($/1M tokens)，用于加权路由
    DEFAULT_COST: dict[str, float] = {
        "anthropic": 3.0,
        "openai": 5.0,
        "deepseek": 0.2,
    }
    DEFAULT_QUALITY: dict[str, float] = {
        "anthropic": 0.95,
        "openai": 0.90,
        "deepseek": 0.80,
    }

    def __init__(
        self,
        providers: dict[str, LLMProvider],
        cache: SemanticCache | None = None,
        budget: TokenBudget | None = None,
        weights: dict[str, float] | None = None,
    ):
        self._providers = providers
        self._breakers = {name: CircuitBreaker() for name in providers}
        self._latencies: dict[str, float] = {name: 500.0 for name in providers}  # moving avg ms
        self._cache = cache
        self._budget = budget
        # 路由权重: cost / latency / quality
        self._weights = weights or {"cost": 0.3, "latency": 0.3, "quality": 0.4}

    # ── 公开 API ──

    async def chat_stream(
        self,
        messages: list[ChatMessage],
        model: str,
        *,
        tenant_id: str = "",
        provider_hint: str = "",
        max_tokens: int = 4096,
        temperature: float = 0.7,
        tools: list[dict] | None = None,
    ) -> AsyncIterator[ChatResponse]:
        """流式推理 — 前置缓存检查 + 预算扣减"""
        # ── 缓存查找（仅无工具时） ──
        if self._cache and not tools:
            cached = await self._cache.lookup(model, messages, tools, temperature)
            if cached:
                logger.info("Stream cache HIT for model=%s", model)
                yield cached
                return

        provider = await self._select(model, provider_hint)
        if not provider:
            yield ChatResponse(
                content="", finish_reason="error", provider="none", model=model,
            )
            return

        # 预算预检查
        estimated = max_tokens  # 粗估
        if self._budget and tenant_id:
            ok = await self._budget.check(tenant_id, estimated)
            if not ok:
                yield ChatResponse(
                    content="", finish_reason="error", provider=provider.name, model=model,
                )
                return

        start = time.monotonic()
        full_content = ""
        full_tool_calls = []
        finish_reason = ""
        total_input = 0
        total_output = 0
        try:
            async for chunk in provider.chat_stream(
                messages, model,
                max_tokens=max_tokens, temperature=temperature, tools=tools,
            ):
                chunk.provider = provider.name
                chunk.model = model
                # 累积用于缓存
                if chunk.content:
                    full_content += chunk.content
                if chunk.tool_calls:
                    full_tool_calls.extend(chunk.tool_calls)
                if chunk.finish_reason:
                    finish_reason = chunk.finish_reason
                if chunk.input_tokens:
                    total_input += chunk.input_tokens
                if chunk.output_tokens:
                    total_output += chunk.output_tokens
                yield chunk

            self._breakers[provider.name].record_success()

            # ── 流完成后存入缓存（仅无工具时） ──
            if self._cache and not tools and finish_reason != "error" and full_content:
                resp = ChatResponse(
                    content=full_content,
                    finish_reason=finish_reason,
                    provider=provider.name,
                    model=model,
                    input_tokens=total_input,
                    output_tokens=total_output,
                )
                await self._cache.store(model, messages, tools, temperature, resp)
        except Exception as e:
            self._breakers[provider.name].record_failure()
            logger.error("Provider %s chat_stream failed: %s", provider.name, e)
            yield ChatResponse(
                content=str(e), finish_reason="error", provider=provider.name, model=model,
            )
        finally:
            elapsed = (time.monotonic() - start) * 1000
            self._latencies[provider.name] = (
                self._latencies[provider.name] * 0.8 + elapsed * 0.2
            )

    async def chat(
        self,
        messages: list[ChatMessage],
        model: str,
        *,
        tenant_id: str = "",
        provider_hint: str = "",
        max_tokens: int = 4096,
        temperature: float = 0.7,
        tools: list[dict] | None = None,
    ) -> ChatResponse:
        """非流式推理 — 自动缓存 + 预算扣减"""
        # 缓存查找
        if self._cache and not tools:
            cached = await self._cache.lookup(model, messages, tools, temperature)
            if cached:
                logger.info("Cache hit for model=%s", model)
                return cached

        provider = await self._select(model, provider_hint)
        if not provider:
            return ChatResponse(
                content="", finish_reason="error", provider="none", model=model,
            )

        # 预算预检查
        if self._budget and tenant_id:
            ok = await self._budget.check(tenant_id, max_tokens)
            if not ok:
                return ChatResponse(
                    content="", finish_reason="error", provider=provider.name, model=model,
                )

        start = time.monotonic()
        try:
            resp = await provider.chat(
                messages, model,
                max_tokens=max_tokens, temperature=temperature, tools=tools,
            )
            resp.provider = provider.name
            resp.model = model

            self._breakers[provider.name].record_success()

            # 存缓存
            if self._cache and not tools and resp.finish_reason != "error":
                await self._cache.store(model, messages, tools, temperature, resp)

            # 预算扣减
            if self._budget and tenant_id and resp.input_tokens + resp.output_tokens > 0:
                await self._budget.deduct(tenant_id, resp.input_tokens + resp.output_tokens)

            return resp
        except Exception as e:
            self._breakers[provider.name].record_failure()
            logger.error("Provider %s chat failed: %s", provider.name, e)
            return ChatResponse(
                content="", finish_reason="error", provider=provider.name, model=model,
            )
        finally:
            elapsed = (time.monotonic() - start) * 1000
            self._latencies[provider.name] = (
                self._latencies[provider.name] * 0.8 + elapsed * 0.2
            )

    async def embed(self, text: str, model: str, provider_hint: str = "") -> EmbeddingResponse:
        """文本嵌入 — 无缓存无预算"""
        provider = await self._select(model, provider_hint)
        if not provider:
            return EmbeddingResponse()

        try:
            resp = await provider.embed(text, model)
            self._breakers[provider.name].record_success()
            return resp
        except Exception as e:
            self._breakers[provider.name].record_failure()
            logger.error("Provider %s embed failed: %s", provider.name, e)
            return EmbeddingResponse()

    async def close(self) -> None:
        for p in self._providers.values():
            await p.close()

    def stats(self) -> dict:
        """返回路由器统计"""
        return {
            "providers": {
                name: {
                    "circuit_state": self._breakers[name].state.value,
                    "avg_latency_ms": round(self._latencies[name], 1),
                }
                for name in self._providers
            },
            "cache": self._cache.stats() if self._cache else None,
        }

    # ── 内部路由 ──

    async def _select(self, model: str, hint: str = "") -> Optional[LLMProvider]:
        """选择 Provider"""
        if hint and hint in self._providers:
            if self._breakers[hint].allow():
                return self._providers[hint]
            logger.warning("Hinted provider %s is circuit-open, falling back", hint)

        # 找到所有支持该 model 的 provider
        candidates = self._find_candidates(model)
        if not candidates:
            # 全部尝试
            candidates = list(self._providers.values())

        # 过滤熔断
        available = [
            p for p in candidates if self._breakers[p.name].allow()
        ]

        if not available:
            logger.error("All providers circuit-open for model=%s", model)
            return None

        # 加权选择
        return self._weighted_select(available)

    def _find_candidates(self, model: str) -> list[LLMProvider]:
        """按 model 前缀匹配 provider"""
        model_lower = model.lower()
        if "claude" in model_lower and "anthropic" in self._providers:
            return [self._providers["anthropic"]]
        if "deepseek" in model_lower and "deepseek" in self._providers:
            return [self._providers["deepseek"]]
        # gpt / o1 / o3 / embedding → openai
        if ("gpt" in model_lower or "o1" in model_lower or "o3" in model_lower or "embedding" in model_lower) and "openai" in self._providers:
            return [self._providers["openai"]] if "openai" in self._providers else []
        # 兜底: 优先用 openai（最通用）
        if "openai" in self._providers:
            return [self._providers["openai"]]
        return list(self._providers.values())

    def _weighted_select(self, providers: list[LLMProvider]) -> LLMProvider:
        """加权随机选择"""
        if len(providers) == 1:
            return providers[0]

        scores = []
        for p in providers:
            cost = self.DEFAULT_COST.get(p.name, 5.0)
            latency = max(self._latencies.get(p.name, 500.0), 1.0)
            quality = self.DEFAULT_QUALITY.get(p.name, 0.5)

            score = (
                self._weights["cost"] * (1.0 / cost)
                + self._weights["latency"] * (1000.0 / latency)
                + self._weights["quality"] * quality
            )
            scores.append(score)

        # 加权随机
        total = sum(scores)
        if total <= 0:
            return random.choice(providers)

        r = random.uniform(0, total)
        cumulative = 0.0
        for p, s in zip(providers, scores):
            cumulative += s
            if r <= cumulative:
                return p

        return providers[-1]
