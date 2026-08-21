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
