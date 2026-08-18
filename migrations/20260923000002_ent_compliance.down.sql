-- Enterprise Compliance (Policies & Marketplace): down

DROP INDEX IF EXISTS idx_audit_logs_user_time;
DROP INDEX IF EXISTS idx_audit_logs_tenant_time;

DROP TABLE IF EXISTS ent_catalog_installs;
DROP TABLE IF EXISTS ent_catalog_items;
DROP TABLE IF EXISTS ent_model_policies;
DROP TABLE IF EXISTS ent_tenant_policies;
