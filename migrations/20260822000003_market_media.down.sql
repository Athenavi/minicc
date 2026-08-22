-- 20260822000003_market_media: down
ALTER TABLE ent_catalog_items DROP CONSTRAINT IF EXISTS chk_catalog_item_type;
ALTER TABLE ent_catalog_items ADD CONSTRAINT chk_catalog_item_type
    CHECK (type IN ('plugin', 'skill'));

DROP INDEX IF EXISTS idx_agents_tenant_user;
ALTER TABLE agents DROP COLUMN IF EXISTS visibility;
ALTER TABLE agents DROP COLUMN IF EXISTS user_id;
