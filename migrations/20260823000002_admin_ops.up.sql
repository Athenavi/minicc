-- 20260823000002_admin_ops: up
-- /admin 全栈实装所需表：租户状态、域名管理、模型注册、定时任务

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS status varchar(16) NOT NULL DEFAULT 'active';

CREATE TABLE IF NOT EXISTS domains
(
    id         uuid                     default gen_random_uuid() not null
        primary key,
    tenant_id  uuid                                               not null
        references tenants (id) on delete cascade,
    domain     varchar(255)                                       not null unique,
    ssl_status varchar(16)             default 'none'::varchar    not null,
    verified   boolean                  default false             not null,
    created_at timestamp with time zone default now()             not null,
    updated_at timestamp with time zone default now()             not null
);

CREATE TABLE IF NOT EXISTS llm_models
(
    id             uuid                     default gen_random_uuid() not null
        primary key,
    provider       varchar(32)                                        not null,
    name           varchar(128)                                       not null,
    display_name   varchar(128)             default ''::varchar       not null,
    enabled        boolean                  default true              not null,
    context_window integer                  default 8192              not null,
    created_at     timestamp with time zone default now()             not null,
    updated_at     timestamp with time zone default now()             not null,
    unique (provider, name)
);

CREATE TABLE IF NOT EXISTS cron_jobs
(
    id          uuid                     default gen_random_uuid() not null
        primary key,
    name        varchar(128)                                       not null,
    schedule    varchar(64)                                        not null,
    task        varchar(255)                                       not null,
    enabled     boolean                  default true              not null,
    last_run_at timestamp with time zone,
    last_status varchar(16)              default 'pending'::varchar not null,
    created_at  timestamp with time zone default now()             not null,
    updated_at  timestamp with time zone default now()             not null
);
