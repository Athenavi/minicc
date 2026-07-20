# OpenAI Provider — 使用 openai SDK
from __future__ import annotations

import json
import logging
from typing import AsyncIterator

from openai import AsyncOpenAI

from app.gateway.provider import (
    ChatMessage,
    ChatResponse,
    EmbeddingResponse,
    LLMProvider,
    ToolCall,
)

logger = logging.getLogger(__name__)


class OpenAIProvider(LLMProvider):
    name = "openai"

    def __init__(self, api_key: str, base_url: str = ""):
        kwargs: dict = {"api_key": api_key}
        if base_url:
            kwargs["base_url"] = base_url
        self._client = AsyncOpenAI(**kwargs)

    async def chat_stream(
        self,
        messages: list[ChatMessage],
        model: str,
        *,
        max_tokens: int = 4096,
        temperature: float = 0.7,
        tools: list[dict] | None = None,
    ) -> AsyncIterator[ChatResponse]:
        kwargs = self._build_kwargs(messages, model, max_tokens, temperature, tools)
        kwargs["stream"] = True

        response = await self._client.chat.completions.create(**kwargs)

        tool_calls: list[dict] = []
        input_tokens = 0
        output_tokens = 0
        reasoning_content = ""

        async for chunk in response:
            if not chunk.choices:
                # 最后一个 chunk 可能只有 usage
                if chunk.usage:
                    input_tokens = chunk.usage.prompt_tokens
                    output_tokens = chunk.usage.completion_tokens
                continue

            delta = chunk.choices[0].delta

            # DeepSeek thinking mode：获取 reasoning_content（API 发送的是增量文本）
            if hasattr(delta, 'reasoning_content') and delta.reasoning_content:
                reasoning_content += delta.reasoning_content  # 累积完整思考文本
                yield ChatResponse(reasoning_content=delta.reasoning_content)  # 传递增量

            if delta.content:
                yield ChatResponse(content=delta.content)

            if delta.tool_calls:
                for tc in delta.tool_calls:
                    while len(tool_calls) <= tc.index:
                        tool_calls.append({"id": "", "name": "", "arguments": ""})
                    if tc.id:
                        tool_calls[tc.index]["id"] = tc.id
                    if tc.function:
                        if tc.function.name:
                            tool_calls[tc.index]["name"] = tc.function.name
                        if tc.function.arguments:
                            tool_calls[tc.index]["arguments"] += tc.function.arguments

            finish = chunk.choices[0].finish_reason
            if finish:
                # 捕获 Token 计数（可能和 finish_reason 在同一 chunk）
                if chunk.usage:
                    input_tokens = chunk.usage.prompt_tokens
                    output_tokens = chunk.usage.completion_tokens
                parsed = [
                    ToolCall(id=tc["id"], name=tc["name"], arguments=tc["arguments"])
                    for tc in tool_calls if tc["id"]
                ]
                yield ChatResponse(
                    tool_calls=parsed,
                    finish_reason="tool_calls" if parsed else (finish or "stop"),
                    input_tokens=input_tokens,
                    output_tokens=output_tokens,
                    reasoning_content=reasoning_content,
                )

        # 发出 token 统计（usage chunk 可能没有 finish_reason）
        if input_tokens > 0 or output_tokens > 0:
            yield ChatResponse(
                input_tokens=input_tokens,
                output_tokens=output_tokens,
                finish_reason="stop",
                reasoning_content=reasoning_content,
            )

    async def chat(
        self,
        messages: list[ChatMessage],
        model: str,
        *,
        max_tokens: int = 4096,
        temperature: float = 0.7,
        tools: list[dict] | None = None,
    ) -> ChatResponse:
        kwargs = self._build_kwargs(messages, model, max_tokens, temperature, tools)
        resp = await self._client.chat.completions.create(**kwargs)

        choice = resp.choices[0]
        content = choice.message.content or ""
        tool_calls = []
        if choice.message.tool_calls:
            for tc in choice.message.tool_calls:
                tool_calls.append(
                    ToolCall(id=tc.id, name=tc.function.name, arguments=tc.function.arguments)
                )

        usage = resp.usage
        return ChatResponse(
            content=content,
            tool_calls=tool_calls,
            finish_reason="tool_calls" if tool_calls else (choice.finish_reason or "stop"),
            input_tokens=usage.prompt_tokens if usage else 0,
            output_tokens=usage.completion_tokens if usage else 0,
        )

    async def embed(self, text: str, model: str) -> EmbeddingResponse:
        resp = await self._client.embeddings.create(model=model, input=text)
        usage = resp.usage
        return EmbeddingResponse(
            embedding=resp.data[0].embedding,
            tokens=usage.total_tokens if usage else 0,
        )

    async def close(self) -> None:
        await self._client.close()

    # ── helpers ──

    @staticmethod
    def _build_kwargs(
        messages: list[ChatMessage],
        model: str,
        max_tokens: int,
        temperature: float,
        tools: list[dict] | None,
    ) -> dict:
        kwargs: dict = {
            "model": model,
            "messages": [m.to_dict() if hasattr(m, 'to_dict') else m for m in messages],
            "max_tokens": max_tokens,
            "temperature": temperature,
        }
        # ── 调试：检查消息序列中 tool 消息的配对 ──
        msgs = kwargs["messages"]
        for i, m in enumerate(msgs):
            if m.get("role") == "tool":
                prev = msgs[i-1] if i > 0 else None
                if prev:
                    has_tc = bool(prev.get("tool_calls"))
                    logger.info("Tool msg #%d: prev role=%s has_tool_calls=%s tool_call_id=%s",
                                i, prev.get("role"), has_tc, m.get("tool_call_id", ""))
        logger.info("LLM call: model=%s msgs=%d roles=%s", model, len(msgs), [m.get("role") for m in msgs])
        if tools:
            # 统一转为 OpenAI function 格式
            converted = []
            for t in tools:
                if "type" in t and t["type"] == "function":
                    converted.append(t)
                else:
                    converted.append({"type": "function", "function": t})
            kwargs["tools"] = converted
            kwargs["tool_choice"] = "auto"
        return kwargs
