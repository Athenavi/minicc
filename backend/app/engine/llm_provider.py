"""LLM Provider 抽象层 — 支持 Anthropic / OpenAI / Ollama。

每个 Provider 将 SDK 特有的流式事件归一化为 StreamEvent，
使 QueryEngine 无需关心底层模型差异。
"""

from __future__ import annotations

import json
from abc import ABC, abstractmethod
from typing import Any, AsyncGenerator, Optional

from pydantic import BaseModel, Field


class StreamEvent(BaseModel):
    """统一的流式事件。QueryEngine 只消费这个类型。"""
    type: str = Field(description="text | tool_use | tool_result | end | error")
    data: dict[str, Any] = Field(default_factory=dict)


class LLMProvider(ABC):
    """LLM Provider 抽象基类。"""

    def __init__(self, api_key: str, model: str) -> None:
        self.api_key = api_key
        self.model = model

    @abstractmethod
    async def send_message(
        self,
        system_prompt: str,
        messages: list[dict],
        tools: Optional[list[dict]] = None,
        max_tokens: int = 8192,
    ) -> AsyncGenerator[StreamEvent, None]:
        """发送消息并流式获取响应。

        Yields:
            StreamEvent: 统一的事件流（text / tool_use / end / error）
        """
        ...


class AnthropicProvider(LLMProvider):
    """Anthropic Messages API Provider。"""

    async def send_message(
        self,
        system_prompt: str,
        messages: list[dict],
        tools: Optional[list[dict]] = None,
        max_tokens: int = 8192,
    ) -> AsyncGenerator[StreamEvent, None]:
        try:
            import anthropic
        except ImportError:
            yield StreamEvent(type="error", data={"message": "anthropic SDK not installed"})
            return

        client = anthropic.AsyncAnthropic(api_key=self.api_key)

        kwargs: dict[str, Any] = {
            "model": self.model,
            "max_tokens": max_tokens,
            "system": system_prompt,
            "messages": messages,
        }
        if tools:
            kwargs["tools"] = tools

        try:
            async with client.messages.stream(**kwargs) as stream:
                async for event in stream:
                    if event.type == "content_block_delta" and event.delta.type == "text_delta":
                        yield StreamEvent(type="text", data={"text": event.delta.text})
                    elif event.type == "content_block_start" and event.content_block.type == "tool_use":
                        yield StreamEvent(
                            type="tool_use",
                            data={
                                "id": event.content_block.id,
                                "name": event.content_block.name,
                                "input": event.content_block.input or {},
                            },
                        )
                    elif event.type == "message_delta" and event.delta.stop_reason == "end_turn":
                        yield StreamEvent(type="end", data={})
        except Exception as exc:
            yield StreamEvent(type="error", data={"message": str(exc)})


class OpenAIProvider(LLMProvider):
    """OpenAI-compatible Provider（也支持 Ollama 本地模型）。"""

    def __init__(self, api_key: str, model: str, base_url: Optional[str] = None) -> None:
        super().__init__(api_key, model)
        self.base_url = base_url

    async def send_message(
        self,
        system_prompt: str,
        messages: list[dict],
        tools: Optional[list[dict]] = None,
        max_tokens: int = 8192,
    ) -> AsyncGenerator[StreamEvent, None]:
        try:
            from openai import AsyncOpenAI
        except ImportError:
            yield StreamEvent(type="error", data={"message": "openai SDK not installed"})
            return

        kwargs: dict[str, Any] = {"api_key": self.api_key}
        if self.base_url:
            kwargs["base_url"] = self.base_url

        client = AsyncOpenAI(**kwargs)

        openai_messages: list[dict] = [{"role": "system", "content": system_prompt}]
        openai_messages.extend(messages)

        request_kwargs: dict[str, Any] = {
            "model": self.model,
            "messages": openai_messages,
            "max_tokens": max_tokens,
            "stream": True,
        }
        if tools:
            request_kwargs["tools"] = tools

        try:
            # Accumulate tool calls across chunks
            tool_call_acc: dict[int, dict] = {}

            stream = await client.chat.completions.create(**request_kwargs)
            async for chunk in stream:
                delta = chunk.choices[0].delta if chunk.choices else None
                if not delta:
                    continue

                if delta.content:
                    yield StreamEvent(type="text", data={"text": delta.content})

                if delta.tool_calls:
                    for tc in delta.tool_calls:
                        idx = tc.index if hasattr(tc, "index") else 0
                        if idx not in tool_call_acc:
                            tool_call_acc[idx] = {
                                "id": tc.id or "",
                                "name": tc.function.name if tc.function else "",
                                "arguments": "",
                            }
                        if tc.function:
                            if tc.function.name:
                                tool_call_acc[idx]["name"] = tc.function.name
                            if tc.id:
                                tool_call_acc[idx]["id"] = tc.id
                            if tc.function.arguments:
                                tool_call_acc[idx]["arguments"] += tc.function.arguments

                # Only yield tool_use when finish_reason is tool_calls
                if chunk.choices[0].finish_reason:
                    finish = chunk.choices[0].finish_reason
                    if finish == "tool_calls":
                        for idx in sorted(tool_call_acc.keys()):
                            tc_data = tool_call_acc[idx]
                            try:
                                args = json.loads(tc_data["arguments"]) if tc_data["arguments"] else {}
                            except json.JSONDecodeError:
                                args = {}
                            yield StreamEvent(
                                type="tool_use",
                                data={
                                    "id": tc_data["id"] or f"call_{idx}",
                                    "name": tc_data["name"],
                                    "input": args,
                                },
                            )
                    tool_call_acc.clear()
                    yield StreamEvent(type="end", data={"reason": finish})

        except Exception as exc:
            yield StreamEvent(type="error", data={"message": str(exc)})


def create_provider(
    provider_type: str,
    api_key: str,
    model: str,
    base_url: Optional[str] = None,
) -> LLMProvider:
    """工厂函数 — 根据类型创建 Provider。"""
    if provider_type == "anthropic":
        return AnthropicProvider(api_key, model)
    elif provider_type == "openai":
        return OpenAIProvider(api_key, model, base_url)
    else:
        raise ValueError(f"Unknown provider: {provider_type}")
