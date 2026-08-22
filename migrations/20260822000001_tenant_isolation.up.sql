-- 20260822000001_tenant_isolation: up
-- 目的：为 agent_sessions 与 uploads 表补齐 tenant_id 列，闭合多租户隔离的最后一块拼图。
-- 设计：
--   1. 用 DO 块幂等检测列是否已存在，避免重复 ALTER 报错（适配已部分加列的库）。
--   2. 新列先 NULL，再用 default tenant UUID 回填历史行（兼容老库）。
--   3. 最后 ALTER SET NOT NULL + 建索引 + 外键。
-- 依赖：tenants 表与默认租户 00000000-0000-0000-0000-000000000001 由 seed.go EnsureDefaultTenant 幂等播种。

-- ── agent_sessions ────────────────────────────────────────────
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'agent_sessions' AND column_name = 'tenant_id') THEN
        ALTER TABLE agent_sessions ADD COLUMN tenant_id uuid;
    END IF;
END $$;

UPDATE agent_sessions
SET tenant_id = '00000000-0000-0000-0000-000000000001'
WHERE tenant_id IS NULL;

ALTER TABLE agent_sessions
    ALTER COLUMN tenant_id SET NOT NULL,
    ADD CONSTRAINT fk_agent_sessions_tenant
        FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_agent_sessions_tenant
    ON agent_sessions (tenant_id);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_tenant_user
    ON agent_sessions (tenant_id, user_id);

-- ── uploads ───────────────────────────────────────────────────
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'uploads' AND column_name = 'tenant_id') THEN
        ALTER TABLE uploads ADD COLUMN tenant_id uuid;
    END IF;
END $$;

UPDATE uploads
SET tenant_id = '00000000-0000-0000-0000-000000000001'
WHERE tenant_id IS NULL;

ALTER TABLE uploads
    ALTER COLUMN tenant_id SET NOT NULL,
    ADD CONSTRAINT fk_uploads_tenant
        FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_uploads_tenant
    ON uploads (tenant_id);

CREATE INDEX IF NOT EXISTS idx_uploads_tenant_user
    ON uploads (tenant_id, user_id);
