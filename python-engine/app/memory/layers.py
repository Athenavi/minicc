"""记忆四层架构 — 数据结构与槽位定义。

L2 用户档案卡（user_memory_entries 表）承载跨会话长期记忆：
- identity(身份) / preference(偏好) / decision(关键决策) / fact(长期事实) 四类槽位
- 纯 KV 精确查询为主；条目级 embedding 供「记忆管理页」的语义检索 + 重排序
  （条目量级 ≤200/用户，进程内 cosine 即可，不引入额外向量库依赖）

向量与排序工具函数集中于本模块，供 ProfileStore / MemoryService 复用。
"""
from __future__ import annotations

import math
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Optional

# ── 槽位定义 ──────────────────────────────────────────────
SLOTS: tuple[str, ...] = ("identity", "preference", "decision", "fact")
SLOT_LABELS: dict[str, str] = {
    "identity": "身份",
    "preference": "偏好",
    "decision": "关键决策",
    "fact": "长期事实",
}
SOURCES: tuple[str, ...] = ("user_confirmed", "derived", "tool_written")
SOURCE_LABELS: dict[str, str] = {
    "user_confirmed": "用户确认",
    "derived": "对话提炼",
    "tool_written": "工具写入",
}


def is_valid_slot(slot: str) -> bool:
    return slot in SLOTS


@dataclass
class MemoryEntry:
    """一条长期记忆（表 user_memory_entries 的一行）。"""

    id: str
    tenant_id: str
    user_id: str
    slot: str
    item_key: str
    item_value: str
    confidence: int = 50
    source: str = "derived"
    embedding: Optional[list[float]] = None
    access_count: int = 0
    last_accessed_at: Optional[datetime] = None
    status: str = "active"
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None

    def to_dict(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "slot": self.slot,
            "slot_label": SLOT_LABELS.get(self.slot, self.slot),
            "key": self.item_key,
            "value": self.item_value,
            "confidence": self.confidence,
            "source": self.source,
            "source_label": SOURCE_LABELS.get(self.source, self.source),
            "has_embedding": self.embedding is not None and len(self.embedding) > 0,
            "access_count": self.access_count,
            "last_accessed_at": self.last_accessed_at.isoformat() if self.last_accessed_at else None,
            "status": self.status,
            "created_at": self.created_at.isoformat() if self.created_at else None,
            "updated_at": self.updated_at.isoformat() if self.updated_at else None,
        }


@dataclass
class SummaryEntry:
    """L3 近期对话摘要（memory_summaries 表的一行 + Milvus 向量）。"""

    id: str
    tenant_id: str
    user_id: str
    session_id: str
    content: str
    topics: list[str] = field(default_factory=list)
    entities: dict[str, list[str]] = field(default_factory=dict)
    turn_start: int = 0
    turn_end: int = 0
    content_hash: str = ""
    embedding: Optional[list[float]] = None
    access_count: int = 0
    last_accessed_at: Optional[datetime] = None
    status: str = "active"
    created_at: Optional[datetime] = None

    def to_dict(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "session_id": self.session_id,
            "content": self.content,
            "topics": self.topics,
            "entities": self.entities,
            "turn_range": [self.turn_start, self.turn_end],
            "has_embedding": self.embedding is not None and len(self.embedding) > 0,
            "access_count": self.access_count,
            "last_accessed_at": self.last_accessed_at.isoformat() if self.last_accessed_at else None,
            "status": self.status,
            "created_at": self.created_at.isoformat() if self.created_at else None,
        }

    @property
    def embed_text(self) -> str:
        """嵌入用文本 = 摘要正文（可加 topics 前缀增强语义匹配）。"""
        prefix = ", ".join(self.topics) if self.topics else ""
        return f"{prefix}: {self.content}" if prefix else self.content


@dataclass
class RecallResult:
    """一次记忆召回的结果（L2 档案卡 + L3 摘要）。"""

    profile_block: str = ""
    summary_items: list[dict[str, Any]] = field(default_factory=list)

    @property
    def has_content(self) -> bool:
        return bool(self.profile_block) or bool(self.summary_items)


@dataclass
class MemoryConflict:
    """记忆冲突（新旧值不一致且旧值 user_confirmed）。"""

    conflict_id: str
    tenant_id: str
    user_id: str
    slot: str
    item_key: str
    old_value: str
    new_value: str
    old_source: str
    new_source: str
    status: str = "pending"  # pending / resolved
    resolution: str = ""     # keep_old / adopt_new / manual
    resolved_value: str = ""
    created_at: Optional[datetime] = None

    def to_dict(self) -> dict[str, Any]:
        return {
            "conflict_id": self.conflict_id,
            "slot": self.slot,
            "slot_label": SLOT_LABELS.get(self.slot, self.slot),
            "key": self.item_key,
            "old_value": self.old_value,
            "new_value": self.new_value,
            "old_source": self.old_source,
            "new_source": self.new_source,
            "status": self.status,
            "resolution": self.resolution or None,
            "resolved_value": self.resolved_value or None,
            "created_at": self.created_at.isoformat() if self.created_at else None,
        }


@dataclass
class OrganizeResult:
    """一次「智能整理」的产出统计。"""

    backfilled: int = 0   # 补齐嵌入向量的条目数
    merged: int = 0       # 合并的近重复条目数
    archived: int = 0     # 归档的衰退条目数
    evicted: int = 0      # 容量超限淘汰的条目数
    errors: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "backfilled": self.backfilled,
            "merged": self.merged,
            "archived": self.archived,
            "evicted": self.evicted,
            "errors": self.errors,
        }


# ── 向量工具 ──────────────────────────────────────────────

def cosine_similarity(a: list[float], b: list[float]) -> float:
    """余弦相似度（维度不一致或零向量返回 0.0）。"""
    if not a or not b or len(a) != len(b):
        return 0.0
    dot = sum(x * y for x, y in zip(a, b))
    na = math.sqrt(sum(x * x for x in a))
    nb = math.sqrt(sum(x * x for x in b))
    if na == 0.0 or nb == 0.0:
        return 0.0
    return dot / (na * nb)


def recency_decay(ref: Optional[datetime], half_life_days: float = 60.0) -> float:
    """新近度衰减因子：exp(-Δdays / half_life)。无引用时间按 0.5 处理。"""
    if ref is None:
        return 0.5
    now = datetime.now(timezone.utc)
    ref = ref if ref.tzinfo else ref.replace(tzinfo=timezone.utc)
    days = max(0.0, (now - ref).total_seconds() / 86400.0)
    return math.exp(-days / half_life_days)


def rerank_score(
    similarity: float,
    confidence: int,
    last_ref: Optional[datetime],
    keyword_hit: bool,
) -> float:
    """重排序打分：语义相似度 × (置信度+新近度加权) + 关键词命中加成。"""
    base = similarity * (0.65 + 0.35 * max(0, min(100, confidence)) / 100.0)
    base *= 0.7 + 0.3 * recency_decay(last_ref)
    if keyword_hit:
        base += 0.15
    return base
