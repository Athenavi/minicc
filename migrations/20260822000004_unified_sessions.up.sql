-- 20260822000004_unified_sessions: up
-- 统一任务会话落库（与 python-engine app/db.py ensure_tables 保持一致，双路径幂等）
CREATE TABLE IF NOT EXISTS unified_sessions
(
    id            text                     PRIMARY KEY,
    tenant_id     text                     NOT NULL,
    user_id       text                     NOT NULL,
    title         text                     DEFAULT '' NOT NULL,
    mode          text                     DEFAULT 'auto' NOT NULL,
    shared_context jsonb                   DEFAULT '{}'::jsonb NOT NULL,
    created_at    timestamptz              DEFAULT now() NOT NULL,
    updated_at    timestamptz              DEFAULT now() NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_unified_sessions_user_updated
    ON unified_sessions (tenant_id, user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS unified_messages
(
    id         bigserial                PRIMARY KEY,
    session_id text                     NOT NULL REFERENCES unified_sessions (id) ON DELETE CASCADE,
    role       text                     NOT NULL,
    content    text                     NOT NULL,
    metadata   jsonb                    DEFAULT '{}'::jsonb NOT NULL,
    error      text                     DEFAULT '' NOT NULL,
    created_at timestamptz              DEFAULT now() NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_unified_messages_session
    ON unified_messages (session_id, created_at);
