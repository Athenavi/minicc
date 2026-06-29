"""查询引擎 QueryEngine — MiniCC 的灵魂组件。

对应 Claude Code 的 QueryEngine.ts 设计哲学：
"一个会话一个 QueryEngine，管理的是会话，不是一次请求。"

核心职责：
- 接收用户输入
- 调用 ContextBuilder 装配上下文
- 驱动 LLM 调用（流式）
- 拦截并路由工具调用
- 将工具结果回流到消息历史
- 管理会话级别的状态缓存（跨轮次）

跨轮次状态（参考 Claude Code 源码）：
- mutableMessages: 完整的消息历史
- permissionDenials: 结构化的权限拒绝记录
- totalUsage: 累计 token 用量和成本
- readFileState: 文件读取缓存（避免重复读取）
- discoveredSkillNames: 已发现的技能名称
- loadedNestedMemoryPaths: 已加载的记忆路径
"""

from __future__ import annotations

import asyncio
import logging
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, AsyncGenerator, Callable, Optional

from pydantic import BaseModel

from app.core.context_builder import ContextBuilder
from app.core.permission import PermissionHandler, PermissionLevel, PermissionResult
from app.engine.llm_provider import LLMProvider, StreamEvent, create_provider
from app.models.chat import ContentBlock, Message, Role
from app.models.tool import ToolCall, ToolResult
from app.tools.base import ToolRegistry

logger = logging.getLogger("minicc.engine")

MAX_FILE_CACHE_ENTRIES = 50
MAX_MEMORY_PATHS = 20


# ── 数据模型 ─────────────────────────────────────────────


class UsageStats(BaseModel):
    """累计 token 用量和成本。"""
    input_tokens: int = 0
    output_tokens: int = 0
    total_cost_usd: float = 0.0
    turn_count: int = 0


class PermissionDenial(BaseModel):
    """结构化的权限拒绝记录。"""
    tool_name: str
    reason: str
    input_preview: str = ""
    turn: int
    timestamp: str = ""


class FileCacheEntry(BaseModel):
    """文件读取缓存条目。"""
    content: str
    total_lines: int
    hash: str  # 内容哈希，用于检测文件变更


@dataclass
class QueryEngineConfig:
    """QueryEngine 的运行时配置。

    对应 Claude Code 的 QueryEngineConfig 类型：
    把主循环真正依赖的外部资源都显式列出来。
    """
    session_id: str
    provider: LLMProvider
    tool_registry: ToolRegistry
    context_builder: ContextBuilder
    permission_handler: PermissionHandler = field(default_factory=lambda: PermissionHandler())
    mcp_refresh_callback: Callable | None = None
    custom_system_prompt: str | None = None
    append_system_prompt: str | None = None
    max_tool_rounds: int = 25
    max_tokens: int = 8192
    max_budget_usd: float = 0.0  # 0 = 不限制
    persist_session: bool = True


@dataclass
class SnipReplayState:
    """中断/恢复回放状态。

    对应 Claude Code 的 snipReplay：
    当任务被中断时记录断点，重连后可回放。
    """
    interrupted: bool = False
    last_complete_turn: int = 0
    pending_messages: list[Message] = field(default_factory=list)


# ── QueryEngine ──────────────────────────────────────────


class QueryEngine:
    """会话级主循环编排器。一个会话一个实例。"""

    def __init__(self, config: QueryEngineConfig) -> None:
        self.config = config
        self.session_id = config.session_id

        self._provider = config.provider
        self._tool_registry = config.tool_registry
        self._context_builder = config.context_builder
        self._permission_handler = config.permission_handler
        self._mcp_refresh = config.mcp_refresh_callback

        # ── 跨轮次状态（对应 Claude Code QueryEngine 成员变量） ──

        # 消息历史
        self.mutable_messages: list[Message] = []

        # 中断控制
        self.abort_event = asyncio.Event()
        self.snip_replay = SnipReplayState()

        # 权限拒绝记忆（结构化列表，含原因和上下文）
        self.permission_denials: list[PermissionDenial] = []

        # Token 用量（跨所有轮次累计）
        self.total_usage = UsageStats()

        # 文件读取缓存（避免重复读同一文件）
        self._file_cache: dict[str, FileCacheEntry] = {}

        # 已发现的 skill 名称（避免重复注入）
        self.discovered_skill_names: set[str] = set()

        # 已加载的记忆路径（避免重复读取）
        self.loaded_memory_paths: set[str] = set()

    # ── 主入口 ──

    async def submit_message(
        self,
        content: str,
        is_meta: bool = False,
    ) -> AsyncGenerator[dict[str, Any], None]:
        """一次完整的任务提交。

        对应 Claude Code 的 submitMessage()：
        - 读取配置和状态
        - 准备系统提示词与上下文
        - 调用模型交互
        - 处理工具调用和消息追加
        - 统计 usage、成本、边界状态

        Yields:
            事件字典，供 WebSocket 转发
        """
        # 1. 重置轮次相关状态
        self.abort_event.clear()
        self.snip_replay = SnipReplayState()

        # 2. 追加用户消息到会话历史
        user_msg = Message(
            role=Role.user,
            content=content,
            created_at=datetime.now(timezone.utc),
        )
        self.mutable_messages.append(user_msg)
        yield {"type": "user_message", "payload": {"content": content, "is_meta": is_meta}}

        # 3. 装配上下文
        system_context = await self._context_builder.build_context()
        system_prompt = system_context.build_system_prompt()

        # 追加自定义提示词（如有）
        if self.config.custom_system_prompt:
            system_prompt = f"{system_prompt}\n\n{self.config.custom_system_prompt}"
        if self.config.append_system_prompt:
            system_prompt = f"{system_prompt}\n\n{self.config.append_system_prompt}"

        # 4. 获取工具定义
        tools = self._tool_registry.to_anthropic_tools()

        # 5. 主循环
        for turn in range(self.config.max_tool_rounds):
            if self.abort_event.is_set():
                self.snip_replay.interrupted = True
                yield {"type": "message_complete", "payload": {"interrupted": True, "turn": turn}}
                return

            # 5a. 转换消息为 LLM 格式
            llm_messages = self._messages_to_llm_format()

            # 5b. 调用 LLM
            text_buffer = ""
            tool_calls: list[ToolCall] = []

            async for event in self._provider.send_message(
                system_prompt=system_prompt,
                messages=llm_messages,
                tools=tools if tools else None,
                max_tokens=self.config.max_tokens,
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
                        "payload": {"call_id": tc.id, "name": tc.name, "input": tc.input},
                    }

                elif event.type == "error":
                    logger.error("LLM error: %s", event.data.get("message"))
                    yield {"type": "error", "payload": {"message": event.data.get("message", "LLM call failed")}}
                    return

                elif event.type == "end":
                    # 捕获 usage 信息（若有）
                    usage = event.data.get("usage", {})
                    if usage:
                        inp = usage.get("input_tokens", 0) or usage.get("prompt_tokens", 0)
                        out = usage.get("output_tokens", 0) or usage.get("completion_tokens", 0)
                        self.total_usage.input_tokens += inp
                        self.total_usage.output_tokens += out

                        # Budget 检查
                        if self.config.max_budget_usd > 0:
                            cost = (inp * 3e-6) + (out * 15e-6)  # Claude Sonnet 近似费率
                            self.total_usage.total_cost_usd += cost
                            if self.total_usage.total_cost_usd >= self.config.max_budget_usd:
                                yield {
                                    "type": "message_complete",
                                    "payload": {"budget_exceeded": True, "usage": self.total_usage.model_dump()},
                                }
                                return
                    break

            if self.abort_event.is_set():
                self.snip_replay.interrupted = True
                yield {"type": "message_complete", "payload": {"interrupted": True, "turn": turn}}
                return

            self.total_usage.turn_count += 1

            # 5c. 将 LLM 回复追加到消息历史
            if text_buffer:
                self.mutable_messages.append(
                    Message(
                        role=Role.assistant,
                        content=[ContentBlock(type="text", text=text_buffer)],
                        created_at=datetime.now(timezone.utc),
                    )
                )

            # 5d. 如果没有工具调用 → 完成
            if not tool_calls:
                yield {
                    "type": "message_complete",
                    "payload": {"usage": self.total_usage.model_dump()},
                }
                return

            # 5e. 处理批量工具调用（参考 Claude Code：一轮可发起多个工具调用）
            for tc in tool_calls:
                if self.abort_event.is_set():
                    break

                tool = self._tool_registry.get(tc.name)

                # 未知工具
                if tool is None:
                    result = ToolResult(
                        tool_call_id=tc.id,
                        output=f"Unknown tool: {tc.name}",
                        is_error=True,
                    )
                    self._append_tool_result(tc, result)
                    yield {"type": "tool_call_result", "payload": {"call_id": tc.id, "output": result.output, "is_error": True}}
                    continue

                # Diff 预览
                diff = ""
                if tool.name in ("write_to_file", "str_replace_editor"):
                    from app.tools.file_system import DiffGenerator
                    diff = DiffGenerator.generate_diff_for_tool(tc.input.get("path", ""), tool.name, tc.input) or ""

                # 权限检查（含拒绝记忆增强）
                perm_result = await self._permission_handler.request_permission(
                    tool_call=tc,
                    level=tool.permission_level,
                    reason=f"Tool: {tc.name}",
                    diff_preview=diff,
                )

                if perm_result in (PermissionResult.REJECTED, PermissionResult.TIMEOUT):
                    denial = PermissionDenial(
                        tool_name=tc.name,
                        reason="rejected" if perm_result == PermissionResult.REJECTED else "timeout",
                        input_preview=str(tc.input)[:200],
                        turn=turn,
                        timestamp=datetime.now(timezone.utc).isoformat(),
                    )
                    self.permission_denials.append(denial)
                    result = ToolResult(
                        tool_call_id=tc.id,
                        output=self._permission_handler.format_rejection_feedback(tc.name),
                        is_error=True,
                    )
                else:
                    # 执行工具
                    try:
                        validated = tool.input_schema.model_validate(tc.input)
                        result = await tool.execute(validated)

                        # 更新文件缓存（如果工具是文件读取类操作）
                        if tool.name == "read_file" and not result.is_error:
                            path = str(tc.input.get("path", ""))
                            if path and result.metadata.get("total_lines"):
                                import hashlib
                                self._file_cache[path] = FileCacheEntry(
                                    content=result.output,
                                    total_lines=result.metadata["total_lines"],
                                    hash=hashlib.md5(result.output.encode()).hexdigest(),
                                )
                                # 限制缓存大小
                                if len(self._file_cache) > MAX_FILE_CACHE_ENTRIES:
                                    oldest = next(iter(self._file_cache))
                                    del self._file_cache[oldest]

                    except Exception as exc:
                        result = ToolResult(
                            tool_call_id=tc.id,
                            output=f"Tool execution error: {exc}",
                            is_error=True,
                        )

                self._append_tool_result(tc, result)
                yield {"type": "tool_call_result", "payload": {"call_id": tc.id, "output": result.output, "is_error": result.is_error}}

            # 5f. MCP 刷新（对应 Claude Code 每轮之间的 refreshTools()）
            if self._mcp_refresh:
                try:
                    await self._mcp_refresh()
                except Exception:
                    pass

            # 5g. 更新 snip replay（记录断点以便恢复）
            self.snip_replay.last_complete_turn = turn

        # 到达最大轮次
        yield {
            "type": "message_complete",
            "payload": {"max_turns_reached": True, "turn": self.config.max_tool_rounds, "usage": self.total_usage.model_dump()},
        }

    # ── 状态访问器 ──

    def get_usage_report(self) -> dict:
        """获取用量报告。"""
        return self.total_usage.model_dump()

    def has_recent_denial(self, tool_name: str, within_turns: int = 3) -> bool:
        """检查某工具最近是否被拒绝过。"""
        current_turn = self.total_usage.turn_count
        for d in self.permission_denials:
            if d.tool_name == tool_name and (current_turn - d.turn) <= within_turns:
                return True
        return False

    def get_file_cache_info(self) -> list[dict]:
        """获取文件缓存状态（用于调试/审计）。"""
        return [
            {"path": k, "total_lines": v.total_lines, "hash": v.hash}
            for k, v in self._file_cache.items()
        ]

    def discover_skill(self, name: str) -> bool:
        """记录发现的 skill，返回是否首次发现。"""
        if name in self.discovered_skill_names:
            return False
        self.discovered_skill_names.add(name)
        return True

    def mark_memory_loaded(self, path: str) -> bool:
        """记录加载的记忆路径，返回是否首次加载。"""
        if path in self.loaded_memory_paths:
            return False
        self.loaded_memory_paths.add(path)
        return True

    # ── 内部方法 ──

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
        """中断当前执行。对应 Claude Code 的 abortController。"""
        self.abort_event.set()
        self._permission_handler.cancel_all_pending()
        self.snip_replay.interrupted = True
        logger.info("QueryEngine cancelled: session=%s (turn=%d)", self.session_id, self.total_usage.turn_count)
