"""上下文压缩管理 — 多层分级压缩管线。

对应 Claude Code 的 query.ts 压缩链路设计：
1. Tool Result Budget — 裁剪过大的工具结果
2. Snip — 局部瘦身
3. Microcompact — tool_use 细粒度压缩
4. Context Collapse — 折叠视图（不删历史，重投影）
5. AutoCompact — 自动摘要压缩
6. Reactive Compact — API 报错后兜底

设计原则：能局部处理就不做全局摘要，能折叠视图就不立即合并，
尽量延后信息损失，尽量保留结构，最后才牺牲细节。
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import Any, Optional

from app.models.chat import ContentBlock, Message, Role

logger = logging.getLogger("minicc.compact")

# 默认上下文预算（token 数）
DEFAULT_CONTEXT_BUDGET = 100_000
TOOL_RESULT_BUDGET = 40_000  # 工具结果最大占用
SNIP_THRESHOLD = 80_000  # 触发 snip 的阈值 (80%)
COLLAPSE_THRESHOLD = 90_000  # 触发 collapse 的阈值 (90%)


@dataclass
class CompactBoundary:
    """压缩边界标记 — 对应 Claude Code 的 compact_boundary。

    标记一次压缩在会话链条中的边界，
    告诉 transcript 哪一段历史已经被总结。
    """
    tail_uuid: str
    summary: str = ""
    preserved_count: int = 0


@dataclass
class CompactResult:
    """压缩结果。"""
    messages: list[Message]
    tokens_freed: int = 0
    boundary: Optional[CompactBoundary] = None


# ── 第 1 层：工具结果预算裁剪 ──


class BudgetManager:
    """工具结果预算管理。

    对应 Claude Code 的 applyToolResultBudget()。
    在进入真正压缩前，先把明显过大的工具结果做替换或裁剪。
    """

    def __init__(self, max_tool_result_chars: int = TOOL_RESULT_BUDGET) -> None:
        self._max_chars = max_tool_result_chars

    def apply(self, messages: list[Message]) -> CompactResult:
        """裁剪超过预算的工具结果。"""
        freed = 0
        result: list[Message] = []

        for msg in messages:
            if msg.role != Role.tool:
                result.append(msg)
                continue

            content = msg.content
            if isinstance(content, str):
                if len(content) > self._max_chars:
                    truncated = content[: self._max_chars] + "\n...[tool result truncated]"
                    freed += len(content) - len(truncated)
                    result.append(Message(role=Role.tool, content=truncated, created_at=msg.created_at))
                else:
                    result.append(msg)
            elif isinstance(content, list):
                trimmed_blocks: list[ContentBlock] = []
                for block in content:
                    if block.type == "tool_result" and block.content:
                        text_blocks: list[ContentBlock] = []
                        for sub in block.content:
                            sub_text = sub.text
                            if sub.type == "text" and sub_text and len(sub_text) > self._max_chars:
                                sub_text = sub_text[: self._max_chars] + "\n...[truncated]"
                                freed += len(sub.text) - len(sub_text)
                            text_blocks.append(ContentBlock(type=sub.type, text=sub_text) if sub.type == "text" else sub)
                        trimmed_blocks.append(ContentBlock(
                            type="tool_result",
                            tool_use_id=block.tool_use_id,
                            content=text_blocks,
                        ))
                    else:
                        trimmed_blocks.append(block)
                result.append(Message(role=Role.tool, content=trimmed_blocks, created_at=msg.created_at))

        return CompactResult(messages=result, tokens_freed=freed // 4)

    def estimate_tokens(self, text: str) -> int:
        """粗略估计 token 数（4 字符 ≈ 1 token）。"""
        return len(text) // 4


# ── 第 2 层：Snip 局部裁剪 ──


class SnipCompactor:
    """Snip 局部裁剪。

    对应 Claude Code 的 snipCompactIfNeeded()。
    在不破坏主要会话结构的前提下，先对低价值部分做局部裁剪。
    """

    def __init__(self, threshold: int = SNIP_THRESHOLD) -> None:
        self._threshold = threshold

    def compact_if_needed(self, messages: list[Message], total_tokens: int) -> CompactResult:
        """如果接近阈值，执行 snip 裁剪。"""
        if total_tokens < self._threshold:
            return CompactResult(messages=messages)

        budget = BudgetManager()
        estimated = total_tokens if total_tokens > 0 else sum(budget.estimate_tokens(str(m.content)) for m in messages)

        if estimated < self._threshold:
            return CompactResult(messages=messages)

        # Snip 策略：压缩早期工具结果中的输出文本
        modified = list(messages)
        freed = 0
        snipped = 0

        for i, msg in enumerate(modified):
            if msg.role != Role.tool or snipped > 3:
                continue
            content = msg.content
            # Handle both string and list (ContentBlock) content
            text_content = ""
            if isinstance(content, str):
                text_content = content
            elif isinstance(content, list):
                for block in content:
                    if block.type == "tool_result" and block.content:
                        for sub in block.content:
                            if sub.type == "text" and sub.text:
                                text_content += sub.text

            if len(text_content) > 2000:
                original_len = len(text_content)
                truncated = text_content[:1000] + "\n...[snip: trimmed]...\n" + text_content[-500:]
                # Replace with simple string message
                modified[i] = Message(
                    role=Role.tool,
                    content=truncated,
                    created_at=msg.created_at,
                )
                freed += original_len - len(truncated)
                snipped += 1

        tokens_freed = freed // 4
        boundary = CompactBoundary(tail_uuid=f"snip_{id(messages)}", summary=f"Snip compressed {snipped} tool results", preserved_count=snipped)

        return CompactResult(messages=modified, tokens_freed=tokens_freed, boundary=boundary)


# ── 第 4 层：Context Collapse 折叠视图 ──


@dataclass
class CollapseEntry:
    """一次 collapse 操作的记录。"""
    start_idx: int
    end_idx: int
    summary: str = ""
    message_count: int = 0


class ContextCollapser:
    """上下文折叠视图。

    对应 Claude Code 的 context collapse：
    不是删除历史，而是"重新投影视图"。
    底层日志未必被彻底抹掉，但当前喂给模型的视图被折叠了。
    """

    def __init__(self, threshold: int = COLLAPSE_THRESHOLD, collapse_window: int = 10) -> None:
        self._threshold = threshold
        self._collapse_window = collapse_window
        self.collapse_history: list[CollapseEntry] = []

    def apply_if_needed(self, messages: list[Message], total_tokens: int) -> CompactResult:
        """如果接近阈值，折叠早期的工具调用回合。"""
        if total_tokens < self._threshold or len(messages) < self._collapse_window:
            return CompactResult(messages=messages)

        # 折叠策略：找到最早的一批 tool_use + tool_result 回合，折叠成摘要
        modified = list(messages)
        collapse_indices = []
        current_tool_call: Optional[int] = None
        collapsed_count = 0

        for i, msg in enumerate(modified):
            if collapsed_count >= 3:  # 最多折叠 3 组
                break
            if msg.role == Role.assistant:
                if isinstance(msg.content, list):
                    for block in msg.content:
                        if block.type == "tool_use":
                            current_tool_call = i
            elif msg.role == Role.tool and current_tool_call is not None:
                collapse_indices.append((current_tool_call, i))
                current_tool_call = None
                collapsed_count += 1

        if not collapse_indices:
            return CompactResult(messages=messages)

        # 从后往前删除（保持索引正确）
        collapse_indices.sort(key=lambda x: -x[1])
        for start, end in collapse_indices:
            n_deleted = end - start + 1
            summary = f"[{n_deleted} messages collapsed: tool call + result]"
            # 在删除位置插入摘要标记
            modified = modified[:start] + modified[end + 1:]
            entry = CollapseEntry(start_idx=start, end_idx=end, summary=summary, message_count=n_deleted)
            self.collapse_history.append(entry)

        budget = BudgetManager()
        tokens_freed = sum(budget.estimate_tokens(str(m.content)) for m in messages) - sum(
            budget.estimate_tokens(str(m.content)) for m in modified
        )
        boundary = CompactBoundary(tail_uuid=f"collapse_{id(messages)}", summary=f"Collapsed {len(collapse_indices)} tool call rounds", preserved_count=len(collapse_indices))

        return CompactResult(messages=modified, tokens_freed=tokens_freed, boundary=boundary)


# ── 第 5 层：AutoCompact 自动摘要 ──


class AutoCompactor:
    """自动摘要压缩。

    对应 Claude Code 的 autocompact()。
    当前面几层都不足以把上下文降回阈值以下时，
    由 LLM 生成摘要替换最早的消息。
    """

    def __init__(self, target_ratio: float = 0.6, min_keep_messages: int = 6) -> None:
        self._target_ratio = target_ratio
        self._min_keep = min_keep_messages
        self._consecutive_failures = 0

    async def compact(
        self,
        messages: list[Message],
        total_tokens: int,
        threshold: int,
    ) -> CompactResult:
        """执行自动摘要压缩。

        保留最近的 N 条消息完整，对早期的消息生成摘要。
        """
        if len(messages) <= self._min_keep:
            return CompactResult(messages=messages)

        budget = BudgetManager()
        # 保留最近的消息
        keep = messages[-self._min_keep:]
        compress = messages[:-self._min_keep]

        if not compress:
            return CompactResult(messages=messages)

        # 生成摘要（实际项目中应调用 LLM，这里使用简单策略）
        summary_parts = []
        for msg in compress:
            content_preview = ""
            if isinstance(msg.content, str):
                content_preview = msg.content[:100]
            elif isinstance(msg.content, list):
                for block in msg.content:
                    if block.type == "text" and block.text:
                        content_preview = block.text[:100]
                        break
            if content_preview:
                role_label = msg.role.value
                summary_parts.append(f"[{role_label}] {content_preview}")

        summary = "\n".join(summary_parts[:30])
        summary_msg = Message(
            role=Role.system,
            content=f"[Previous conversation summary]\n{summary}\n[End of summary]",
        )

        # 计算释放的 token
        original_tokens = sum(budget.estimate_tokens(str(m.content)) for m in compress)
        summary_tokens = budget.estimate_tokens(summary)
        tokens_freed = original_tokens - summary_tokens

        result_messages = [summary_msg] + keep
        boundary = CompactBoundary(
            tail_uuid=f"auto_{id(messages)}",
            summary=f"Auto-compressed {len(compress)} messages into summary",
            preserved_count=len(keep),
        )

        return CompactResult(messages=result_messages, tokens_freed=tokens_freed, boundary=boundary)


# ── 压缩管线 ──


class CompactPipeline:
    """完整压缩管线 — 按顺序执行所有层级。

    对应 Claude Code 的 query.ts 压缩链路：
    budget → snip → microcompact → collapse → autocompact
    """

    def __init__(self) -> None:
        self.budget = BudgetManager()
        self.snip = SnipCompactor()
        self.collapser = ContextCollapser()
        self.autocompactor = AutoCompactor()
        self.boundaries: list[CompactBoundary] = []

    async def apply_all(self, messages: list[Message], total_tokens: int) -> CompactResult:
        """按顺序执行所有压缩层级。

        层层升级：能局部处理就不做全局摘要。
        """
        current = messages
        total_freed = 0

        # 第 1 层：工具结果预算裁剪
        r1 = self.budget.apply(current)
        current = r1.messages
        total_freed += r1.tokens_freed
        if r1.boundary:
            self.boundaries.append(r1.boundary)

        # 估计当前 token 数
        budget = BudgetManager()
        estimated = total_tokens - total_freed

        # 第 2 层：Snip 局部裁剪
        r2 = self.snip.compact_if_needed(current, estimated)
        if r2.tokens_freed > 0:
            current = r2.messages
            total_freed += r2.tokens_freed
            estimated -= r2.tokens_freed
            if r2.boundary:
                self.boundaries.append(r2.boundary)

        # 第 4 层：Context Collapse 折叠
        if estimated >= SNIP_THRESHOLD:
            r4 = self.collapser.apply_if_needed(current, estimated)
            if r4.tokens_freed > 0:
                current = r4.messages
                total_freed += r4.tokens_freed
                estimated -= r4.tokens_freed
                if r4.boundary:
                    self.boundaries.append(r4.boundary)

        # 第 5 层：AutoCompact 自动摘要
        if estimated >= COLLAPSE_THRESHOLD and len(current) > 6:
            r5 = await self.autocompactor.compact(current, estimated, COLLAPSE_THRESHOLD)
            if r5.tokens_freed > 0:
                current = r5.messages
                total_freed += r5.tokens_freed
                if r5.boundary:
                    self.boundaries.append(r5.boundary)

        if total_freed > 0:
            logger.info("Compression freed %d tokens (%d boundaries)", total_freed, len(self.boundaries))

        return CompactResult(messages=current, tokens_freed=total_freed, boundary=self.boundaries[-1] if self.boundaries else None)
