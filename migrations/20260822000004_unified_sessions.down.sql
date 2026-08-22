-- 20260822000004_unified_sessions: down
DROP INDEX IF EXISTS idx_unified_messages_session;
DROP TABLE IF EXISTS unified_messages;
DROP INDEX IF EXISTS idx_unified_sessions_user_updated;
DROP TABLE IF EXISTS unified_sessions;
