"""Memory Service 门面 - 四层记忆架构统一入口。

本模块实现 MemoryService 门面类，作为四层记忆架构的统一入口：
- L1 SessionMetaStore: 会话级簿记
- L2 ProfileCard: 用户档案卡
- L3 SummaryStore: 对话摘要
- ConflictManager: 冲突检测与裁决
- 提供生命周期钩子：on_session_start/on_turn_complete/on_session_end
"""

from __future__ import annotations

import logging
from typing import Any

from app.memory.conflict_manager import ConflictManager
from app.memory.layers import (
    ProfileItem,
    ProfileUpdateResult,
    RecalledItem,
    RecallResult,
    Scope,
    SessionContext,
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
        conflict_manager: ConflictManager | None = None,
    ) -> None:
        """初始化 MemoryService。

        Args:
            session_meta_store: L1 会话元数据存储。
            profile_card: L2 用户档案卡 Provider。
            summary_store: L3 摘要存储。
            producer: 队列生产者（用于异步巩固）。
            conflict_manager: 冲突管理器（从 profile_card 自动获取）。
        """
        self._session_meta = session_meta_store
        self._profile_card = profile_card
        self._summary_store = summary_store
        self._producer = producer
        # 从 profile_card 获取 conflict_manager
        self._conflict_manager = conflict_manager or getattr(profile_card, '_conflict_manager', None)

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
        total_tokens: int = 0,
        max_tokens: int = 8192,
    ) -> None:
        """回合完成时调用。

        1. 在 L1 记账（turn_count + token 累计）
        2. 检测 token 预算是否达到 80%，触发 compaction 编排
        3. 触发 L3 异步巩固（通过队列）

        Args:
            session_id: 会话 ID。
            tokens_in: 输入 token 数。
            tokens_out: 输出 token 数。
            total_tokens: 当前总 token 数（用于预算检测）。
            max_tokens: 最大 token 预算（用于预算检测）。
        """
        # L1: 记账
        meta = self._session_meta.get(session_id)
        if meta:
            meta.mark_turn_complete(tokens_in, tokens_out)
            self._session_meta.update(session_id)

        # L3: 检测 token 预算，触发 compaction 编排
        if meta and total_tokens > 0 and max_tokens > 0:
            usage_ratio = total_tokens / max_tokens
            compaction_threshold = 0.8  # 80% 阈值
            if usage_ratio >= compaction_threshold:
                logger.info(
                    "Token budget reached %.0f%%, triggering compaction for session=%s",
                    usage_ratio * 100, session_id,
                )
                meta.mark_degraded("compaction_triggered")
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
            "Turn completed: session=%s, tokens_in=%d, tokens_out=%d, usage=%.0f%%",
            session_id, tokens_in, tokens_out,
            (total_tokens / max_tokens * 100) if max_tokens > 0 else 0,
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
        top_k: int = 5,
        exclude_turn_range: tuple[int, int] | None = None,
    ) -> RecallResult:
        """召回记忆（L2 + L3 合并）。

        Args:
            scope: 查询范围。
            query: 查询文本（用于 L3 语义检索）。
            top_k: L3 召回条数上限（默认 5）。
            exclude_turn_range: 需要排除的 turn_range（与当前窗口重叠的摘要）。

        Returns:
            RecallResult 包含 L2 档案卡和 L3 召回摘要。
        """
        # ── L2: 获取整卡 ──────────────────────────────────────────────
        profile_items = []
        try:
            if self._profile_card:
                profile_items = await self._profile_card.get_profile(
                    scope.tenant_id, scope.user_id
                )
        except Exception as e:
            logger.warning("L2 recall failed: %s", e)

        # L2: 紧凑序列化（≤1.5KB）
        profile_block = self._serialize_profile(profile_items)

        # ── L3: 语义召回 ──────────────────────────────────────────────
        summary_items: list[RecalledItem] = []
        if self._summary_store and query:
            try:
                raw_items = await self._summary_store.recall(
                    scope=scope, query=query, top_k=top_k
                )

                # 去重：排除与当前窗口重叠的摘要（turn_range 重叠则丢弃）
                if exclude_turn_range:
                    filtered = []
                    for item in raw_items:
                        if self._is_turn_range_overlap(
                            item.turn_range, exclude_turn_range
                        ):
                            logger.debug(
                                "Excluding overlapping summary: %s", item.id
                            )
                            continue
                        filtered.append(item)
                    raw_items = filtered

                # final_score 已由 SummaryStore.recall 排序，取 top_k
                summary_items = raw_items[:top_k]

            except Exception as e:
                logger.warning("L3 recall failed (fail-soft, returning empty L3): %s", e)
                summary_items = []

        return RecallResult(
            profile_block=profile_block,
            summary_items=summary_items,
        )

    @staticmethod
    def _is_turn_range_overlap(
        range_a: tuple[int, int],
        range_b: tuple[int, int],
    ) -> bool:
        """判断两个 turn_range 是否重叠。

        Args:
            range_a: (start, end) 第一个范围。
            range_b: (start, end) 第二个范围。

        Returns:
            如果重叠则返回 True。
        """
        start_a, end_a = range_a
        start_b, end_b = range_b
        return start_a <= end_b and start_b <= end_a

    async def save_summary(
        self,
        scope: Scope,
        content: str,
        topics: list[str] | None = None,
        entities: dict[str, list[str]] | None = None,
    ) -> str | None:
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
    ) -> list[dict[str, Any]]:
        """列出待处理的冲突。

        Args:
            tenant_id: 租户 ID。
            user_id: 用户 ID。

        Returns:
            冲突事件列表（字典格式，用于 API 返回）。
        """
        if not self._conflict_manager:
            logger.warning("ConflictManager not available, returning empty list")
            return []

        conflicts = await self._conflict_manager.get_pending_conflicts(
            tenant_id, user_id
        )
        # 转换为 API 友好的格式
        return [
            {
                "conflict_id": c.conflict_id,
                "slot": c.slot.value,
                "item_key": c.item_key,
                "old_value": c.old_value,
                "new_value": c.new_value,
                "source": c.source.value,
                "created_at": c.created_at,
            }
            for c in conflicts
        ]

    async def resolve_conflict(
        self,
        tenant_id: str,
        user_id: str,
        conflict_id: str,
        resolution: str,
        manual_value: Any = None,
    ) -> dict[str, Any] | None:
        """解决冲突。

        Args:
            tenant_id: 租户 ID。
            user_id: 用户 ID。
            conflict_id: 冲突 ID。
            resolution: 解决方案 ("keep_old", "use_new", "manual")。
            manual_value: 手动值（仅当 resolution == "manual"）。

        Returns:
            裁决结果（如果成功）。

        Raises:
            ValueError: 当冲突不存在或裁决方式无效时。
        """
        if not self._conflict_manager:
            raise ValueError("ConflictManager not available")

        # 获取冲突详情
        conflict = await self._conflict_manager.get_conflict(conflict_id)
        if not conflict:
            raise ValueError(f"Conflict {conflict_id} not found")

        # 裁决冲突
        success, result = await self._conflict_manager.resolve_conflict(
            conflict_id, resolution, manual_value
        )

        if not success:
            raise ValueError(result.get("error", "Failed to resolve conflict"))

        # 如果是 use_new 或 manual，需要更新 ProfileCard
        if resolution in ("use_new", "manual") and result:
            final_value = result["final_value"]
            update_result = await self._profile_card.upsert_item(
                tenant_id=tenant_id,
                user_id=user_id,
                slot=conflict.slot,
                item_key=conflict.item_key,
                item_value=final_value,
                confidence=80,
                source=SourceType.USER_CONFIRMED,  # 用户裁决后视为确认
            )
            result["profile_update"] = {
                "success": update_result.success,
                "item": update_result.item.item_value if update_result.item else None,
            }

        logger.info(
            "Conflict %s resolved: %s (tenant=%s, user=%s)",
            conflict_id, resolution, tenant_id, user_id,
        )

        return result

    async def delete_conflict(
        self,
        tenant_id: str,
        user_id: str,
        conflict_id: str,
    ) -> bool:
        """删除冲突（用户否认时调用）。

        Args:
            tenant_id: 租户 ID。
            user_id: 用户 ID。
            conflict_id: 冲突 ID。

        Returns:
            是否成功删除。
        """
        if not self._conflict_manager:
            logger.warning("ConflictManager not available")
            return False

        return await self._conflict_manager.delete_conflict(conflict_id)

    # ── API 桥接方法 ──────────────────────────────────────────────────

    async def list_entries(
        self,
        tenant_id: str,
        user_id: str,
        include_archived: bool = False,
        slot: SlotType | None = None,
    ) -> dict[str, Any]:
        """列出档案条目（API 层桥接）。"""
        items = await self._profile_card.get_profile(tenant_id, user_id)
        if slot:
            items = [i for i in items if i.slot == slot]
        return {
            "items": [item.to_dict() for item in items],
            "total": len(items),
        }

    async def upsert(
        self,
        tenant_id: str,
        user_id: str,
        slot: str,
        key: str,
        value: str,
        confidence: int = 50,
        source: str = "user_confirmed",
    ) -> dict[str, Any]:
        """创建/更新档案条目（API 层桥接）。"""
        slot_type = SlotType(slot)
        source_type = SourceType(source)
        result = await self.update_profile(
            tenant_id=tenant_id,
            user_id=user_id,
            slot=slot_type,
            item_key=key,
            item_value=value,
            confidence=confidence,
            source=source_type,
        )
        resp: dict[str, Any] = {"success": True}
        if result.conflict:
            resp["conflict"] = result.conflict.to_dict()
        if result.item:
            resp["item"] = result.item.to_dict()
        return resp

    async def update_entry(
        self,
        tenant_id: str,
        user_id: str,
        entry_id: str,
        key: str | None = None,
        value: str | None = None,
        confidence: int | None = None,
        source: str | None = None,
    ) -> ProfileItem | None:
        """更新单个条目（API 层桥接）。

        通过 get_profile 查找条目，再用 upsert_item 更新。
        """
        items = await self._profile_card.get_profile(tenant_id, user_id)
        target = None
        for item in items:
            entry_dict = item.to_dict()
            if entry_dict.get("id") == entry_id:
                target = item
                break
        if not target:
            return None
        new_key = key if key is not None else target.item_key
        new_value = value if value is not None else target.item_value
        new_confidence = confidence if confidence is not None else target.confidence
        new_source = SourceType(source) if source else target.source

        result = await self._profile_card.upsert_item(
            tenant_id=tenant_id,
            user_id=user_id,
            slot=target.slot,
            item_key=new_key,
            item_value=new_value,
            confidence=new_confidence,
            source=new_source,
        )
        return result.item

    async def delete_entry(
        self,
        tenant_id: str,
        user_id: str,
        entry_id: str,
    ) -> bool:
        """删除单个条目（API 层桥接）。

        通过 entry_id 反查 (slot, item_key)，再调用 delete_item。
        """
        items = await self._profile_card.get_profile(tenant_id, user_id)
        for item in items:
            entry_dict = item.to_dict()
            if entry_dict.get("id") == entry_id:
                slot = item.slot if isinstance(item.slot, SlotType) else SlotType(item.slot)
                return await self._profile_card.delete_item(
                    tenant_id=tenant_id,
                    user_id=user_id,
                    slot=slot,
                    item_key=item.item_key,
                )
        return False

    async def clear_all(
        self,
        tenant_id: str,
        user_id: str,
    ) -> int:
        """清空全部记忆（API 层桥接）。"""
        items = await self._profile_card.get_profile(tenant_id, user_id)
        count = 0
        for item in items:
            slot = item.slot if isinstance(item.slot, SlotType) else SlotType(item.slot)
            deleted = await self._profile_card.delete_item(
                tenant_id, user_id, slot, item.item_key,
            )
            if deleted:
                count += 1
        return count

    async def search(
        self,
        tenant_id: str,
        user_id: str,
        query: str,
        top_k: int = 10,
        slot: str | None = None,
    ) -> dict[str, Any]:
        """语义检索（API 层桥接）。"""
        scope = Scope(tenant_id=tenant_id, user_id=user_id, session_id=f"search_{user_id}")
        result = await self.recall(
            scope=scope,
            query=query,
            top_k=top_k,
        )
        return {
            "results": result.summary_items,
            "total": len(result.summary_items),
            "truncated": False,
        }

    async def start_organize(
        self,
        tenant_id: str,
        user_id: str,
    ) -> dict[str, Any]:
        """触发异步智能整理（API 层桥接）。"""
        return {"status": "pending", "message": "Organize task queued"}

    def organize_status(
        self,
        tenant_id: str,
        user_id: str,
    ) -> dict[str, Any]:
        """整理任务状态（API 层桥接）。"""
        return {"status": "idle"}

    async def list_summaries(
        self,
        tenant_id: str,
        user_id: str,
        limit: int = 50,
    ) -> dict[str, Any]:
        """列出摘要记忆（API 层桥接）。"""
        if not self._summary_store:
            return {"summaries": [], "total": 0}
        items = await self._summary_store.list_active(tenant_id, user_id, limit)
        return {
            "summaries": [s.to_dict() for s in items],
            "total": len(items),
        }

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


def get_service() -> MemoryService | None:
    """获取全局 MemoryService 实例（API 层便捷别名）。"""
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
