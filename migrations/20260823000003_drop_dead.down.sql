-- 20260823000003_drop_dead: down
-- 还原被清理的死表（仅供回滚参考）
CREATE TABLE IF NOT EXISTS episodes
(
    id          uuid                     default gen_random_uuid() not null
        primary key,
    task        text                     default ''::text          not null,
    summary     text                     default ''::text          not null,
    tools_used  text[]                   default '{}'::text[]      not null,
    success     boolean                  default true              not null,
    duration_ms bigint                   default 0                 not null,
    created_at  timestamp with time zone default now()             not null
);
