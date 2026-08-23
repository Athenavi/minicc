-- 20260821000004_memory_summaries: down
-- 回滚：删除对话摘要表（L3层）

DROP TABLE IF EXISTS memory_summaries;

-- 相关索引（表删除后自动移除，此处保留以便手动清理残留）
DROP INDEX IF EXISTS idx_ms_lookup;
DROP INDEX IF EXISTS uq_ms_hash;
DROP INDEX IF EXISTS idx_ms_access;
DROP INDEX IF EXISTS idx_ms_session;