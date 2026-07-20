# Gateway Provider 接口定义
from __future__ import annotations

import time
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import AsyncIterator, Optional


@dataclass
class ChatMessage:
    role: str  # "system" | "user" | "assistant" | "tool"
    content: str = ""
    tool_call_id: str = ""
    tool_calls: list[ToolCall] | None = None

    def to_dict(self) -> dict:
        d: dict = {"role": self.role}
        # tool_calls 存在时 content 应为 None（OpenAI API 规范），但非空文本应保留
        if self.tool_calls:
            d["content"] = self.content or None
        else:
            d["content"] = self.content
        if self.tool_call_id:
            d["tool_call_id"] = self.tool_call_id
        if self.tool_calls:
            d["tool_calls"] = [tc.to_dict() for tc in self.tool_calls]
        return d


@dataclass
class ToolCall:
    id: str
    name: str
    arguments: str  # JSON string

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "type": "function",
            "function": {"name": self.name, "arguments": self.arguments},
        }


@dataclass
class ChatResponse:
    content: str = ""
    reasoning_content: str = ""  # DeepSeek thinking mode 思考过程
    tool_calls: list[ToolCall] = field(default_factory=list)
    finish_reason: str = ""  # "stop" | "tool_calls" | "length"
    input_tokens: int = 0
    output_tokens: int = 0
    provider: str = ""
    model: str = ""
    latency_ms: float = 0.0


@dataclass
class EmbeddingResponse:
    embedding: list[float] = field(default_factory=list)
    tokens: int = 0


class LLMProvider(ABC):
    """LLM Provider 基类 — 每个 Provider 必须实现"""

    name: str = ""

    @abstractmethod
    async def chat_stream(
        self,
        messages: list[ChatMessage],
        model: str,
        *,
        max_tokens: int = 4096,
        temperature: float = 0.7,
        tools: list[dict] | None = None,
    ) -> AsyncIterator[ChatResponse]:
        """流式推理，逐 chunk yield"""
        ...

    @abstractmethod
    async def chat(
        self,
        messages: list[ChatMessage],
        model: str,
        *,
        max_tokens: int = 4096,
        temperature: float = 0.7,
        tools: list[dict] | None = None,
    ) -> ChatResponse:
        """非流式推理"""
        ...

    @abstractmethod
    async def embed(self, text: str, model: str) -> EmbeddingResponse:
        """文本嵌入"""
        ...

    @abstractmethod
    async def close(self) -> None:
        """释放资源"""
        ...
