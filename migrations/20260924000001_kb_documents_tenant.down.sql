-- 回滚：移除 knowledge_documents 的 tenant_id 相关变更
DROP INDEX IF EXISTS idx_knowledge_documents_tenant;
ALTER TABLE knowledge_documents DROP COLUMN IF EXISTS tenant_id;
