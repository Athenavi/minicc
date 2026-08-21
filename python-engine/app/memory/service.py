"""MemoryService — 记忆系统门面（本轮：L2 档案卡全生命周期）。

职责（对 API / 工具层是唯一入口）：
- 档案卡 CRUD（槽位校验、容量护栏、键值 upsert）
- 语义检索：embed(query) → 进程内 cosine → 重排序（置信度 × 新近度 × 关键词加成）
- 智能整理（手动/自动异步）：补嵌入 → 近重复合并 → 衰退归档 → 容量淘汰
- 整理任务状态跟踪（每用户单飞行锁，防并发重入）

fail-loud/fail-soft 约定：
- 写路径（upsert/delete）：存储异常直接上抛；
- 嵌入为「尽力而为」：embedder 不可用时条目照常入库（embedding=NULL），
  整理任务稍后补齐——检索降级为关键词匹配，绝不因嵌入失败阻断记忆写入。
"""
from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import dataclass, field
from typing import Any, Awaitable, Callable, Optional

from app.config import settings
from app.memory.layers import (
    MemoryConflict,
    MemoryEntry,
    OrganizeResult,
    RecallResult,
    SLOTS,
    SOURCES,
    SummaryEntry,
    cosine_similarity,
    recency_decay,
    rerank_score,
)
from app.memory.profile import ProfileStore, new_entry_id
from app.memory.summaries import SummaryStore, compute_hash as compute_summary_hash
from app.memory.consolidator import Consolidator, ConsolidateResult

logger = logging.getLogger(__name__)

Embedder = Callable[[str], Awaitable[list[float]]]

_MAX_VALUE_CHARS = 4000
_MAX_KEY_CHARS = 128


@dataclass
class SearchHit:
    entry: MemoryEntry
    similarity: float   # 原始 cosine（0-1）
    score: float        # 重排序分数

    def to_dict(self) -> dict[str, Any]:
        d = self.entry.to_dict()
        d["similarity"] = round(self.similarity, 4)
        d["score"] = round(self.score, 4)
        return d


@dataclass
class _OrganizeState:
    running: bool = False
    started_at: float = 0.0
    finished_at: float = 0.0
    result: Optional[OrganizeResult] = None
    error: str = ""

    def to_dict(self) -> dict[str, Any]:
        return {
            "running": self.running,
            "started_at": self.started_at or None,
            "finished_at": self.finished_at or None,
            "result": self.result.to_dict() if self.result else None,
            "error": self.error or None,
        }


class MemoryService:
    """L2 档案卡 + L3 摘要服务。"""

    def __init__(
        self,
        store: ProfileStore,
        embedder: Optional[Embedder] = None,
        summary_store: Optional[SummaryStore] = None,
        consolidator: Optional[Consolidator] = None,
    ) -> None:
        self._store = store
        self._embedder = embedder
        self._summary_store = summary_store
        self._consolidator = consolidator
        self._organize_states: dict[str, _OrganizeState] = {}
        self._organize_locks: dict[str, asyncio.Lock] = {}
        self._conflicts: dict[str, MemoryConflict] = {}  # PR-4: 进程内冲突暂存

    # ── 嵌入（尽力而为）────────────────────────────────

    async def _embed(self, text: str) -> Optional[list[float]]:
        if self._embedder is None:
            return None
        try:
            return await self._embedder(text)
        except Exception as e:  # noqa: BLE001 — 嵌入失败不阻断主流程
            logger.warning("memory embed failed: %s", e)
            return None

    # ── 档案卡 CRUD ────────────────────────────────────

    async def upsert(
        self,
        tenant_id: str,
        user_id: str,
        slot: str,
        key: str,
        value: str,
        confidence: int = 50,
        source: str = "derived",
    ) -> dict[str, Any]:
        """新建或更新一条记忆。返回 {entry, created, duplicate_of}。

        - 同 (slot, key) 已存在 → 更新值并重算嵌入（版本语义）
        - 值与既有**同槽位其他条目**语义近重复（cosine>阈值）→ 照常写入，
          但返回 duplicate_of 提示（整理任务会合并）
        - 容量护栏：active 条目数达上限时淘汰最低分的 derived 条目
        """
        slot = (slot or "").strip().lower()
        key = (key or "").strip()
        value = (value or "").strip()
        if slot not in SLOTS:
            raise ValueError(f"invalid slot: {slot} (expected one of {SLOTS})")
        if not key or len(key) > _MAX_KEY_CHARS:
            raise ValueError(f"key required (1-{_MAX_KEY_CHARS} chars)")
        if not value or len(value) > _MAX_VALUE_CHARS:
            raise ValueError(f"value required (1-{_MAX_VALUE_CHARS} chars)")
        if source not in SOURCES:
            raise ValueError(f"invalid source: {source} (expected one of {SOURCES})")
        confidence = max(0, min(100, int(confidence)))

        # 容量护栏（仅新建路径需要；更新不增量）
        existing = await self._store.get_by_key(tenant_id, user_id, slot, key)
        evicted = 0
        if existing is None:
            count = await self._store.count(tenant_id, user_id)
            if count >= settings.memory_profile_max_items:
                evicted = await self._evict_lowest(tenant_id, user_id)

        embedding = await self._embed(self._embed_text(key, value))
        if existing is not None:
            # 冲突检测：旧值 user_confirmed 且新值不同 → 挂起冲突，不自动覆盖
            if (
                existing.source == "user_confirmed"
                and source != "user_confirmed"
                and existing.item_value != value
            ):
                import uuid as _uuid
                conflict = MemoryConflict(
                    conflict_id=f"cfl_{_uuid.uuid4().hex[:16]}",
                    tenant_id=tenant_id,
                    user_id=user_id,
                    slot=slot,
                    item_key=key,
                    old_value=existing.item_value,
                    new_value=value,
                    old_source=existing.source,
                    new_source=source,
                )
                self._conflicts[conflict.conflict_id] = conflict
                result: dict[str, Any] = {
                    "entry": existing.to_dict(),
                    "created": False,
                    "conflict": conflict.to_dict(),
                }
                return result
            entry = await self._store.update(
                tenant_id, user_id, existing.id,
                item_value=value, confidence=confidence, source=source,
                embedding=embedding, embedding_set=True,
            )
            created = False
        else:
            entry = await self._store.insert(MemoryEntry(
                id=new_entry_id(), tenant_id=tenant_id, user_id=user_id,
                slot=slot, item_key=key, item_value=value,
                confidence=confidence, source=source, embedding=embedding,
            ))
            created = True

        # 近重复探测（仅提示，不阻断）
        duplicate_of: Optional[dict[str, Any]] = None
        if embedding:
            dup = await self._find_near_duplicate(entry)
            if dup is not None:
                duplicate_of = {"id": dup.id, "key": dup.item_key, "value": dup.item_value}

        result: dict[str, Any] = {"entry": entry.to_dict(), "created": created}
        if duplicate_of:
            result["duplicate_of"] = duplicate_of
        if evicted:
            result["evicted"] = evicted
        return result

    async def _evict_lowest(self, tenant_id: str, user_id: str) -> int:
        """容量超限：淘汰最低「置信度×新近度」的 derived 条目（用户确认过的永不淘汰）。"""
        entries = await self._store.list(tenant_id, user_id)
        candidates = [e for e in entries if e.source != "user_confirmed"]
        if not candidates:
            return 0
        worst = min(
            candidates,
            key=lambda e: (
                e.confidence * 0.5
                + (e.access_count or 0) * 2
                + (time.time() - (e.updated_at.timestamp() if e.updated_at else 0)) / 86400 * -1
            ),
        )
        await self._store.delete(tenant_id, user_id, worst.id)
        return 1

    async def _find_near_duplicate(self, entry: MemoryEntry) -> Optional[MemoryEntry]:
        """在同槽位中查找与 entry 语义近重复的其他条目。"""
        if not entry.embedding:
            return None
        siblings = await self._store.list(tenant_id=entry.tenant_id, user_id=entry.user_id, slot=entry.slot)
        best: Optional[MemoryEntry] = None
        best_sim = 0.0
        for other in siblings:
            if other.id == entry.id or not other.embedding:
                continue
            sim = cosine_similarity(entry.embedding, other.embedding)
            if sim > best_sim:
                best_sim, best = sim, other
        if best is not None and best_sim > settings.memory_dedup_threshold:
            return best
        return None

    @staticmethod
    def _embed_text(key: str, value: str) -> str:
        """嵌入文本 = 槽位语义 + key + value（让不同槽位的同名词可区分）。"""
        return f"{key}: {value}"

    async def list_entries(
        self, tenant_id: str, user_id: str, include_archived: bool = False
    ) -> dict[str, Any]:
        entries = await self._store.list(tenant_id, user_id, include_archived=include_archived)
        counts = {s: 0 for s in SLOTS}
        for e in entries:
            if e.slot in counts and e.status == "active":
                counts[e.slot] += 1
        return {
            "entries": [e.to_dict() for e in entries],
            "counts": counts,
            "total": sum(counts.values()),
        }

    async def update_entry(
        self,
        tenant_id: str,
        user_id: str,
        entry_id: str,
        *,
        key: Optional[str] = None,
        value: Optional[str] = None,
        confidence: Optional[int] = None,
        source: Optional[str] = None,
    ) -> Optional[MemoryEntry]:
        current = await self._store.get_by_id(tenant_id, user_id, entry_id)
        if current is None:
            return None
        if key is not None:
            key = key.strip()
            if not key or len(key) > _MAX_KEY_CHARS:
                raise ValueError(f"key required (1-{_MAX_KEY_CHARS} chars)")
        if value is not None:
            value = value.strip()
            if not value or len(value) > _MAX_VALUE_CHARS:
                raise ValueError(f"value required (1-{_MAX_VALUE_CHARS} chars)")
        if confidence is not None:
            confidence = max(0, min(100, int(confidence)))
        if source is not None and source not in SOURCES:
            raise ValueError(f"invalid source: {source}")

        new_value = value if value is not None else current.item_value
        new_key = key if key is not None else current.item_key
        embedding = await self._embed(self._embed_text(new_key, new_value))
        return await self._store.update(
            tenant_id, user_id, entry_id,
            item_key=key, item_value=value, confidence=confidence, source=source,
            embedding=embedding, embedding_set=embedding is not None,
        )

    async def delete_entry(self, tenant_id: str, user_id: str, entry_id: str) -> bool:
        return await self._store.delete(tenant_id, user_id, entry_id)

    async def forget_by_key(
        self, tenant_id: str, user_id: str, key: str, slot: Optional[str] = None
    ) -> int:
        return await self._store.delete_by_key(tenant_id, user_id, key, slot)

    async def clear_all(self, tenant_id: str, user_id: str) -> int:
        """清空该用户全部记忆（隐私出口，含归档条目）。"""
        return await self._store.delete_all(tenant_id, user_id)

    # ── 语义检索（相似度 + 重排序）──────────────────────

    async def search(
        self,
        tenant_id: str,
        user_id: str,
        query: str,
        top_k: int = 10,
        slot: Optional[str] = None,
    ) -> dict[str, Any]:
        query = (query or "").strip()
        if not query:
            raise ValueError("query is required")
        top_k = max(1, min(50, int(top_k)))
        if slot is not None and slot not in SLOTS:
            raise ValueError(f"invalid slot: {slot}")

        entries = await self._store.list(tenant_id, user_id, slot=slot)
        q_lower = query.lower()
        qvec = await self._embed(query)

        hits: list[SearchHit] = []
        for e in entries:
            kw_hit = q_lower in e.item_key.lower() or q_lower in e.item_value.lower()
            sim = 0.0
            if qvec and e.embedding:
                sim = cosine_similarity(qvec, e.embedding)
            if qvec is not None:
                # 嵌入可用：语义召回下限；关键词命中可豁免下限
                if sim < settings.memory_search_min_cosine and not kw_hit:
                    continue
            else:
                # 嵌入不可用：降级为纯关键词检索
                if not kw_hit:
                    continue
                sim = 0.5
            score = rerank_score(sim, e.confidence, e.last_accessed_at, kw_hit)
            hits.append(SearchHit(entry=e, similarity=sim, score=score))

        hits.sort(key=lambda h: h.score, reverse=True)
        top = hits[:top_k]
        if top:
            await self._store.touch([h.entry.id for h in top])
        return {
            "query": query,
            "mode": "semantic" if qvec is not None else "keyword",
            "count": len(top),
            "results": [h.to_dict() for h in top],
        }

    # ── 智能整理（异步）────────────────────────────────

    def _state(self, user_key: str) -> _OrganizeState:
        return self._organize_states.setdefault(user_key, _OrganizeState())

    def _lock(self, user_key: str) -> asyncio.Lock:
        return self._organize_locks.setdefault(user_key, asyncio.Lock())

    async def start_organize(self, tenant_id: str, user_id: str) -> dict[str, Any]:
        """触发异步整理（单飞行：已有任务在跑时返回 running）。"""
        user_key = f"{tenant_id}:{user_id}"
        state = self._state(user_key)
        if state.running:
            return {"started": False, "reason": "already_running", "status": state.to_dict()}
        task = asyncio.create_task(self._run_organize(tenant_id, user_id))
        task.add_done_callback(lambda _: None)
        return {"started": True}

    def organize_status(self, tenant_id: str, user_id: str) -> dict[str, Any]:
        return self._state(f"{tenant_id}:{user_id}").to_dict()

    async def organize_now(self, tenant_id: str, user_id: str) -> OrganizeResult:
        """同步整理（内部/测试入口；线上走 start_organize 异步路径）。"""
        return await self._do_organize(tenant_id, user_id)

    async def _run_organize(self, tenant_id: str, user_id: str) -> None:
        user_key = f"{tenant_id}:{user_id}"
        state = self._state(user_key)
        lock = self._lock(user_key)
        async with lock:
            state.running = True
            state.started_at = time.time()
            state.error = ""
            try:
                state.result = await self._do_organize(tenant_id, user_id)
            except Exception as e:  # noqa: BLE001 — 整理失败记入状态，不崩任务
                logger.error("memory organize failed for %s: %s", user_key, e)
                state.error = str(e)
            finally:
                state.running = False
                state.finished_at = time.time()

    async def _do_organize(self, tenant_id: str, user_id: str) -> OrganizeResult:
        result = OrganizeResult()
        entries = await self._store.list(tenant_id, user_id)

        # ① 补嵌入：无向量条目逐条嵌入（整理是非关键路径，失败跳过并记录）
        for e in entries:
            if e.embedding:
                continue
            emb = await self._embed(self._embed_text(e.item_key, e.item_value))
            if emb is None:
                result.errors.append(f"embedding unavailable, {len([x for x in entries if not x.embedding])} entries left unbackfilled")
                break
            await self._store.set_embedding(e.id, emb)
            e.embedding = emb
            result.backfilled += 1

        # ② 近重复合并：同槽位 cosine > 阈值 → 保留高分方（置信度优先、新者优先）
        by_slot: dict[str, list[MemoryEntry]] = {}
        for e in entries:
            if e.status == "active":
                by_slot.setdefault(e.slot, []).append(e)
        for slot_entries in by_slot.values():
            removed_ids: set[str] = set()
            for i, a in enumerate(slot_entries):
                if a.id in removed_ids or not a.embedding:
                    continue
                for b in slot_entries[i + 1:]:
                    if b.id in removed_ids or not b.embedding:
                        continue
                    if cosine_similarity(a.embedding, b.embedding) > settings.memory_dedup_threshold:
                        keep, drop = (a, b) if _keep_rank(a) >= _keep_rank(b) else (b, a)
                        await self._store.delete(tenant_id, user_id, drop.id)
                        removed_ids.add(drop.id)
                        result.merged += 1

        # ③ 衰退归档：超期未引用且低置信 → archived（不删，管理端可查）
        #    注意：第②步已物理删除的条目 archive() 会返回 False，不会误计数
        now = time.time()
        for e in entries:
            if e.status != "active":
                continue
            ref_ts = e.last_accessed_at.timestamp() if e.last_accessed_at else (
                e.created_at.timestamp() if e.created_at else now
            )
            idle_days = (now - ref_ts) / 86400.0
            if idle_days > settings.memory_archive_days and e.confidence < 80:
                if await self._store.archive(e.id):
                    result.archived += 1

        return result

    # ── L3 摘要：巩固 + 语义检索 ──────────────────────

    async def save_summary(
        self,
        tenant_id: str,
        user_id: str,
        session_id: str,
        messages: list[dict],
        turn_start: int = 0,
        turn_end: int = 0,
    ) -> dict[str, Any]:
        """巩固一批消息为 L3 摘要（consolidator pipeline）。"""
        if self._consolidator is None:
            raise RuntimeError("consolidator not bound (summary_store/consolidator missing)")
        result = await self._consolidator.consolidate(
            tenant_id, user_id, session_id, messages, turn_start, turn_end,
        )
        return result.to_dict()

    async def recall_summaries(
        self,
        tenant_id: str,
        user_id: str,
        query: str,
        top_k: int = 0,
    ) -> dict[str, Any]:
        """L3 语义检索：query 嵌入 → 进程内 cosine → 重排序。"""
        if self._summary_store is None:
            return {"query": query, "mode": "unavailable", "count": 0, "results": []}
        top_k = top_k or settings.memory_summary_top_k
        query = (query or "").strip()
        if not query:
            return {"query": "", "mode": "empty", "count": 0, "results": []}

        entries = await self._summary_store.list_active(tenant_id, user_id, limit=100)
        if not entries:
            return {"query": query, "mode": "empty", "count": 0, "results": []}

        qvec = await self._embed(query)
        hits: list[dict[str, Any]] = []
        for e in entries:
            sim = 0.0
            if qvec and e.embedding:
                sim = cosine_similarity(qvec, e.embedding)
            elif qvec is None:
                # 嵌入不可用：降级关键词
                if query.lower() not in e.content.lower():
                    continue
                sim = 0.5
            else:
                continue
            if qvec is not None and sim < settings.memory_summary_min_cosine:
                continue
            decay = recency_decay(e.last_accessed_at or e.created_at, half_life_days=30.0)
            score = sim * decay * (1 + 0.1 * min(e.access_count, 10))
            hits.append({**e.to_dict(), "similarity": round(sim, 4), "score": round(score, 4)})

        hits.sort(key=lambda h: h["score"], reverse=True)
        top = hits[:top_k]
        # touch 命中条目
        for h in top:
            await self._summary_store.touch(h["id"])
        return {
            "query": query,
            "mode": "semantic" if qvec is not None else "keyword",
            "count": len(top),
            "results": top,
        }

    async def recall(
        self,
        tenant_id: str,
        user_id: str,
        query: str,
    ) -> RecallResult:
        """每回合记忆召回：L2 档案卡紧凑块 + L3 相关摘要。"""
        profile_block = ""
        summary_items: list[dict[str, Any]] = []

        # L2：整卡序列化（confidence >= 50 的 active 条目）
        try:
            entries = await self._store.list(tenant_id, user_id)
            lines: list[str] = []
            for e in entries:
                if e.status != "active" or e.confidence < 50:
                    continue
                lines.append(f"- [{e.slot}] {e.item_key}: {e.item_value}")
            if lines:
                profile_block = "\n".join(lines[:50])  # ≤1.5KB
        except Exception as e:
            logger.warning("recall L2 failed: %s", e)

        # L3：语义检索 top_k
        try:
            sr = await self.recall_summaries(tenant_id, user_id, query)
            summary_items = sr.get("results", [])
        except Exception as e:
            logger.warning("recall L3 failed: %s", e)

        return RecallResult(profile_block=profile_block, summary_items=summary_items)

    async def list_summaries(
        self, tenant_id: str, user_id: str, limit: int = 50
    ) -> dict[str, Any]:
        """管理端审计：列出摘要记忆。"""
        if self._summary_store is None:
            return {"summaries": [], "count": 0}
        entries = await self._summary_store.list_active(tenant_id, user_id, limit)
        return {"summaries": [e.to_dict() for e in entries], "count": len(entries)}

    # ── 冲突裁决（PR-4）────────────────────────────────

    def list_conflicts(self, tenant_id: str, user_id: str) -> list[dict[str, Any]]:
        """列出待裁决的冲突。"""
        return [
            c.to_dict() for c in self._conflicts.values()
            if c.tenant_id == tenant_id and c.user_id == user_id and c.status == "pending"
        ]

    async def resolve_conflict(
        self, conflict_id: str, resolution: str, manual_value: str = ""
    ) -> dict[str, Any]:
        """裁决冲突：keep_old / adopt_new / manual。"""
        conflict = self._conflicts.get(conflict_id)
        if conflict is None:
            raise ValueError(f"conflict not found: {conflict_id}")
        if resolution not in ("keep_old", "adopt_new", "manual"):
            raise ValueError(f"invalid resolution: {resolution}")

        if resolution == "keep_old":
            final_value = conflict.old_value
        elif resolution == "adopt_new":
            final_value = conflict.new_value
        else:
            final_value = manual_value or conflict.new_value

        # 写入裁决后的值（source=user_confirmed, confidence=100）
        embedding = await self._embed(self._embed_text(conflict.item_key, final_value))
        existing = await self._store.get_by_key(
            conflict.tenant_id, conflict.user_id, conflict.slot, conflict.item_key,
        )
        if existing is not None:
            await self._store.update(
                conflict.tenant_id, conflict.user_id, existing.id,
                item_value=final_value, confidence=100, source="user_confirmed",
                embedding=embedding, embedding_set=embedding is not None,
            )

        conflict.status = "resolved"
        conflict.resolution = resolution
        conflict.resolved_value = final_value
        return conflict.to_dict()


def _keep_rank(e: MemoryEntry) -> tuple[int, float]:
    """合并保留优先级：置信度高 > 更新时间新。"""
    updated = e.updated_at.timestamp() if e.updated_at else 0.0
    return (e.confidence, updated)


# ── 模块级单例（main.py lifespan 注入；与 llm_client.bind_gateway 同模式）──

_memory_service: Optional[MemoryService] = None


def bind_service(svc: MemoryService) -> None:
    global _memory_service
    _memory_service = svc


def get_service() -> Optional[MemoryService]:
    return _memory_service


def require_service() -> MemoryService:
    """fail-loud 获取：未初始化（无 PostgreSQL）时显式报错。"""
    if _memory_service is None:
        raise RuntimeError(
            "memory service unavailable: PostgreSQL pool not initialized "
            "(check postgres_dsn / engine startup logs)"
        )
    return _memory_service
