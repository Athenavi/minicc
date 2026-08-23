-- 20260823000006_user_memory_profile: down
-- 回滚：删除 user_memory_profile 表及相关对象

DROP TRIGGER IF EXISTS trigger_ump_updated_at ON user_memory_profile;
DROP FUNCTION IF EXISTS update_updated_at_column();

DROP TABLE IF EXISTS user_memory_profile;