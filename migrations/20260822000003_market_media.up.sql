-- 20260822000003_market_media: up
-- 目的：
--   1. agents 用户级隔离（严格私有，预留 visibility 扩展）
--   2. 市场目录类型扩展：plugin/skill → + agent/mcp

-- ── 1. agents 用户级隔离 ──
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'agents' AND column_name = 'user_id') THEN
        ALTER TABLE agents ADD COLUMN user_id uuid;
    END IF;
END $$;

-- 存量行归属系统首个 owner 用户（单租户迁移语义）
UPDATE agents
SET user_id = (SELECT id FROM users WHERE role = 'owner' ORDER BY created_at LIMIT 1)
WHERE user_id IS NULL;

ALTER TABLE agents
    ALTER COLUMN user_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_agents_tenant_user
    ON agents (tenant_id, user_id);

-- visibility: private（默认）/ tenant / public —— 预留未来共享/公开市场
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'agents' AND column_name = 'visibility') THEN
        ALTER TABLE agents ADD COLUMN visibility varchar(16) NOT NULL DEFAULT 'private';
    END IF;
END $$;

-- ── 2. 市场目录类型扩展 ──
ALTER TABLE ent_catalog_items DROP CONSTRAINT IF EXISTS chk_catalog_item_type;
ALTER TABLE ent_catalog_items ADD CONSTRAINT chk_catalog_item_type
    CHECK (type IN ('plugin', 'skill', 'agent', 'mcp'));
