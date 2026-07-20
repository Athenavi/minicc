-- Knowledge Base System Migration
-- Version: 20260709000001

-- 1. 知识库表
CREATE TABLE IF NOT EXISTS knowledge_bases (
    id VARCHAR(32) PRIMARY KEY,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(32) NOT NULL DEFAULT 'rag',  -- 'wiki' | 'rag'
    visibility VARCHAR(32) NOT NULL DEFAULT 'private',  -- 'public' | 'private'
    status VARCHAR(32) NOT NULL DEFAULT 'active',  -- 'active' | 'building' | 'error'
    document_count INT DEFAULT 0,
    total_size_bytes BIGINT DEFAULT 0,
    credits_consumed INT DEFAULT 0,
    config JSONB DEFAULT '{}',  -- 构建配置
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_tenant ON knowledge_bases(tenant_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_user ON knowledge_bases(user_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_visibility ON knowledge_bases(visibility);
CREATE INDEX IF NOT EXISTS idx_knowledge_bases_status ON knowledge_bases(status);

-- RLS 策略
ALTER TABLE knowledge_bases ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON knowledge_bases
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

-- 2. 文档表
CREATE TABLE IF NOT EXISTS knowledge_documents (
    id VARCHAR(32) PRIMARY KEY,
    knowledge_base_id VARCHAR(32) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id VARCHAR(32) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    file_url VARCHAR(1024),
    file_type VARCHAR(32),  -- 'pdf' | 'txt' | 'md' | 'csv' | 'docx'
    file_size_bytes BIGINT DEFAULT 0,
    chunk_count INT DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',  -- 'pending' | 'processing' | 'completed' | 'error'
    error_message TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_knowledge_documents_kb ON knowledge_documents(knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_documents_tenant ON knowledge_documents(tenant_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_documents_status ON knowledge_documents(status);

-- RLS 策略
ALTER TABLE knowledge_documents ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON knowledge_documents
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

-- 3. 文档分块表（Wiki 模式）
CREATE TABLE IF NOT EXISTS knowledge_chunks (
    id VARCHAR(32) PRIMARY KEY,
    document_id VARCHAR(32) NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
    knowledge_base_id VARCHAR(32) NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    tenant_id VARCHAR(32) NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    chunk_index INT NOT NULL,
    content TEXT NOT NULL,
    metadata JSONB DEFAULT '{}',
    -- 全文搜索向量
    search_vector TSVECTOR,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_doc ON knowledge_chunks(document_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_kb ON knowledge_chunks(knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_tenant ON knowledge_chunks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_search ON knowledge_chunks USING GIN(search_vector);

-- RLS 策略
ALTER TABLE knowledge_chunks ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON knowledge_chunks
    USING (tenant_id = current_setting('app.current_tenant_id')::VARCHAR);

-- 4. 全文搜索触发器（Wiki 模式自动更新 search_vector）
CREATE OR REPLACE FUNCTION knowledge_chunks_search_vector_update() RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector := to_tsvector('chinese', COALESCE(NEW.content, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER knowledge_chunks_search_vector_trigger
    BEFORE INSERT OR UPDATE ON knowledge_chunks
    FOR EACH ROW
    EXECUTE FUNCTION knowledge_chunks_search_vector_update();

-- 5. 更新 updated_at 触发器
CREATE OR REPLACE FUNCTION update_updated_at_column() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_knowledge_bases_updated_at
    BEFORE UPDATE ON knowledge_bases
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_knowledge_documents_updated_at
    BEFORE UPDATE ON knowledge_documents
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- 6. 插入默认公共知识库（可选）
-- INSERT INTO knowledge_bases (id, tenant_id, user_id, name, description, type, visibility)
-- VALUES (
--     '00000000-0000-0000-0000-000000000001',
--     '00000000-0000-0000-0000-000000000001',
--     (SELECT id FROM users WHERE role = 'owner' LIMIT 1),
--     '系统知识库',
--     'MiniCC 系统级公共知识库',
--     'rag',
--     'public'
-- );
