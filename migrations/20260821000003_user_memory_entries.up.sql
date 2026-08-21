-- 用户长期记忆条目表（记忆四层架构 L2 档案卡）
-- 四类槽位: identity(身份) / preference(偏好) / decision(关键决策) / fact(长期事实)
-- 跨会话留存；embedding 列存 JSONB 向量供语义检索（条目量级 ≤200/用户，进程内 cosine + 重排序即可）
CREATE TABLE IF NOT EXISTS user_memory_entries (
    id               VARCHAR(64) PRIMARY KEY,
    tenant_id        VARCHAR(64) NOT NULL DEFAULT 'default',
    user_id          VARCHAR(64) NOT NULL,
    slot             VARCHAR(32) NOT NULL,
    item_key         VARCHAR(128) NOT NULL,
    item_value       TEXT NOT NULL,
    confidence       SMALLINT NOT NULL DEFAULT 50,
    source           VARCHAR(16) NOT NULL DEFAULT 'derived',
    embedding        JSONB,
    access_count     INTEGER NOT NULL DEFAULT 0,
    last_accessed_at TIMESTAMPTZ,
    status           VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, user_id, slot, item_key)
);

CREATE INDEX IF NOT EXISTS idx_ume_lookup
    ON user_memory_entries (tenant_id, user_id, status);
CREATE INDEX IF NOT EXISTS idx_ume_access
    ON user_memory_entries (tenant_id, user_id, last_accessed_at);
