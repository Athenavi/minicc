-- Enterprise SSO (OIDC): down
-- 按依赖反序回滚（绑定表先删，提供商表后删；索引随表一并移除）

DROP TABLE IF EXISTS ent_user_identities;
DROP TABLE IF EXISTS ent_oidc_providers;
