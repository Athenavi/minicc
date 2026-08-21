-- 三方登录扩展（OAuth2 协议支持 + 手机号 + 人机验证配置）
-- Date: 2026-08-21
-- 依赖: 20260922000002_ent_sso.up.sql（ent_oidc_providers / ent_user_identities）

-- ── 1. ent_oidc_providers 扩展：OAuth2 协议 + 预设模板 ──
ALTER TABLE ent_oidc_providers
  ADD COLUMN IF NOT EXISTS protocol      VARCHAR(16)  NOT NULL DEFAULT 'oidc',   -- 'oidc' | 'oauth2'
  ADD COLUMN IF NOT EXISTS provider_type VARCHAR(32)  NOT NULL DEFAULT 'custom', -- google/github/wechat/dingtalk/feishu/qq/custom
  ADD COLUMN IF NOT EXISTS display_name  VARCHAR(64),
  ADD COLUMN IF NOT EXISTS icon          VARCHAR(64),
  ADD COLUMN IF NOT EXISTS sort_order    INT         NOT NULL DEFAULT 100,
  ADD COLUMN IF NOT EXISTS auth_url      VARCHAR(512),   -- oauth2 端点覆盖（模板缺省自动填充）
  ADD COLUMN IF NOT EXISTS token_url     VARCHAR(512),
  ADD COLUMN IF NOT EXISTS userinfo_url  VARCHAR(512),
  ADD COLUMN IF NOT EXISTS extra         JSONB       NOT NULL DEFAULT '{}';     -- provider 特有项（微信 mode、unionid 策略等）

-- issuer 对 oauth2 协议非必填，放宽 NOT NULL 约束
ALTER TABLE ent_oidc_providers ALTER COLUMN issuer DROP NOT NULL;

-- ── 2. users 扩展：手机号 + 密码可用性标识 ──
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS phone        VARCHAR(32),
  ADD COLUMN IF NOT EXISTS password_set BOOLEAN NOT NULL DEFAULT TRUE;  -- SSO 自动建号写 FALSE（随机 bcrypt 不可用于口令登录）

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_tenant_phone ON users(tenant_id, phone) WHERE phone IS NOT NULL;

-- ── 3. 人机验证配置（防接口滥用）──
CREATE TABLE IF NOT EXISTS ent_captcha_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    provider VARCHAR(32) NOT NULL DEFAULT 'turnstile',  -- turnstile/recaptcha/hcaptcha/tencent/custom
    site_key VARCHAR(256) NOT NULL DEFAULT '',
    secret_enc TEXT NOT NULL DEFAULT '',                -- AES-GCM 加密后的验证密钥
    verify_url VARCHAR(512),                            -- custom 协议的验证端点
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(tenant_id)
);

COMMENT ON TABLE ent_captcha_config IS 'Per-tenant human verification (CAPTCHA) configuration (secret_enc encrypted at rest)';
