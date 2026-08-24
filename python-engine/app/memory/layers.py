"""四层记忆架构的核心数据模型定义。

本模块定义了记忆系统四层架构的核心数据结构和类型：
- L1 SessionMeta: 会话元数据（进程内存）
- L2 ProfileCard 相关类型: 用户档案卡（PostgreSQL）
- L3 SummaryStore 相关类型: 对话摘要（Milvus + PG）
- Scope: 记忆查询范围
- 事件类型: MemoryConflict 等
"""
from __future__ import annotations

import time
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Literal


# ── L1: 会话元数据 ─────────────────────────────────────────────────────


@dataclass
class SessionMeta:
    """L1 会话元数据（进程内存，用完即丢）。

    存储会话级别的簿记数据：session_id、入口渠道、模式、回合数、token 用量等。
    与对话内容无关，纯粹为路由、计费、审计、降级标记服务。
    """
    session_id: str
    tenant_id: str
    user_id: str
    entry_channel: Literal["web", "api", "quick_execute", "workflow"]
    mode: str  # agent 模式
    started_at: float
    last_active_at: float
    turn_count: int = 0
    total_tokens_in: int = 0
    total_tokens_out: int = 0
    degraded: bool = False  # LLM 摘要不可用等降级标记
    flags: dict[str, Any] = field(default_factory=dict)

    def mark_turn_complete(self, tokens_in: int, tokens_out: int) -> None:
        """标记回合完成，更新计数器和时间戳。"""
        self.turn_count += 1
        self.total_tokens_in += tokens_in
        self.total_tokens_out += tokens_out
        self.last_active_at = time.time()

    def mark_degraded(self, reason: str = "") -> None:
        """标记会话降级。"""
        self.degraded = True
        if reason:
            self.flags["degraded_reason"] = reason


@dataclass
class SessionContext:
    """会话上下文（on_session_start 返回）。"""
    meta: SessionMeta
    profile_cached: bool  # L2 档案卡是否已缓存
    summaries_prefetched: int  # 预取的 L3 摘要数量


# ── 通用类型 ─────────────────────────────────────────────────────────────


@dataclass
class Scope:
    """记忆查询范围（tenant + user + session）。"""
    tenant_id: str
    user_id: str
    session_id: str


class MemoryType(str, Enum):
    """记忆类型引用（Milvus memory_type 取值）。"""
    PROFILE = "profile"
    SUMMARY = "summary"
    TOPIC = "topic"
    LONG_TERM = "long_term"
    SHORT_TERM = "short_term"


# ── L2: 用户档案卡相关类型 ──────────────────────────────────────────────


class SlotType(str, Enum):
    """L2 档案卡槽位类型。"""
    IDENTITY = "identity"  # 身份属性
    PREFERENCE = "preference"  # 偏好
    DECISION = "decision"  # 关键决策
    FACT = "fact"  # 长期事实


class SourceType(str, Enum):
    """记忆来源类型。"""
    USER_CONFIRMED = "user_confirmed"  # 用户显式确认
    DERIVED = "derived"  # Agent 提炼
    TOOL_WRITTEN = "tool_written"  # 工具写入


SLOTS: list[str] = [s.value for s in SlotType]

SLOT_LABELS: dict[str, str] = {
    "identity": "身份信息",
    "preference": "偏好设置",
    "decision": "关键决策",
    "fact": "长期事实",
}


@dataclass
class ProfileItem:
    """L2 档案卡单个条目。"""
    slot: SlotType
    item_key: str
    item_value: Any
    confidence: int  # 0-100
    source: SourceType
    version: int
    confirmed_at: float | None  # 用户最后确认时间（NULL=未确认）
    last_referenced_at: float | None  # 最近被召回引用时间
    created_at: float
    updated_at: float
    id: str | None = None  # 唯一标识符（用于 API 路由）

    def to_dict(self) -> dict[str, Any]:
        slot_val = self.slot.value if isinstance(self.slot, SlotType) else self.slot
        source_val = self.source.value if isinstance(self.source, SourceType) else self.source
        return {
            "id": self.id or f"{slot_val}:{self.item_key}",
            "slot": slot_val,
            "item_key": self.item_key,
            "item_value": self.item_value,
            "confidence": self.confidence,
            "source": source_val,
            "version": self.version,
            "confirmed_at": self.confirmed_at,
            "last_referenced_at": self.last_referenced_at,
            "created_at": self.created_at,
            "updated_at": self.updated_at,
        }


@dataclass
class MemoryEntry:
    """L2 用户记忆条目（= user_memory_entries 行模型，profile.py 持久层使用）。"""
    id: str
    tenant_id: str
    user_id: str
    slot: Any
    item_key: str
    item_value: Any
    confidence: int  # 0-100
    source: Any
    embedding: list[float] | None = None
    access_count: int = 0
    last_accessed_at: Any = None
    status: str = "active"
    created_at: Any = None
    updated_at: Any = None


@dataclass
class ProfileUpdateResult:
    """L2 档案卡更新结果。"""
    success: bool
    item: ProfileItem | None
    conflict: ConflictRef | None = None


@dataclass
class ConflictRef:
    """冲突引用（当 L2 更新时与 user_confirmed 冲突）。"""
    conflict_id: str
    slot: SlotType
    item_key: str
    old_value: Any
    new_value: Any
    old_source: SourceType
    old_confirmed_at: float | None

    def to_dict(self) -> dict[str, Any]:
        return {
            "conflict_id": self.conflict_id,
            "slot": self.slot.value,
            "item_key": self.item_key,
            "old_value": self.old_value,
            "new_value": self.new_value,
            "old_source": self.old_source.value,
            "old_confirmed_at": self.old_confirmed_at,
            "status": "pending",
        }


# ── L3: 对话摘要相关类型 ────────────────────────────────────────────────


@dataclass
class SummaryEntry:
    """L3 对话摘要条目（存储用）。"""
    id: str
    tenant_id: str
    user_id: str
    session_id: str
    content: str
    topics: list[str]
    entities: dict[str, list[str]]
    turn_start: int
    turn_end: int
    content_hash: str
    access_count: int
    last_accessed_at: float | None
    status: str = "active"
    created_at: float = 0.0
    embedding: list[float] | None = None


@dataclass
class RecalledItem:
    """L3 召回的单个摘要项。"""
    id: str
    content: str
    topics: list[str]
    entities: dict[str, list[str]]  # {"person": [], "tech": [], "url": []}
    turn_range: tuple[int, int]
    session_id: str
    access_count: int
    last_accessed_at: float
    created_at: float
    score: float  # final_score


@dataclass
class RecallResult:
    """recall 方法返回结果。"""
    profile_block: str  # L2 档案卡紧凑序列化（≤1.5KB）
    summary_items: list[RecalledItem]  # L3 召回的摘要（≤6KB）

    @property
    def has_content(self) -> bool:
        """是否有有效内容（L2 档案卡或 L3 摘要）。"""
        # 注意：profile_block 中的 "暂无用户档案信息" 是无档案时的占位文本，
        # 不应视为有效内容。
        has_profile = bool(self.profile_block.strip()) and "暂无用户档案信息" not in self.profile_block
        return has_profile or len(self.summary_items) > 0


# ── 事件类型 ─────────────────────────────────────────────────────────────


@dataclass
class MemoryConflict:
    """记忆冲突事件（SSE 推送）。"""
    conflict_id: str
    slot: SlotType
    item_key: str
    old_value: Any
    new_value: Any
    source: SourceType
    tenant_id: str
    user_id: str
    created_at: float


# ── 其他类型 ─────────────────────────────────────────────────────────────


@dataclass
class MemoryRef:
    """记忆引用（用于 forget 操作）。"""
    memory_type: MemoryType
    # 如果 memory_type == "profile"
    slot: SlotType | None = None
    item_key: str | None = None
    # 如果 memory_type == "summary"
    memory_id: str | None = None


@dataclass
class TokenUsage:
    """Token 使用统计。"""
    prompt_tokens: int
    completion_tokens: int
    total_tokens: int

    @property
    def tokens_in(self) -> int:
        return self.prompt_tokens

    @property
    def tokens_out(self) -> int:
        return self.completion_tokens


# ── 工具函数 ─────────────────────────────────────────────────────────────


def cosine_similarity(a: list[float], b: list[float]) -> float:
    """计算两个向量的余弦相似度。"""
    import math

    if len(a) != len(b):
        return 0.0

    dot_product = sum(x * y for x, y in zip(a, b))
    norm_a = math.sqrt(sum(x * x for x in a))
    norm_b = math.sqrt(sum(x * x for x in b))

    if norm_a == 0 or norm_b == 0:
        return 0.0

    return dot_product / (norm_a * norm_b)