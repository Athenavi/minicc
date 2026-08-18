-- Enterprise Cost Center (Quota) Migration
-- Version: 1.0
-- Date: 2026-09-23
-- Description: 企业成本中心：配额池、配额分配，以及 billing_records 按用户组归集
-- 注意: 外键锚定真实表 tenants(id)，不引用影子表 admin_tenants

-- ── 1. 配额池表 ──────────────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_quota_pools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    resource_type VARCHAR(20) NOT NULL,  -- token/storage_mb/concurrency/credits
    total_amount BIGINT NOT NULL DEFAULT 0,  -- 0 = 无限制
    period VARCHAR(10) NOT NULL DEFAULT 'monthly',  -- daily/monthly
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(tenant_id, resource_type, period),
    CONSTRAINT chk_quota_pool_resource CHECK (resource_type IN ('token','storage_mb','concurrency','credits')),
    CONSTRAINT chk_quota_pool_amount CHECK (total_amount >= 0),
    CONSTRAINT chk_quota_pool_period CHECK (period IN ('daily','monthly'))
);

COMMENT ON TABLE ent_quota_pools IS 'Per-tenant quota pools by resource type and reset period';

-- ── 2. 配额分配表 ────────────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_quota_allocations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pool_id UUID NOT NULL REFERENCES ent_quota_pools(id) ON DELETE CASCADE,
    target_type VARCHAR(10) NOT NULL,  -- group/user
    target_id UUID NOT NULL,  -- ent_groups.id 或 users.id（多态引用，不加外键）
    amount BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(pool_id, target_type, target_id),
    CONSTRAINT chk_quota_alloc_target CHECK (target_type IN ('group','user')),
    CONSTRAINT chk_quota_alloc_amount CHECK (amount >= 0)
);

COMMENT ON TABLE ent_quota_allocations IS 'Quota allocations from a pool to a group or user';

-- ── 3. billing_records 增加用户组归集字段 ────────────────

ALTER TABLE billing_records ADD COLUMN IF NOT EXISTS group_id UUID;

CREATE INDEX IF NOT EXISTS idx_billing_records_group ON billing_records(group_id);
