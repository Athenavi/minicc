-- 回滚三方登录扩展迁移

DROP TABLE IF EXISTS ent_captcha_config;
DROP INDEX IF EXISTS uq_users_tenant_phone;

ALTER TABLE users
  DROP COLUMN IF EXISTS phone,
  DROP COLUMN IF EXISTS password_set;

ALTER TABLE ent_oidc_providers
  DROP COLUMN IF EXISTS protocol,
  DROP COLUMN IF EXISTS provider_type,
  DROP COLUMN IF EXISTS display_name,
  DROP COLUMN IF EXISTS icon,
  DROP COLUMN IF EXISTS sort_order,
  DROP COLUMN IF EXISTS auth_url,
  DROP COLUMN IF EXISTS token_url,
  DROP COLUMN IF EXISTS userinfo_url,
  DROP COLUMN IF EXISTS extra;

-- 恢复 issuer NOT NULL（oidc 协议必填；存量 oauth2 行可能为空串，回滚仅约束语义恢复）
ALTER TABLE ent_oidc_providers ALTER COLUMN issuer SET NOT NULL;
