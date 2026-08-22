-- 20260822000002_security_hardening: down
DROP INDEX IF EXISTS idx_episodes_tenant;
ALTER TABLE episodes DROP CONSTRAINT IF EXISTS fk_episodes_tenant;
ALTER TABLE episodes DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_sessions_user_updated;
DROP INDEX IF EXISTS idx_tool_calls_created;
DROP INDEX IF EXISTS idx_tool_calls_session_created;
DROP INDEX IF EXISTS idx_media_assets_name_tsv;
DROP INDEX IF EXISTS idx_messages_content_tsv;

ALTER TABLE api_keys DROP COLUMN IF EXISTS revoked;
