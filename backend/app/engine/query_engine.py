"""查询引擎 QueryEngine — MiniCC 的灵魂组件。

对应 Claude Code 的 QueryEngine.ts 设计哲学：
"一个会话一个 QueryEngine，管理的是会话，不是一次请求。"

核心职责：
- 接收用户输入
- 调用 ContextBuilder 装配上下文
- 驱动 LLM 调用（流式）
- 拦截并路由工具调用
- 将工具结果回流到消息历史
- 管理会话级别的状态缓存
"""

from __future__ import annotations

import asyncio
import logging
import uuid
from datetime import datetime, timezone
from typing import Any, AsyncGenerator, Optional

from app.core.context_builder import ContextBuilder
from app.engine.llm_provider import LLMProvider, StreamEvent
from app.models.chat import ContentBlock, Message, Role
from app.models.tool import ToolCall, ToolResult
from app.tools.base import ToolRegistry

logger = logging.getLogger("minicc.engine")


class QueryEngine:
    """会话级主循环编排器。一个会话一个实例。"""

    def __init__(
        self,
        session_id: str,
        provider: LLMProvider,
        tool_registry: ToolRegistry,
        context_builder: ContextBuilder,
        permission_callback: Optional[callable] = None,
        max_tool_rounds: int = 25,
        max_tokens: int = 8192,
    ) -> None:
        self.session_id = session_id
        self._provider = provider
        self._tool_registry = tool_registry
        self._context_builder = context_builder
        self._permission_callback = permission_callback
        self._max_tool_rounds = max_tool_rounds
        self._max_tokens = max_tokens

        # 跨轮次状态（对应 Claude Code QueryEngine 的成员变量）
        self.mutable_messages: list[Message] = []
        self.abort_event = asyncio.Event()
        self.permission_denials: dict[str, bool] = {}
        self.total_usage: dict[str, int] = {}

    async def submit_message(
        self,
        content: str,
    ) -> AsyncGenerator[dict[str, Any], None]:
        """一次完整的任务提交。

        Yields:
            事件字典，包含 type 和 payload，供 WebSocket 转发：
            - {"type": "message_chunk", "payload": {"index": N, "text": "..."}}
            - {"type": "tool_call_start", ...}
            - {"type": "message_complete", ...}
        """
        # 1. 追加用户消息
        user_msg = Message(
            role=Role.user,
            content=content,
            created_at=datetime.now(timezone.utc),
        )
        self.mutable_messages.append(user_msg)
        yield {"type": "user_message", "payload": {"content": content}}

        # 2. 装配上下文
        system_context = await self._context_builder.build_context()

        # 3. 获取工具定义
        tools = self._tool_registry.to_anthropic_tools()
        system_prompt = system_context.build_system_prompt()

        # 4. 主循环
        for turn in range(self._max_tool_rounds):
            if self.abort_event.is_set():
                yield {"type": "message_complete", "payload": {"interrupted": True}}
                return

            # 4a. 转换消息为 LLM 格式
            llm_messages = self._messages_to_llm_format()

            # 4b. 调用 LLM
            text_buffer = ""
            tool_calls: list[ToolCall] = []

            async for event in self._provider.send_message(
                system_prompt=system_prompt,
                messages=llm_messages,
                tools=tools if tools else None,
                max_tokens=self._max_tokens,
            ):
                if self.abort_event.is_set():
                    break

                if event.type == "text":
                    text_buffer += event.data.get("text", "")
                    yield {
                        "type": "text_chunk",
                        "payload": {"text": event.data.get("text", "")},
                    }

                elif event.type == "tool_use":
                    tc = ToolCall(
                        id=event.data.get("id", f"call_{uuid.uuid4().hex[:8]}"),
                        name=event.data.get("name", "unknown"),
                        type="function",
                        input=event.data.get("input", {}),
                    )
                    tool_calls.append(tc)
                    yield {
                        "type": "tool_call_start",
                        "payload": {
                            "call_id": tc.id,
                            "name": tc.name,
                            "input": tc.input,
                        },
                    }

                elif event.type == "error":
                    logger.error("LLM error: %s", event.data.get("message"))
                    yield {
                        "type": "error",
                        "payload": {"message": event.data.get("message", "LLM call failed")},
                    }
                    return

                elif event.type == "end":
                    break

            if self.abort_event.is_set():
                yield {"type": "message_complete", "payload": {"interrupted": True}}
                return

            # 4c. 将 LLM 回复追加到消息历史
            if text_buffer:
                self.mutable_messages.append(
                    Message(
                        role=Role.assistant,
                        content=[ContentBlock(type="text", text=text_buffer)],
                        created_at=datetime.now(timezone.utc),
                    )
                )

            # 4d. 如果没有工具调用 → 完成
            if not tool_calls:
                yield {"type": "message_complete", "payload": {}}
                return

            # 4e. 处理工具调用（Phase 2 将插入权限检查）
            for tc in tool_calls:
                # 权限检查 (Phase 2 实现)
                if self._permission_callback:
                    approved = await self._permission_callback(tc)
                    if not approved:
                        self.permission_denials[tc.name] = True
                        result = ToolResult(
                            tool_call_id=tc.id,
                            output="Operation was rejected by user.",
                            is_error=True,
                        )
                        self._append_tool_result(tc, result)
                        yield {
                            "type": "tool_call_result",
                            "payload": {
                                "call_id": tc.id,
                                "output": result.output,
                                "is_error": True,
                            },
                        }
                        continue

                # 执行工具
                tool = self._tool_registry.get(tc.name)
                if tool is None:
                    result = ToolResult(
                        tool_call_id=tc.id,
                        output=f"Unknown tool: {tc.name}",
                        is_error=True,
                    )
                else:
                    try:
                        validated = tool.input_schema.model_validate(tc.input)
                        result = await tool.execute(validated)
                    except Exception as exc:
                        result = ToolResult(
                            tool_call_id=tc.id,
                            output=f"Tool execution error: {exc}",
                            is_error=True,
                        )

                # 回流结果到消息历史
                self._append_tool_result(tc, result)
                yield {
                    "type": "tool_call_result",
                    "payload": {
                        "call_id": tc.id,
                        "output": result.output,
                        "is_error": result.is_error,
                    },
                }

            # 4f. 继续下一轮（LLM 看到 tool_result 后继续推理）
            continue

        # 到达最大轮次
        yield {"type": "message_complete", "payload": {"max_turns_reached": True}}

    def _messages_to_llm_format(self) -> list[dict]:
        """将内部 Message 列表转为 LLM API 格式。"""
        result: list[dict] = []
        for msg in self.mutable_messages:
            entry: dict[str, Any] = {"role": msg.role.value}

            if isinstance(msg.content, str):
                entry["content"] = msg.content
            else:
                blocks = []
                for block in msg.content:
                    b: dict[str, Any] = {"type": block.type}
                    if block.text is not None:
                        b["text"] = block.text
                    if block.id is not None:
                        b["id"] = block.id
                    if block.name is not None:
                        b["name"] = block.name
                    if block.input is not None:
                        b["input"] = block.input
                    if block.tool_use_id is not None:
                        b["tool_use_id"] = block.tool_use_id
                    if block.content is not None:
                        b["content"] = [c.model_dump(exclude_none=True) for c in block.content]
                    blocks.append(b)
                entry["content"] = blocks

            result.append(entry)
        return result

    def _append_tool_result(self, tc: ToolCall, result: ToolResult) -> None:
        """将工具执行结果追加为 tool role 消息。"""
        self.mutable_messages.append(
            Message(
                role=Role.tool,
                content=[ContentBlock(
                    type="tool_result",
                    tool_use_id=tc.id,
                    content=[ContentBlock(type="text", text=result.output)],
                )],
                created_at=datetime.now(timezone.utc),
            )
        )

    def cancel(self) -> None:
        """中断当前执行。"""
        self.abort_event.set()
        logger.info("QueryEngine cancelled: session=%s", self.session_id)
