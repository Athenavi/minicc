-- 知识库文档元数据增强：添加 tenant_id 列 + 索引
-- knowledge_documents 表已由 db.py ensure_tables() 创建，此处补 tenant_id 列
ALTER TABLE knowledge_documents ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(32) NOT NULL DEFAULT 'default';

-- 按 tenant_id 过滤的索引（多租户隔离查询）
CREATE INDEX IF NOT EXISTS idx_knowledge_documents_tenant ON knowledge_documents (tenant_id, created_at DESC);

-- 回填：从 knowledge_bases 表继承 tenant_id（兼容旧数据）
-- knowledge_bases 表也使用 user_id 而非 tenant_id，将 user_id 作为 tenant_id 的 fallback
UPDATE knowledge_documents
SET tenant_id = COALESCE(
    (SELECT kb.user_id FROM knowledge_bases kb WHERE kb.id = knowledge_documents.knowledge_base_id),
    'default'
)
WHERE tenant_id = 'default' OR tenant_id IS NULL;
