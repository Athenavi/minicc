-- 20260822000002_security_hardening: up
-- 目的：生产安全加固批量迁移
--   1. api_keys 补 revoked 列（AuthMiddleware 引用但缺失，导致 API Key 认证整体 401）
--   2. /search 全文检索补 GIN 表达式索引（消除全表扫描）
--   3. tool_calls / sessions 常用查询列补索引
--   4. episodes 死表补 tenant_id 隔离列 + 索引 + 外键

-- ── 1. api_keys.revoked ──
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'api_keys' AND column_name = 'revoked') THEN
        ALTER TABLE api_keys ADD COLUMN revoked boolean NOT NULL DEFAULT false;
    END IF;
END $$;

-- ── 2. /search 全文检索 GIN 索引 ──
-- messages.content 使用 to_tsvector('simple', content) @@ plainto_tsquery(...)
CREATE INDEX IF NOT EXISTS idx_messages_content_tsv
    ON messages USING GIN (to_tsvector('simple', content));
-- media_assets.name 同理
CREATE INDEX IF NOT EXISTS idx_media_assets_name_tsv
    ON media_assets USING GIN (to_tsvector('simple', COALESCE(name, '')));

-- ── 3. tool_calls / sessions 常用查询索引 ──
CREATE INDEX IF NOT EXISTS idx_tool_calls_session_created
    ON tool_calls (session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_calls_created
    ON tool_calls (created_at);
-- sessions 列表按 (user_id, updated_at DESC) 排序
CREATE INDEX IF NOT EXISTS idx_sessions_user_updated
    ON sessions (user_id, updated_at DESC);

-- ── 4. episodes 补租户隔离（参照 tenant_isolation 迁移模式）──
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'episodes' AND column_name = 'tenant_id') THEN
        ALTER TABLE episodes ADD COLUMN tenant_id uuid;
    END IF;
END $$;

UPDATE episodes
SET tenant_id = '00000000-0000-0000-0000-000000000001'
WHERE tenant_id IS NULL;

ALTER TABLE episodes
    ALTER COLUMN tenant_id SET NOT NULL,
    ADD CONSTRAINT fk_episodes_tenant
        FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_episodes_tenant
    ON episodes (tenant_id);
