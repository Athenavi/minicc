-- Enterprise SSO (OIDC) Migration
-- Version: 1.0
-- Date: 2026-09-22
-- Description: 企业级单点登录：OIDC 提供商配置与用户外部身份绑定
-- 注意: 外键锚定真实表 tenants(id) / users(id)，不引用影子表 admin_tenants

-- ── 1. OIDC 提供商配置表 ─────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_oidc_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(64) NOT NULL,
    issuer VARCHAR(512) NOT NULL,
    client_id VARCHAR(256) NOT NULL,
    client_secret_enc TEXT NOT NULL,
    scopes TEXT[] DEFAULT '{openid,email,profile}',
    enabled BOOLEAN DEFAULT TRUE,
    auto_provision BOOLEAN DEFAULT TRUE,
    role_mapping JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(tenant_id, name)
);

COMMENT ON TABLE ent_oidc_providers IS 'Per-tenant OIDC identity providers (client_secret_enc is encrypted at rest)';

-- ── 2. 用户外部身份绑定表 ────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_user_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES ent_oidc_providers(id) ON DELETE CASCADE,
    subject VARCHAR(256) NOT NULL,
    email VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(provider_id, subject)
);

CREATE INDEX IF NOT EXISTS idx_ent_user_identities_user ON ent_user_identities(user_id);

COMMENT ON TABLE ent_user_identities IS 'External identity bindings (provider subject -> local user)';
