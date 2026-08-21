-- L3 近期对话摘要 — PG 镜像表（向量之外的运维视图 + 归档冷存储）
-- Milvus memory_store collection 扩展 memory_type='summary'，本表存正文与元数据

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

CREATE INDEX IF NOT EXISTS idx_ms_lookup ON memory_summaries (tenant_id, user_id, status, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_ms_hash ON memory_summaries (tenant_id, user_id, content_hash);
