-- Enterprise Cost Center (Quota): down

DROP TABLE IF EXISTS ent_quota_allocations;
DROP TABLE IF EXISTS ent_quota_pools;

DROP INDEX IF EXISTS idx_billing_records_group;
ALTER TABLE billing_records DROP COLUMN IF EXISTS group_id;
