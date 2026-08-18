-- Enterprise Compliance (Policies & Marketplace) Migration
-- Version: 1.0
-- Date: 2026-09-23
-- Description: 企业合规：租户安全策略、模型访问策略、插件/技能市场目录，及审计查询索引
-- 注意: 外键锚定真实表 tenants(id)，不引用影子表 admin_tenants
-- 兼容性: UNIQUE ... NULLS NOT DISTINCT 需 PostgreSQL >= 15（项目基准镜像 pg16，满足）

-- ── 1. 租户安全策略表 ────────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_tenant_policies (
    tenant_id UUID PRIMARY KEY REFERENCES tenants(id),
    privacy_mode BOOLEAN DEFAULT FALSE,
    data_retention_days INT DEFAULT 0,  -- 0 = 不主动清理
    training_allowed BOOLEAN DEFAULT TRUE,
    redaction_rules JSONB DEFAULT '{}',
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

COMMENT ON TABLE ent_tenant_policies IS 'Per-tenant compliance policies (privacy, retention, redaction)';

-- ── 2. 模型访问策略表 ────────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_model_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    role_id UUID REFERENCES ent_roles(id) ON DELETE SET NULL,
    allowed_models TEXT[] NOT NULL DEFAULT '{}',
    per_model_limits JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    -- 每租户每角色仅一条策略；role_id 为 NULL 表示租户级兜底策略，同样要求唯一
    UNIQUE NULLS NOT DISTINCT (tenant_id, role_id)
);

COMMENT ON TABLE ent_model_policies IS 'Model allow-list and per-model limits by tenant and role';

-- ── 3. 市场目录条目表 ────────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_catalog_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(8) NOT NULL,  -- plugin/skill
    name VARCHAR(128) NOT NULL,
    version VARCHAR(32) DEFAULT '1.0.0',
    manifest JSONB DEFAULT '{}',
    status VARCHAR(16) NOT NULL DEFAULT 'draft',  -- draft/published/retired
    created_by UUID,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT chk_catalog_item_type CHECK (type IN ('plugin','skill')),
    CONSTRAINT chk_catalog_item_status CHECK (status IN ('draft','published','retired'))
);

COMMENT ON TABLE ent_catalog_items IS 'Marketplace catalog entries (plugins and skills)';

-- ── 4. 市场安装记录表 ────────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_catalog_installs (
    item_id UUID NOT NULL REFERENCES ent_catalog_items(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    enabled BOOLEAN DEFAULT TRUE,
    installed_at TIMESTAMPTZ DEFAULT NOW(),

    PRIMARY KEY (item_id, tenant_id)
);

COMMENT ON TABLE ent_catalog_installs IS 'Which catalog items are installed per tenant';

-- ── 5. 审计日志查询索引 ──────────────────────────────────

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_time ON audit_logs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_time ON audit_logs(user_id, created_at);
