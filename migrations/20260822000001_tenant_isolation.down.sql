-- 20260822000001_tenant_isolation: down
-- 回滚：先 DROP 约束与索引，再 DROP 列。
-- 注意：回滚会导致多租户隔离失效，仅在开发/紧急回滚时使用。

-- ── uploads ───────────────────────────────────────────────────
DROP INDEX IF EXISTS idx_uploads_tenant_user;
DROP INDEX IF EXISTS idx_uploads_tenant;
ALTER TABLE uploads DROP CONSTRAINT IF EXISTS fk_uploads_tenant;
ALTER TABLE uploads DROP COLUMN IF EXISTS tenant_id;

-- ── agent_sessions ────────────────────────────────────────────
DROP INDEX IF EXISTS idx_agent_sessions_tenant_user;
DROP INDEX IF EXISTS idx_agent_sessions_tenant;
ALTER TABLE agent_sessions DROP CONSTRAINT IF EXISTS fk_agent_sessions_tenant;
ALTER TABLE agent_sessions DROP COLUMN IF EXISTS tenant_id;
