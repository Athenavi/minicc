# MiniCC Agent 记忆系统四层架构实施

## 1. 需求背景

### 1.1 现状问题

当前记忆能力散落在四个互不知晓的组件中，各有缺陷：

| 组件 | 现状 | 问题 |
|------|------|------|
| `SessionStore`（main.py:22） | Redis/内存 LRU（max 200），存完整消息列表 | 刷新即失忆；无持久化；无跨会话恢复 |
| `MemoryManager`（memory/manager.py） | 短期 Redis list + 长期 Milvus 向量 | 已被 prompt_engine 注入（:191），但写入无人调用——**只读不写的孤岛** |
| `MemoryStore`（memory/store.py） | 进程内 KV facts + 文件落盘，remember/recall/forget 工具 | 单例全局存储，**无租户/用户隔离**；Go deprecated 兼容层 |
| `ContextManager`（context/manager.py） | token 计数 + 80% 阈值 LLM 摘要 + trim_to_fit | 摘要结果直接丢弃，未沉淀为可检索记忆 |

**核心结论**：基础设施都在（Redis/PG/Milvus/嵌入网关/NER/摘要器），缺的是统一架构与流转机制。

### 1.2 用户需求

1. **跨会话记忆持久化**：Agent 应该能记住用户在之前会话中的重要信息
2. **语义检索能力**：能根据当前对话内容，召回相关的历史对话
3. **用户档案卡**：记录用户的身份、偏好、关键决策等结构化信息
4. **记忆冲突处理**：当记忆与用户确认的信息冲突时，应该询问用户
5. **前缀稳定**：保证对话缓存的高命中率
6. **多租户隔离**：记忆数据严格按租户/用户隔离

### 1.3 成功标准

- [ ] 每个PR的pytest全绿
- [ ] 新增模块测试覆盖 ≥90%
- [ ] fail-loud 断言同步更新
- [ ] 前缀稳定断言验证（compaction后messages[:retain]与之前逐字节一致）
- [ ] 多租户隔离测试通过
- [ ] 前端冲突流完整可用

## 2. 架构设计

### 2.1 四层架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                     Agent 主循环（每回合）                      │
│  prompt = 系统提示 + L2档案卡 + L3相关摘要 + L4原始窗口 + 本轮输入 │
└──────┬──────────────┬──────────────┬──────────────┬─────────┘
       │L1            │L2            │L3            │L4
  ┌────▼────┐   ┌─────▼─────┐  ┌─────▼──────┐ ┌────▼─────┐
  │会话元数据 │   │用户档案卡   │  │近期对话摘要  │ │ 滑动窗口  │
  │ 进程内存  │   │ PG JSONB  │  │ Milvus+PG  │ │  Redis   │
  │ 用完即丢  │   │ 精确KV查询 │  │ 唯一语义检索 │ │原始消息序列│
  └─────────┘   └───────────┘  └─────┬──────┘ └────┬─────┘
                                     │巩固          │巩固
                                L4滑出消息 ──摘要──► L3
                                L3稳定事实 ──提炼──► L2（冲突→询问用户）
```

### 2.2 分层职责

| 维度 | L1 会话元数据 | L2 用户档案卡 | L3 近期对话摘要 | L4 滑动窗口 |
|------|--------------|--------------|----------------|------------|
| **存储内容** | session_id、入口渠道、模式、回合数、token 用量 | 身份、偏好、长期事实、关键决策 | 会话级摘要 + 主题/实体 | 当前对话原始消息（含工具调用） |
| **存储介质** | 进程内存 dict | PostgreSQL JSONB | Milvus 向量 + PG 元数据 | Redis list |
| **访问频率** | 每回合读写（最高） | 每回合读（经缓存）；写低频 | 每回合 1 次语义查询；写每 N 回合 | 每回合读写（最高） |
| **容量限制** | ~1KB/会话；会话数上限沿用 200 | 32KB/用户；≤200 条目 | 注入预算 ≤6KB（top_k=5）；条目保留 90 天 | token 预算由 compaction 80% 阈值控制 |
| **生命周期** | 会话结束即丢弃 | 永久（衰退归档） | 衰退式遗忘 | 窗口滑出→巩固后丢弃 |
| **决策作用** | 路由/计费/审计/降级标记 | 系统提示个性化（"你是谁、你要什么"） | 相关历史召回（"上次聊过什么"） | 当前对话连贯性（"刚才说了什么"） |
| **检索方式** | 键直取 | 键值精确匹配（**不做语义检索**） | 向量语义检索（**全系统唯一**） | 顺序回放 |

### 2.3 设计原则

1. **语义检索只发生在 L3 一层**——控制嵌入成本与召回噪声
2. **每层只做一件事**——L4 管连贯、L3 管相关、L2 管稳定、L1 管簿记
3. **写路径异步、读路径同步**——巩固/嵌入走后台任务，召回在回合内必须确定路径
4. **不破坏前缀稳定**（L4 摘要式滑窗，保持前缀稳定）

## 3. 详细设计

### 3.1 L1：会话元数据（SessionMeta）

**数据结构**（新增 `app/memory/layers.py`）：

```python
@dataclass
class SessionMeta:
    session_id: str
    tenant_id: str
    user_id: str
    entry_channel: str        # "web" | "api" | "quick_execute" | "workflow"
    mode: str                 # agent 模式
    started_at: float
    last_active_at: float
    turn_count: int = 0
    total_tokens_in: int = 0
    total_tokens_out: int = 0
    degraded: bool = False    # LLM 摘要不可用等降级标记
    flags: dict[str, Any] = field(default_factory=dict)
```

**存储与生命周期**：
- 进程内存 `dict[session_id, SessionMeta]`，加 15 分钟空闲 TTL 清理
- **不持久化、不跨实例共享**
- 会话结束（显式 end 或 TTL）→ 触发一次 L4→L3 巩固后整体丢弃

### 3.2 L2：用户结构化档案卡（ProfileCard）

**数据结构**——PG 单表（迁移 `20260821000003_user_memory_profile.{up,down}.sql`）：

```sql
CREATE TABLE IF NOT EXISTS user_memory_profile (
    tenant_id   VARCHAR(64)  NOT NULL,
    user_id     VARCHAR(64)  NOT NULL,
    slot        VARCHAR(32)  NOT NULL,   -- identity / preference / decision / fact
    item_key    VARCHAR(128) NOT NULL,   -- 槽位内键，如 "timezone" / "preferred_language"
    item_value  JSONB        NOT NULL,   -- 值，允许对象
    confidence  SMALLINT     NOT NULL DEFAULT 50,        -- 0-100
    source      VARCHAR(16)  NOT NULL DEFAULT "derived", -- user_confirmed / derived / tool_written
    version     INTEGER      NOT NULL DEFAULT 1,
    confirmed_at TIMESTAMPTZ,            -- 用户最后确认时间（NULL=未确认）
    last_referenced_at TIMESTAMPTZ,      -- 最近被召回引用时间（衰退依据）
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id, slot, item_key)
);
CREATE INDEX idx_ump_reference ON user_memory_profile (tenant_id, user_id, last_referenced_at);
```

**槽位（slot）设计**：

| slot | 内容 | 典型 item_key | 写入来源 |
|------|------|--------------|---------|
| `identity` | 身份属性 | name / role / org / timezone | 用户显式陈述、SSO 档案初始化 |
| `preference` | 偏好 | output_language / code_style / detail_level | 对话中提炼 |
| `decision` | 关键决策 | "2026-08 决定用 PG 而非 Mongo" | 对话中提炼（须 ≥2 次出现或用户确认） |
| `fact` | 长期事实 | "负责 MiniCC 项目"、"团队 5 人" | 对话中提炼、remember 工具 |

**访问与更新规则**：
- **读**：全量 ≤200 条目、≤32KB，整卡读出后经 Redis 缓存（TTL 60s）
- **写**：槽位级 upsert（`ON CONFLICT ... DO UPDATE SET item_value=..., version=version+1`）
- **冲突规则**：
  - 新旧 `item_value` 不一致且旧值 `source=user_confirmed` → **不自动覆盖**，产出 `MemoryConflict` 事件
  - 旧值为 `derived` → 新值覆盖但 `version+1`
- **遗忘/衰退**：
  - 用户显式 forget → 硬删
  - `last_referenced_at` 超 180 天且 confidence < 80 → 归档
  - 用户否认 → 硬删 + 记 `negative_fact` 抑制条目

### 3.3 L3：近期对话摘要（唯一语义检索层）

**数据结构**——复用现有 Milvus `memory_store` collection，扩展 `memory_type`：

```
memory_type ∈ {"summary", "topic", "long_term(legacy)"}
```

每条记忆的 `metadata_json`：

```json
{
  "session_id": "s-xxx",
  "turn_range": [12, 30],
  "topics": ["数据库选型", "迁移策略"],
  "entities": {"person": [], "tech": ["PostgreSQL", "Milvus"], "url": []},
  "rollup_of": ["mem-id-1", "mem-id-2"],
  "content_hash": "sha256:...",
  "access_count": 3,
  "last_accessed_at": 1755753600,
  "status": "active"
}
```

**PG 侧镜像表**：

```sql
CREATE TABLE IF NOT EXISTS memory_summaries (
    id          VARCHAR(64) PRIMARY KEY,
    tenant_id   VARCHAR(64) NOT NULL,
    user_id     VARCHAR(64) NOT NULL,
    session_id  VARCHAR(64) NOT NULL,
    content     TEXT NOT NULL,
    topics      JSONB NOT NULL DEFAULT '[]',
    entities    JSONB NOT NULL DEFAULT '{}',
    content_hash VARCHAR(80) NOT NULL,
    access_count INTEGER NOT NULL DEFAULT 0,
    last_accessed_at TIMESTAMPTZ,
    status      VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 唯一索引用于 content_hash 去重
CREATE UNIQUE INDEX uq_ms_hash ON memory_summaries (tenant_id, user_id, content_hash);
```

**SummaryStore 实现**（`app/memory/summary_store.py`）：

```python
class SummaryStore:
    """L3 摘要存储 Provider。"""
    
    # 双写：save_summary 同时写入 PG 和 Milvus
    async def save_summary(self, scope, content, topics, entities, 
                           turn_start, turn_end, embedding):
        # 1. 插入 PG memory_summaries 表（含 ON CONFLICT 去重）
        # 2. 插入 Milvus memory_store collection
        pass
    
    # 语义检索：向量搜索 + final_score 排序
    async def recall(self, scope, query, top_k=5):
        # 1. 检查 Redis 查询缓存（key = sha256(tenant:user:query)，TTL 5min）
        # 2. 生成 query embedding（检查嵌入缓存，TTL 30min）
        # 3. Milvus 向量搜索（cosine ≥ 0.45 阈值）
        # 4. final_score 排序（recency×0.4 + access×0.3 + similarity×0.3）
        # 5. token 预算截断（6KB 硬上限）
        pass
```

**检索与排序**：

```
final_score = recency_decay × 0.4 + access_count_norm × 0.3 + cosine_similarity × 0.3
```

- 查询入口 `recall(query, top_k=5)`；**注入预算硬上限 6KB**
- 阈值：cosine ≥ 0.45
- 缓存策略：查询结果 Redis 5 分钟，嵌入向量 Redis 30 分钟

**遗忘/衰退规则**：
- 90 天未命中 → archived
- archived 条目保留 1 年后删除
- 同 `topics` 下条目 > 20 条 → 触发 rollup（记忆压缩）

### 3.4 L4：滑动窗口（当前对话原始上下文）

**采用的混合方案——"摘要式滑窗"**：
1. 消息序列保持 **append-only**（前缀稳定，缓存命中不受损）
2. token 预算达到 80% 阈值 → 走既有 compaction：**滑出部分先巩固到 L3（摘要入库），再以摘要块替换**
3. compaction 依赖的 LLM 摘要服务不可用 → **降级链**：重试 1 次 → 仍失败则 `trim_to_fit`（硬截断，标记 `L1.degraded=True`）
4. 窗口滑出的原始消息在巩固确认**之前不得从 SessionStore 删除**

## 4. 层间流转与同步机制

### 4.1 巩固（Consolidation）：L4 → L3 → L2

**触发时机**（三个入口，全部异步入队）：

| 触发 | 时机 | 动作 |
|------|------|------|
| 滚动巩固 | compaction 执行时（滑出内容） | 滑出消息 → 摘要 → L3 |
| 会话收尾 | `on_session_end`（显式结束 / TTL） | 整会话 rollup 摘要 → L3 |
| 定期 rollup | 每日凌晨 | 同主题 >20 条 → 合并压缩 |

**巩固 pipeline 步骤**（`app/memory/consolidator.py`）：

```
滑出消息 → ① LLM 摘要（ContextManager._summarise，失败则降级提取式摘要）
        → ② NER 实体 + 主题解析（规则式，零依赖）
        → ③ 去重检查（content_hash 精确 + 嵌入 cosine>0.95 近重复 → 合并）
        → ④ 写 L3（Milvus insert + PG 镜像，双写）
        → ⑤ 稳定事实探测：entities/topics 命中 L2 既有槽位 or 出现 ≥2 次跨会话
             → 是 → 产出"档案卡候选"（pending_confirmation 表）
             → 否 → 结束
```

**Consolidator 实现要点**：

```python
class Consolidator:
    """L4 滑出消息 → L3 摘要巩固 pipeline。"""
    
    async def consolidate(self, tenant_id, user_id, session_id, 
                          messages, turn_start, turn_end):
        # 步骤①: LLM 摘要（或降级）
        summary = await self._summarise_or_fallback(messages)
        
        # 步骤②: NER 实体 + 主题解析
        entities = self._extract_entities(summary)
        topics = self._extract_topics(summary)
        
        # 步骤③: 去重检查
        content_hash = self._compute_hash(summary)
        existing = await self._store.get_by_hash(tenant_id, user_id, content_hash)
        if existing:
            return ConsolidateResult(deduplicated=True)
        
        # 步骤④: 生成 embedding 并双写
        embedding = await self._embedder(summary)
        entry = await self._store.save_summary(...)
        
        # 步骤⑤: 稳定事实探测（可选）
        return ConsolidateResult(summary=entry)
```

**异步队列集成**（`app/memory/service.py`）：
- `on_turn_complete`: 入队 `memory_consolidate` 任务（每回合触发）
- `on_session_end`: 入队 `memory_rollup` 任务（会话结束时触发，高优先级）
- Worker 处理：`app/queue/worker.py` 中 `handle_memory_consolidate` / `handle_memory_rollup`

### 4.2 检索与召回（每回合 prompt 组装）

```
recalled = await memory.recall(
    scope=Scope(tenant_id, user_id, session_id),
    query=当前用户消息 + 最近1条助手消息拼接,
    token_budget=8_000
)
```

**组装顺序**：

```
[系统提示]
── 记忆：用户档案 ──        ← L2 整卡紧凑序列化（≤1.5KB）
── 记忆：相关历史 ──        ← L3 top_k=5 按 final_score 排序（≤6KB）
[L4 原始窗口消息（前缀稳定）]
[本轮用户输入]
```

## 5. 接口规范

### 5.1 核心门面：MemoryService

新增 `app/memory/service.py`：

```python
class MemoryService:
    def __init__(self, session_meta_store, profile_card, summaries,
                 session_cache, gateway, queue=None): ...

    # ── 会话生命周期 ──
    async def on_session_start(self, meta: SessionMeta) -> SessionContext:
        """L1 建立；从 PG/Milvus 恢复该用户 L2 档案卡缓存 + 最近 3 条 L3 摘要预取。"""

    async def on_turn_complete(self, session_id: str, messages: list[dict],
                               usage: TokenUsage) -> None:
        """L1 记账；L4 append（内部判断 compaction 触发）；异步入队巩固。"""

    async def on_session_end(self, session_id: str) -> None:
        """触发会话级 rollup 巩固 → L1 丢弃。"""

    # ── 读路径（每回合，同步、fail-soft）──
    async def recall(self, scope: Scope, query: str,
                     token_budget: int = 8_000) -> RecallResult:
        """返回 RecallResult(profile_block: str, summary_items: list[RecalledItem])。"""

    # ── 写路径 ──
    async def save_summary(self, scope: Scope, content: str,
                           turn_range: tuple[int, int]) -> str:
        """L3 写入（巩固 pipeline 内部调用；也暴露给测试）。返回 memory_id。"""

    async def update_profile(self, scope: Scope, slot: str, key: str,
                             value: Any, *, confirm: bool = False) -> ProfileUpdateResult:
        """L2 upsert。confirm=True 表示用户显式确认；冲突时返回 pending 的 ConflictRef。"""

    # ── 删除 ──
    async def forget(self, target: MemoryRef) -> bool:
        """MemoryRef = ("profile", slot, key) | ("summary", memory_id)。"""

    # ── 冲突 ──
    async def list_conflicts(self, user_id: str) -> list[MemoryConflict]: ...
    async def resolve_conflict(self, conflict_id: str, resolution: str,
                               manual_value: Any = None) -> None:
        """resolution ∈ {"keep_old", "adopt_new", "manual"}。"""
```

### 5.2 与 Agent 主循环集成

改造点集中在 `runtime.py`：

```
execute() 开头:
  ctx = await memory.on_session_start(SessionMeta(...))     # ← 新增
  messages = session_cache.get_or_init(...)                  # ← 现有逻辑保留
  recalled = await memory.recall(scope, query)               # ← 新增
  task.messages = prompt_engine.build(task, recalled)        # ← prompt_engine 记忆区分段

execute() 结尾:
  await memory.on_turn_complete(session_id, messages, usage) # ← L4 append + compaction 编排
```

### 5.3 与工具调用流程集成

改造 `app/tools/memory.py` 三个工具：

| 工具 | 现行为 | 新行为 |
|------|--------|--------|
| `remember(key, value)` | 写全局 KV 单例 | `update_profile(slot="fact", key, value, confirm=False)` → L2 |
| `recall(query)` | 子串搜索全局 facts | L2 精确匹配 + L3 语义 top_k 合并返回 |
| `forget(key)` | 删全局 KV | `forget(("profile", slot, key))` |
| （新增）`memory_search(query)` | — | 直查 L3 语义层 |

### 5.4 冲突询问流（前端）

```
MemoryConflict{conflict_id, slot, key, old_value, new_value, source}
  → SSE 事件 "memory_conflict" 推送到前端
  → ChatView 内联确认卡片：「检测到记忆冲突——旧: X / 新: Y」
     [保留旧值] [采用新值] [手动修改]
  → POST /v1/memory/conflicts/{id}/resolve {resolution, manual_value}
  → resolve_conflict() 写 L2
```

新增 API 路由（挂 `app/api/memory.py`，Go 网关 newProxy 代理）：

```
GET    /v1/memory/profile                 # 读整卡
PUT    /v1/memory/profile/{slot}/{key}    # 用户手工维护档案卡
DELETE /v1/memory/profile/{slot}/{key}
GET    /v1/memory/summaries?limit=50      # 查看摘要记忆
GET    /v1/memory/conflicts               # 待裁决冲突列表
POST   /v1/memory/conflicts/{id}/resolve  # 裁决
```

## 6. 实施计划

| PR | 内容 | 交付物 | 依赖 |
|----|------|--------|------|
| **PR-1 基础层** | `layers.py`（SessionMeta/Scope/dataclass）+ L2 PG 表迁移 + `ProfileCard` provider + `MemoryService` 门面骨架 + remember/recall/forget 工具改接 L2 | 迁移对、单元测试 | 无 |
| **PR-2 摘要层** | `consolidator.py` 巩固 pipeline + Milvus memory_type 扩展 + PG 镜像表 + 去重/rollup + `save_summary/recall` 语义查询 | 单元测试（mock gateway embed） | PR-1 |
| **PR-3 主循环集成** | AgentRuntime 挂接三生命周期 hook + prompt_engine 分段注入 + compaction 编排 + 降级链 | 集成测试（验证前缀稳定） | PR-1/2 |
| **PR-4 冲突与前端** | conflict 事件流 + 6 个 REST API + Go 网关代理 + ChatView 确认卡片 + Profile 页"记忆管理"卡片 | vitest 组件测试 + e2e | PR-3 |

## 7. 受影响文件清单

### 新增文件

```
python-engine/app/memory/layers.py
python-engine/app/memory/service.py
python-engine/app/memory/consolidator.py
python-engine/app/api/memory.py
migrations/20260821000003_user_memory_profile.up.sql
migrations/20260821000003_user_memory_profile.down.sql
migrations/20260821000004_memory_summaries.up.sql
migrations/20260821000004_memory_summaries.down.sql
frontend-vue/src/views/ProfileView.vue (扩展)
frontend-vue/src/components/MemoryConflictCard.vue (新)
```

### 修改文件

```
python-engine/app/db.py (ensure_tables 双轨)
python-engine/app/agent/runtime.py (生命周期集成)
python-engine/app/agent/prompt_engine.py (记忆分段注入)
python-engine/app/context/manager.py (复用摘要)
python-engine/app/tools/memory.py (改接 MemoryService)
python-engine/app/skill/store.py (移除全局单例引用)
internal/api/gateway_router.go (新增 6 个 API 代理)
internal/engine/python_client.go (内部调用)
frontend-vue/src/views/ChatView.vue (冲突流)
frontend-vue/src/api/index.ts (API 封装)
```

### 删除文件

```
python-engine/app/memory/store.py (全局单例，已废弃)
```

## 8. 测试策略

### 单元测试

**PR-1 测试（已完成）**：
- **L2 单元** (`test_profile_card.py`, 32 用例)：槽位 upsert 幂等、version 递增、冲突不覆盖 user_confirmed、衰退归档、200 条目淘汰
- **MemoryService 单元** (`test_service.py`, 44 用例)：on_session_start/end/turn_complete 生命周期、工具集成、序列化格式

**PR-2 测试（已完成）**：
- **SummaryStore 单元** (`test_summary_store.py`, 46 用例)：save_summary 双写、recall 向量检索、final_score 排序、查询缓存、token 预算截断
- **Consolidator 单元** (`test_consolidator.py`, 44 用例)：摘要提取、NER 实体抽取、去重逻辑、完整 pipeline、幂等性
- **L3 语义查询单元** (`test_l3_semantic_query.py`, 24 用例)：L2+L3 合并、去重逻辑、fail-soft 降级、边界条件

**测试结果**：186 个用例全部通过，新增模块覆盖率：
- `consolidator.py`: 99%
- `layers.py`: 95%
- `profile_card.py`: 95%
- `summary_store.py`: 77%（异常路径待补充）
- `service.py`: 84%（异常路径待补充）

### 集成测试

- **主循环集成**：前缀稳定断言（compaction 后 messages[:retain] 与之前逐字节一致）
- **降级链**：LLM 摘要失败 → trim + degraded 标记
- **兼容性**：memory=None 时行为回归一致
- **多租户**：Milvus expr / PG 行级 tenant 隔离、跨租户零泄漏

### 组件测试

- **冲突流**：derived 二次出现自动写入、user_confirmed 冲突挂起、三选项裁决生效、否认 → 抑制名单

## 9. 风险与缓解

1. **LLM 摘要质量**：摘要失真会把错误固化进 L3/L2
   - 缓解：摘要 prompt 要求保留实体原文；用户可经 Profile 页查看/删除全部记忆

2. **嵌入成本**：每次巩固 1 次 embed + 每回合 1 次查询 embed
   - 缓解：查询缓存 + 攒批；本地 sentence-transformers 优先链路

3. **Milvus 不可用**：读路径 fail-soft（该段缺省）；写路径入队重试，队列积压超限告警

4. **隐私合规**：记忆默认租户内隔离；提供"清空我的记忆"（级联 L2/L3/归档）

5. **L4 评估遗留**：若实测发现 compaction 头尾保留策略与超长工具调用序列冲突
   - 观察项：引入"工具结果引用化"（工具大输出转存对象存储、窗口留指针）

## 10. 验收标准

- [ ] 所有单元测试通过（pytest，覆盖率≥90%）
- [ ] 所有集成测试通过
- [ ] 前端冲突流完整可用
- [ ] 前缀稳定断言验证通过
- [ ] 多租户隔离测试通过
- [ ] 性能指标达标（L2缓存命中率>95%，L3召回<500ms）
- [ ] 降级链测试通过
- [ ] Go 侧代理测试通过
- [ ] 前端 e2e 测试通过
- [ ] 文档完整（API 文档、架构文档）
