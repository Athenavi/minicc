-- 短信验证码登录（三方登录设计 P4 收尾）
-- Date: 2026-08-21
-- 依赖: 20260821000001_oauth_login.up.sql（users.phone + uq_users_tenant_phone）

CREATE TABLE IF NOT EXISTS ent_sms_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    provider VARCHAR(32) NOT NULL DEFAULT 'aliyun',   -- aliyun/tencent/custom
    sign_name VARCHAR(64) NOT NULL DEFAULT '',        -- 短信签名
    template_id VARCHAR(64) NOT NULL DEFAULT '',      -- 模板 ID（阿里云 TemplateCode / 腾讯云 TemplateId）
    access_key_id VARCHAR(256) NOT NULL DEFAULT '',   -- 明文存储（与 client_id 同级敏感度）
    secret_enc TEXT NOT NULL DEFAULT '',              -- AccessKeySecret（AES-GCM 加密）
    endpoint VARCHAR(512),                            -- 服务商端点覆盖（测试/代理；custom 必填）
    code_ttl_seconds INT NOT NULL DEFAULT 300,        -- 验证码有效期
    send_interval_seconds INT NOT NULL DEFAULT 60,    -- 同一手机号发送冷却
    daily_limit INT NOT NULL DEFAULT 10,              -- 同一手机号每日发送上限
    login_enabled BOOLEAN NOT NULL DEFAULT FALSE,     -- 短信登录入口开关
    auto_register BOOLEAN NOT NULL DEFAULT FALSE,     -- 未注册手机号验证码登录时自动建号
    enabled BOOLEAN NOT NULL DEFAULT FALSE,           -- 发送能力总开关（绑定手机号也依赖）
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(tenant_id)
);

COMMENT ON TABLE ent_sms_config IS 'Per-tenant SMS verification code configuration (secret_enc encrypted at rest)';
