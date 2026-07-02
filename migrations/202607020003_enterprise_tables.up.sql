-- MiniCC V2.0: Enterprise tables — Collaboration, Brain, Support

-- 1. 协作任务表
CREATE TABLE IF NOT EXISTS enterprise_tasks (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    title VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    project VARCHAR(128) DEFAULT '',
    assignee VARCHAR(128) DEFAULT '',
    priority VARCHAR(16) NOT NULL DEFAULT 'medium',  -- low/medium/high/urgent
    status VARCHAR(16) NOT NULL DEFAULT 'open',       -- open/in_progress/done/cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Wiki 页面表
CREATE TABLE IF NOT EXISTS wiki_pages (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    tags TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. OKR 表
CREATE TABLE IF NOT EXISTS okrs (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    objective VARCHAR(255) NOT NULL,
    key_results JSONB DEFAULT '[]',  -- [{title, target, current, weight}]
    quarter VARCHAR(16) DEFAULT '',  -- e.g. "2026Q3"
    status VARCHAR(16) NOT NULL DEFAULT 'active',  -- active/completed/cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 4. 会议记录表
CREATE TABLE IF NOT EXISTS meeting_notes (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    title VARCHAR(255) NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    summary TEXT DEFAULT '',
    participants TEXT[] DEFAULT '{}',
    date DATE DEFAULT CURRENT_DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 5. 客服工单表
CREATE TABLE IF NOT EXISTS support_tickets (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    subject VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    priority VARCHAR(16) NOT NULL DEFAULT 'medium',
    status VARCHAR(16) NOT NULL DEFAULT 'open', -- open/in_progress/resolved/closed
    assignee VARCHAR(128) DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 6. 知识库文章表
CREATE TABLE IF NOT EXISTS kb_articles (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    tags TEXT[] DEFAULT '{}',
    category VARCHAR(64) DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 7. 营销活动表
CREATE TABLE IF NOT EXISTS marketing_campaigns (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(32) NOT NULL DEFAULT '',
    name VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    campaign_type VARCHAR(32) NOT NULL DEFAULT 'email',  -- email/social/abtest
    config JSONB DEFAULT '{}',
    status VARCHAR(16) NOT NULL DEFAULT 'draft',  -- draft/running/completed
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
