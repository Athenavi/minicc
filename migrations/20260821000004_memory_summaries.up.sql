-- 20260821000004_memory_summaries: up
-- 目的：创建对话摘要表（L3层），用于存储跨会话的语义摘要记忆。
-- 设计：
--   1. 摘要存储：保存对话摘要内容、主题、实体信息
--   2. 去重机制：content_hash 唯一索引，避免重复摘要
--   3. 访问追踪：access_count 统计召回次数，last_accessed_at 支持排序
--   4. 状态管理：active / archived / expired，支持后台清理
--   5. 行号范围：turn_start / turn_end 记录摘要覆盖的对话轮次
-- 依赖：无

-- ── memory_summaries 表 ─────────────────────────────────────────
CREATE TABLE IF NOT EXISTS memory_summaries (
    id              VARCHAR(64) PRIMARY KEY,
    tenant_id       VARCHAR(64) NOT NULL,
    user_id         VARCHAR(64) NOT NULL,
    session_id      VARCHAR(64) NOT NULL,
    content         TEXT NOT NULL,
    topics          JSONB NOT NULL DEFAULT '[]',
    entities        JSONB NOT NULL DEFAULT '{}',
    turn_start      INTEGER NOT NULL DEFAULT 0,
    turn_end        INTEGER NOT NULL DEFAULT 0,
    content_hash    VARCHAR(80) NOT NULL,
    access_count    INTEGER NOT NULL DEFAULT 0,
    last_accessed_at TIMESTAMPTZ,
    status          VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 索引：支持按租户/用户/状态/时间查询（最常用场景）
CREATE INDEX idx_ms_lookup ON memory_summaries(tenant_id, user_id, status, created_at DESC);

-- 唯一索引：content_hash 去重，避免重复摘要
CREATE UNIQUE INDEX uq_ms_hash ON memory_summaries(tenant_id, user_id, content_hash);

-- 索引：支持访问频率排序
CREATE INDEX idx_ms_access ON memory_summaries(tenant_id, user_id, last_accessed_at DESC NULLS LAST, access_count DESC);

-- 索引：支持会话级查询
CREATE INDEX idx_ms_session ON memory_summaries(session_id) WHERE status = 'active';

-- 约束：status 必须为预定义值
ALTER TABLE memory_summaries
    ADD CONSTRAINT chk_ms_status
    CHECK (status IN ('active', 'archived', 'expired'));

-- 约束：turn_start <= turn_end
ALTER TABLE memory_summaries
    ADD CONSTRAINT chk_ms_turn_range
    CHECK (turn_start <= turn_end);

-- 约束：access_count >= 0
ALTER TABLE memory_summaries
    ADD CONSTRAINT chk_ms_access_count
    CHECK (access_count >= 0);

-- 注释
COMMENT ON TABLE memory_summaries IS '对话摘要表（L3层）：存储跨会话的语义摘要记忆';
COMMENT ON COLUMN memory_summaries.content IS '摘要内容文本';
COMMENT ON COLUMN memory_summaries.topics IS '主题列表（JSONB数组）';
COMMENT ON COLUMN memory_summaries.entities IS '实体字典（JSONB对象），按类型分组';
COMMENT ON COLUMN memory_summaries.turn_start IS '摘要覆盖的起始轮次';
COMMENT ON COLUMN memory_summaries.turn_end IS '摘要覆盖的结束轮次';
COMMENT ON COLUMN memory_summaries.content_hash IS '内容哈希，用于去重';
COMMENT ON COLUMN memory_summaries.access_count IS '访问次数，用于计算 final_score';
COMMENT ON COLUMN memory_summaries.last_accessed_at IS '最近访问时间，用于计算 recency';
COMMENT ON COLUMN memory_summaries.status IS '状态：active（活跃）、archived（已归档）、expired（已过期）';