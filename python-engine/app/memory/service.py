"""Memory Service 门面 - 四层记忆架构统一入口。

本模块实现 MemoryService 门面类，作为四层记忆架构的统一入口：
- L1 SessionMetaStore: 会话级簿记
- L2 ProfileCard: 用户档案卡
- L3 SummaryStore: 对话摘要（占位）
- 提供生命周期钩子：on_session_start/on_turn_complete/on_session_end
"""

from __future__ import annotations

import logging
from typing import Any, Optional

from app.memory.layers import (
    ConflictRef,
    MemoryConflict,
    ProfileItem,
    ProfileUpdateResult,
    RecallResult,
    RecalledItem,
    Scope,
    SessionContext,
    SessionMeta,
    SlotType,
    SourceType,
)
from app.memory.profile_card import ProfileCard
from app.memory.session_meta import SessionMetaStore

logger = logging.getLogger(__name__)


class MemoryService:
    """四层记忆架构门面类。

    作为记忆系统的统一入口，负责协调 L1/L2/L3 各层交互。
    """

    def __init__(
        self,
        session_meta_store: SessionMetaStore,
        profile_card: ProfileCard,
        summary_store: Any = None,
        producer: Any = None,
    ) -> None:
        """初始化 MemoryService。

        Args:
            session_meta_store: L1 会话元数据存储。
            profile_card: L2 用户档案卡 Provider。
            summary_store: L3 摘要存储（Task 12 后启用）。
            producer: 队列生产者（Task 14 后启用，用于异步巩固）。
        """
        self._session_meta = session_meta_store
        self._profile_card = profile_card
        self._summary_store = summary_store
        self._producer = producer

    # ── 生命周期钩子 ──────────────────────────────────────────────────

    async def on_session_start(
        self,
        session_id: str,
        tenant_id: str,
        user_id: str,
        entry_channel: str = "web",
        mode: str = "agent",
    ) -> SessionContext:
        """会话开始时调用。

        1. 在 L1 创建 SessionMeta
        2. 从 L2 预取用户档案卡
        3. 从 L3 预取相关摘要（占位）

        Args:
            session_id: 会话 ID。
            tenant_id: 租户 ID。
            user_id: 用户 ID。
            entry_channel: 入口渠道。
            mode: Agent 模式。

        Returns:
            SessionContext 包含初始化状态信息。
        """
        # L1: 创建会话元数据
        meta = self._session_meta.create(
            session_id=session_id,
            tenant_id=tenant_id,
            user_id=user_id,
            entry_channel=entry_channel,
            mode=mode,
        )

        # L2: 预取用户档案卡
        profile_items = await self._profile_card.get_profile(tenant_id, user_id)
        profile_cached = len(profile_items) > 0

        # L3: 预取相关摘要（占位，Task 12 实现后启用）
        summaries_prefetched = 0

        logger.info(
            "Session started: %s (user=%s, tenant=%s, profile_cached=%s)",
            session_id, user_id, tenant_id, profile_cached,
        )

        return SessionContext(
            meta=meta,
            profile_cached=profile_cached,
            summaries_prefetched=summaries_prefetched,
        )

    async def on_turn_complete(
        self,
        session_id: str,
        tokens_in: int = 0,
        tokens_out: int = 0,
    ) -> None:
        """回合完成时调用。

        1. 在 L1 记账（turn_count + token 累计）
        2. 触发 L3 异步巩固（通过队列）

        Args:
            session_id: 会话 ID。
            tokens_in: 输入 token 数。
            tokens_out: 输出 token 数。
        """
        # L1: 记账
        meta = self._session_meta.get(session_id)
        if meta:
            meta.mark_turn_complete(tokens_in, tokens_out)
            self._session_meta.update(session_id)

        # L3: 异步巩固（入队）
        if meta and self._producer:
            await self._enqueue_consolidate(
                tenant_id=meta.tenant_id,
                user_id=meta.user_id,
                session_id=session_id,
                turn_count=meta.turn_count,
            )

        logger.debug(
            "Turn completed: session=%s, tokens_in=%d, tokens_out=%d",
            session_id, tokens_in, tokens_out,
        )

    async def on_session_end(self, session_id: str) -> None:
        """会话结束时调用。

        1. 触发 L3 会话级 rollup（异步入队）
        2. 从 L1 丢弃 SessionMeta

        Args:
            session_id: 会话 ID。
        """
        # L3: 会话级 rollup（异步入队，在丢弃前获取 meta）
        meta = self._session_meta.get(session_id)
        if meta and self._producer:
            await self._enqueue_rollup(
                tenant_id=meta.tenant_id,
                user_id=meta.user_id,
                session_id=session_id,
            )

        # L1: 丢弃
        self._session_meta.delete(session_id)

        logger.info("Session ended: %s", session_id)

    # ── 记忆操作方法 ──────────────────────────────────────────────────

    async def recall(
        self,
        scope: Scope,
        query: str = "",
    ) -> RecallResult:
        """召回记忆（L2 + L3 合并）。

        Args:
            scope: 查询范围。
            query: 查询文本（用于 L3 语义检索）。

        Returns:
            RecallResult 包含 L2 档案卡和 L3 召回摘要。
        """
        # L2: 获取整卡
        profile_items = await self._profile_card.get_profile(
            scope.tenant_id, scope.user_id
        )

        # L2: 紧凑序列化（≤1.5KB）
        profile_block = self._serialize_profile(profile_items)

        # L3: 语义召回（占位，Task 12 实现后启用）
        summary_items: list[RecalledItem] = []
        if self._summary_store and query:
            try:
                summary_items = await self._summary_store.recall(
                    scope=scope, query=query
                )
            except Exception as e:
                logger.warning("L3 recall failed, returning only L2: %s", e)

        return RecallResult(
            profile_block=profile_block,
            summary_items=summary_items,
        )

    async def save_summary(
        self,
        scope: Scope,
        content: str,
        topics: list[str] | None = None,
        entities: dict[str, list[str]] | None = None,
    ) -> Optional[str]:
        """保存对话摘要（L3）。

        Args:
            scope: 记忆范围。
            content: 摘要内容。
            topics: 主题列表。
            entities: 实体字典。

        Returns:
            摘要 ID 或 None（如果 L3 未启用）。
        """
        if self._summary_store is None:
            logger.warning("Summary store not initialized, skipping save_summary")
            return None

        try:
            summary_id = await self._summary_store.save_summary(
                scope=scope,
                content=content,
                topics=topics or [],
                entities=entities or {},
            )
            return summary_id
        except Exception as e:
            logger.error("Failed to save summary: %s", e)
            return None

    async def update_profile(
        self,
        tenant_id: str,
        user_id: str,
        slot: SlotType,
        item_key: str,
        item_value: Any,
        confidence: int = 50,
        source: SourceType = SourceType.DERIVED,
    ) -> ProfileUpdateResult:
        """更新用户档案（L2）。

        Args:
            tenant_id: 租户 ID。
            user_id: 用户 ID。
            slot: 槽位类型。
            item_key: 条目键。
            item_value: 条目值。
            confidence: 置信度。
            source: 来源类型。

        Returns:
            ProfileUpdateResult 包含更新结果。
        """
        return await self._profile_card.upsert_item(
            tenant_id=tenant_id,
            user_id=user_id,
            slot=slot,
            item_key=item_key,
            item_value=item_value,
            confidence=confidence,
            source=source,
        )

    async def forget(
        self,
        tenant_id: str,
        user_id: str,
        slot: SlotType,
        item_key: str,
    ) -> bool:
        """删除用户档案条目（L2）。

        Args:
            tenant_id: 租户 ID。
            user_id: 用户 ID。
            slot: 槽位类型。
            item_key: 条目键。

        Returns:
            是否成功删除。
        """
        return await self._profile_card.delete_item(
            tenant_id=tenant_id,
            user_id=user_id,
            slot=slot,
            item_key=item_key,
        )

    async def list_conflicts(
        self,
        tenant_id: str,
        user_id: str,
    ) -> list[ConflictRef]:
        """列出待处理的冲突（占位）。

        Args:
            tenant_id: 租户 ID。
            user_id: 用户 ID。

        Returns:
            冲突引用列表。
        """
        # Task 31 实现
        logger.info("list_conflicts called (not implemented yet)")
        return []

    async def resolve_conflict(
        self,
        tenant_id: str,
        user_id: str,
        conflict_id: str,
        resolution: str,  # "keep_old" | "take_new" | "manual"
        manual_value: Any = None,
    ) -> ProfileUpdateResult | None:
        """解决冲突（占位）。

        Args:
            tenant_id: 租户 ID。
            user_id: 用户 ID。
            conflict_id: 冲突 ID。
            resolution: 解决方案。
            manual_value: 手动值（如果 resolution == "manual"）。

        Returns:
            更新结果或 None。
        """
        # Task 31 实现
        logger.info("resolve_conflict called (not implemented yet)")
        return None

    # ── 辅助方法 ──────────────────────────────────────────────────────

    @staticmethod
    def _serialize_profile(items: list[ProfileItem]) -> str:
        """将档案卡序列化为紧凑文本（≤1.5KB）。

        Args:
            items: 档案条目列表。

        Returns:
            序列化文本。
        """
        if not items:
            return "暂无用户档案信息"

        parts = []
        for item in items:
            source_tag = "✓" if item.source == SourceType.USER_CONFIRMED else "◇"
            confirmed_tag = " [已确认]" if item.confirmed_at else ""
            parts.append(
                f"- {source_tag} [{item.slot.value}] {item.item_key}: {item.item_value} "
                f"(置信度:{item.confidence}%){confirmed_tag}"
            )

        # 截断到 1.5KB 以内
        text = "\n".join(parts)
        if len(text.encode("utf-8")) > 1500:
            logger.warning("Profile block exceeds 1.5KB limit, truncating")
            text = text[:1400] + "\n... (已截断)"

        return text

    # ── 异步队列集成 ──────────────────────────────────────────────────

    async def _enqueue_consolidate(
        self,
        tenant_id: str,
        user_id: str,
        session_id: str,
        turn_count: int,
    ) -> None:
        """入队巩固任务（每回合完成后调用）。"""
        if not self._producer:
            return

        try:
            payload = {
                "session_id": session_id,
                "user_id": user_id,
                "turn_count": turn_count,
                "trigger": "turn_complete",
            }
            await self._producer.enqueue(
                task_type="memory_consolidate",
                tenant_id=tenant_id,
                payload=payload,
                priority=0,
            )
            logger.debug(
                "Enqueued consolidate task: session=%s, turn=%d",
                session_id, turn_count,
            )
        except Exception as e:
            logger.warning("Failed to enqueue consolidate task: %s", e)

    async def _enqueue_rollup(
        self,
        tenant_id: str,
        user_id: str,
        session_id: str,
    ) -> None:
        """入队 rollup 任务（会话结束时调用）。"""
        if not self._producer:
            return

        try:
            payload = {
                "session_id": session_id,
                "user_id": user_id,
                "trigger": "session_end",
            }
            await self._producer.enqueue(
                task_type="memory_rollup",
                tenant_id=tenant_id,
                payload=payload,
                priority=1,  # 高优先级
            )
            logger.debug(
                "Enqueued rollup task: session=%s", session_id,
            )
        except Exception as e:
            logger.warning("Failed to enqueue rollup task: %s", e)


# ── 全局单例工厂 ──────────────────────────────────────────────────────────

_memory_service_instance: MemoryService | None = None


def set_memory_service(svc: MemoryService) -> None:
    """设置全局 MemoryService 实例（在应用启动时调用）。

    Args:
        svc: MemoryService 实例。
    """
    global _memory_service_instance
    _memory_service_instance = svc
    logger.info("MemoryService instance set globally")


def get_memory_service() -> MemoryService | None:
    """获取全局 MemoryService 实例。

    Returns:
        MemoryService 实例或 None（如果未初始化）。
    """
    return _memory_service_instance


def create_memory_service(redis: Any = None) -> MemoryService:
    """创建 MemoryService 实例（便捷工厂函数）。

    Args:
        redis: Redis 连接实例。

    Returns:
        MemoryService 实例。
    """
    session_meta_store = SessionMetaStore()
    profile_card = ProfileCard(redis) if redis else None

    return MemoryService(
        session_meta_store=session_meta_store,
        profile_card=profile_card,
        summary_store=None,  # Task 12 实现后启用
    )
