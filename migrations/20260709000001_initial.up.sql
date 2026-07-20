-- ============================================================
-- MiniCC V2.0 企业版数据库初始化
-- 基于 docs/enterprise/ 架构设计
--   - data-layer.md: 多租户 + UUID 主键 + RLS
--   - auth-security.md: 认证授权与安全通信
--   - agent-system.md: Agent 定义与编排
--   - stategraph-engine.md: 工作流引擎
--   - api-layer.md: API 层路由设计
-- ============================================================

-- ============================================================
-- 1. 租户体系（data-layer.md §2.2）
-- ============================================================

CREATE TABLE tenants (
    id VARCHAR(32) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 插入默认租户（单租户模式使用）
INSERT INTO tenants (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'Default Tenant')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 2. 用户体系（auth-security.md §1 + data-layer.md §2.2）
-- ============================================================

CREATE TABLE users (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    name VARCHAR(128) NOT NULL,
    password_hash VARCHAR(255) NOT NULL DEFAULT '',
    role VARCHAR(16) NOT NULL DEFAULT 'user',       -- owner / admin / developer / user / readonly
    storage_id VARCHAR(64) UNIQUE,
    credits INTEGER NOT NULL DEFAULT 1000,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, email)
);
CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_users_email ON users(email);

-- API Keys 表（auth-security.md §1.1）
CREATE TABLE api_keys (
    id VARCHAR(32) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    key_hash VARCHAR(64) NOT NULL,
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_api_keys_user ON api_keys(user_id);
CREATE UNIQUE INDEX idx_api_keys_hash ON api_keys(key_hash);

-- 访客存储映射表（匿名用户存储隔离）
CREATE TABLE guest_storage (
    client_id VARCHAR(64) PRIMARY KEY,
    storage_id VARCHAR(64) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_guest_storage_id ON guest_storage(storage_id);

-- ============================================================
-- 3. Agent 定义与编排（agent-system.md §2）
-- ============================================================

CREATE TABLE agents (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    system_prompt TEXT,
    tools JSONB DEFAULT '[]',
    llm_config JSONB DEFAULT '{}',
    max_turns INT DEFAULT 10,
    timeout_seconds INT DEFAULT 120,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_agents_tenant ON agents(tenant_id);
CREATE INDEX idx_agents_name ON agents(tenant_id, name);

-- Agent 注册表（运行时 agent 类型注册）
CREATE TABLE agent_registry (
    agent_type VARCHAR(32) PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    config JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Agent 执行会话表（agent-system.md §4）
CREATE TABLE agent_sessions (
    id VARCHAR(128) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    agent_id VARCHAR(32) REFERENCES agents(id) ON DELETE SET NULL,
    name VARCHAR(128) NOT NULL,
    task TEXT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',   -- pending / running / completed / failed
    result TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_agent_sessions_user ON agent_sessions(user_id);
CREATE INDEX idx_agent_sessions_status ON agent_sessions(status);

-- Agent 执行记录表
CREATE TABLE episodes (
    id VARCHAR(32) PRIMARY KEY,
    task TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    tools_used TEXT[] NOT NULL DEFAULT '{}',
    success BOOLEAN NOT NULL DEFAULT TRUE,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- 4. 会话与消息（data-layer.md §2.2）
-- ============================================================

CREATE TABLE sessions (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) REFERENCES users(id) ON DELETE SET NULL,
    agent_id VARCHAR(32) REFERENCES agents(id) ON DELETE SET NULL,
    title VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'active',    -- active / archived / deleted
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sessions_tenant ON sessions(tenant_id);
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_agent ON sessions(agent_id);
CREATE INDEX idx_sessions_updated ON sessions(updated_at);

CREATE TABLE messages (
    id VARCHAR(32) PRIMARY KEY,
    session_id VARCHAR(32) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role VARCHAR(16) NOT NULL,                       -- user / assistant / system / tool
    content TEXT NOT NULL DEFAULT '',
    tool_calls JSONB,                                -- tool call requests/responses inline
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_messages_session ON messages(session_id, created_at);

-- 工具调用详细记录表（补充 messages.tool_calls 的详细记录）
CREATE TABLE tool_calls (
    id VARCHAR(32) PRIMARY KEY,
    session_id VARCHAR(32) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    message_id VARCHAR(32) REFERENCES messages(id) ON DELETE SET NULL,
    tool_name VARCHAR(128) NOT NULL,
    input JSONB NOT NULL DEFAULT '{}',
    output TEXT NOT NULL DEFAULT '',
    is_error BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_tool_calls_session ON tool_calls(session_id);

-- ============================================================
-- 5. 工作流引擎（stategraph-engine.md）
-- ============================================================

CREATE TABLE workflow_graphs (
    id VARCHAR(32) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    user_id VARCHAR(32) REFERENCES users(id) ON DELETE SET NULL,
    graph_json JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_workflow_graphs_user ON workflow_graphs(user_id);
CREATE INDEX idx_workflow_graphs_updated ON workflow_graphs(updated_at);

-- ============================================================
-- 6. 任务队列（message-queue.md §3）
-- ============================================================

CREATE TABLE tasks (
    id VARCHAR(32) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(32) NOT NULL,                       -- llm / tool / batch / index / email
    status VARCHAR(16) NOT NULL DEFAULT 'pending',   -- pending / running / completed / failed
    priority INT NOT NULL DEFAULT 0,
    payload JSONB NOT NULL DEFAULT '{}',
    result JSONB,
    error TEXT,
    retries INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_user ON tasks(user_id);
CREATE INDEX idx_tasks_type ON tasks(type);

-- ============================================================
-- 7. 计费与支付（data-layer.md §2.2 billing_records）
-- ============================================================

CREATE TABLE billing_records (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id VARCHAR(32) REFERENCES sessions(id) ON DELETE SET NULL,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cost_cents INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_billing_tenant ON billing_records(tenant_id);
CREATE INDEX idx_billing_user ON billing_records(user_id);
CREATE INDEX idx_billing_session ON billing_records(session_id);
CREATE INDEX idx_billing_created ON billing_records(created_at);

-- Stripe 支付记录表
CREATE TABLE stripe_payments (
    session_id VARCHAR(128) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL,
    credits INTEGER NOT NULL DEFAULT 1000,
    amount_cents BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',   -- pending / completed / failed
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX idx_stripe_payments_user ON stripe_payments(user_id);

-- 积分交易记录表
CREATE TABLE credit_transactions (
    id VARCHAR(32) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount INTEGER NOT NULL,
    balance INTEGER NOT NULL,
    reason VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_credit_tx_user ON credit_transactions(user_id, created_at DESC);

-- ============================================================
-- 8. 媒体资产（media_assets）
-- ============================================================

CREATE TABLE media_assets (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) NOT NULL,
    type VARCHAR(16) NOT NULL DEFAULT 'text',        -- image / text / document / code / audio / video
    name VARCHAR(255) NOT NULL,
    file_url VARCHAR(1024) DEFAULT '',
    file_path VARCHAR(512) DEFAULT '',
    mime_type VARCHAR(64) DEFAULT '',
    thumbnail VARCHAR(512) DEFAULT '',
    metadata JSONB DEFAULT '{}',
    tags TEXT[] DEFAULT '{}',
    category VARCHAR(64) DEFAULT '',
    size BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_media_tenant ON media_assets(tenant_id);
CREATE INDEX idx_media_user ON media_assets(user_id);
CREATE INDEX idx_media_type ON media_assets(type);
CREATE INDEX idx_media_category ON media_assets(category);
CREATE INDEX idx_media_created ON media_assets(created_at);

-- ============================================================
-- 9. 企业业务表（enterprise features）
-- ============================================================

-- 协作任务表
CREATE TABLE enterprise_tasks (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    title VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    project VARCHAR(128) DEFAULT '',
    assignee VARCHAR(128) DEFAULT '',
    priority VARCHAR(16) NOT NULL DEFAULT 'medium',  -- low / medium / high / urgent
    status VARCHAR(16) NOT NULL DEFAULT 'open',      -- open / in_progress / done / cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_enterprise_tasks_tenant ON enterprise_tasks(tenant_id);

-- Wiki 页面表
CREATE TABLE wiki_pages (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    tags TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_wiki_pages_tenant ON wiki_pages(tenant_id);

-- OKR 表
CREATE TABLE okrs (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    objective VARCHAR(255) NOT NULL,
    key_results JSONB DEFAULT '[]',                  -- [{title, target, current, weight}]
    quarter VARCHAR(16) DEFAULT '',                  -- e.g. "2026Q3"
    status VARCHAR(16) NOT NULL DEFAULT 'active',    -- active / completed / cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_okrs_tenant ON okrs(tenant_id);

-- 会议记录表
CREATE TABLE meeting_notes (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    title VARCHAR(255) NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    summary TEXT DEFAULT '',
    participants TEXT[] DEFAULT '{}',
    date DATE DEFAULT CURRENT_DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_meeting_notes_tenant ON meeting_notes(tenant_id);

-- 客服工单表
CREATE TABLE support_tickets (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    subject VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    priority VARCHAR(16) NOT NULL DEFAULT 'medium',
    status VARCHAR(16) NOT NULL DEFAULT 'open',      -- open / in_progress / resolved / closed
    assignee VARCHAR(128) DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_support_tickets_tenant ON support_tickets(tenant_id);

-- 知识库文章表
CREATE TABLE kb_articles (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    tags TEXT[] DEFAULT '{}',
    category VARCHAR(64) DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_kb_articles_tenant ON kb_articles(tenant_id);

-- 营销活动表
CREATE TABLE marketing_campaigns (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    name VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    campaign_type VARCHAR(32) NOT NULL DEFAULT 'email', -- email / social / abtest
    config JSONB DEFAULT '{}',
    status VARCHAR(16) NOT NULL DEFAULT 'draft',     -- draft / running / completed
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_marketing_campaigns_tenant ON marketing_campaigns(tenant_id);

-- ============================================================
-- 10. 审计日志（audit_logs）
-- ============================================================

CREATE TABLE audit_logs (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(64) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    resource_id VARCHAR(64),
    details JSONB DEFAULT '{}',
    ip_address VARCHAR(45),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_tenant ON audit_logs(tenant_id);
CREATE INDEX idx_audit_user ON audit_logs(user_id);
CREATE INDEX idx_audit_action ON audit_logs(action);
CREATE INDEX idx_audit_created ON audit_logs(created_at);

-- ============================================================
-- 11. 行级安全策略（data-layer.md §2.3）
-- ============================================================

-- 启用行级安全
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE media_assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE enterprise_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE wiki_pages ENABLE ROW LEVEL SECURITY;
ALTER TABLE okrs ENABLE ROW LEVEL SECURITY;
ALTER TABLE meeting_notes ENABLE ROW LEVEL SECURITY;
ALTER TABLE support_tickets ENABLE ROW LEVEL SECURITY;
ALTER TABLE kb_articles ENABLE ROW LEVEL SECURITY;
ALTER TABLE marketing_campaigns ENABLE ROW LEVEL SECURITY;

-- 租户隔离策略
CREATE POLICY tenant_isolation ON users
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON sessions
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON messages
    USING (session_id IN (
        SELECT id FROM sessions
        WHERE tenant_id = current_setting('app.current_tenant_id')::VARCHAR
    ));

CREATE POLICY tenant_isolation ON agents
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON billing_records
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON audit_logs
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON media_assets
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON enterprise_tasks
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON wiki_pages
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON okrs
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON meeting_notes
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON support_tickets
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON kb_articles
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

CREATE POLICY tenant_isolation ON marketing_campaigns
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);
