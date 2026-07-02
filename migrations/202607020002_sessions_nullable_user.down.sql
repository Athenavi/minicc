-- MiniCC V2.0: 回滚 sessions.user_id 可空
-- 注意：如果存在 NULL 值需要先处理
ALTER TABLE sessions ALTER COLUMN user_id SET NOT NULL;
