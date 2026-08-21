-- 知识库文档元数据增强：tenant_id 列 + 索引（幂等，兼容两种建表来源）
--
-- 背景：knowledge_documents 可能来自两个来源之一——
--   a) Go initial 迁移：建表时已含 tenant_id uuid 列（本迁移应跳过 ALTER）
--   b) Python db.py ensure_tables()：无 tenant_id（本迁移需补 VARCHAR(32) 列 + 回填）
-- 用 information_schema 探测列是否存在/类型，两条路径都幂等通过。
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'knowledge_documents' AND column_name = 'tenant_id'
    ) THEN
        -- 来源 b)：Python 侧建表，补列
        ALTER TABLE knowledge_documents ADD COLUMN tenant_id VARCHAR(32) NOT NULL DEFAULT 'default';

        -- 回填：从 knowledge_bases.user_id 继承（Python 侧无独立 tenant 概念时的 fallback）
        UPDATE knowledge_documents
        SET tenant_id = COALESCE(
            (SELECT kb.user_id FROM knowledge_bases kb WHERE kb.id = knowledge_documents.knowledge_base_id),
            'default'
        );
    ELSE
        RAISE NOTICE 'knowledge_documents.tenant_id already exists, skipping ALTER/backfill';
    END IF;
END
$$;

-- 按 tenant_id 过滤的索引（多租户隔离查询；uuid/varchar 均适用）
CREATE INDEX IF NOT EXISTS idx_knowledge_documents_tenant ON knowledge_documents (tenant_id, created_at DESC);
