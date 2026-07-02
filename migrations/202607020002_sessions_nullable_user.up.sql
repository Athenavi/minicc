-- MiniCC V2.0: 允许匿名会话 (sessions.user_id 可空)
-- 使未登录用户也能持久化对话

ALTER TABLE sessions ALTER COLUMN user_id DROP NOT NULL;
