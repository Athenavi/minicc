-- 20260824000001_system_settings: up
-- 后台「系统设置」运行时配置存储（键值型，业务配置经 DB 持久化，env 仅作默认值）

CREATE TABLE IF NOT EXISTS system_settings
(
    id         bigserial                                         not null
        primary key,
    category   varchar(32)                                       not null, -- 配置分组：agent / llm / rate_limit / storage / payment / security
    key        varchar(64)                                       not null, -- 配置键，如 max_turns / default_model / rpm
    value      jsonb                                             not null, -- JSON 编码的值（标量或对象）
    updated_at timestamp with time zone default now()            not null,
    updated_by uuid,                                                      -- 最后修改人（可空）
    unique (category, key)
);

CREATE INDEX IF NOT EXISTS idx_system_settings_category ON system_settings (category);