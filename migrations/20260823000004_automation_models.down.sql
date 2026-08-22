-- 20260823000004_automation_models: down
DELETE FROM llm_models WHERE provider = 'deepseek' AND name IN ('deepseek-chat', 'deepseek-reasoner');
DELETE FROM llm_models WHERE provider = 'openai' AND name IN ('gpt-4o-mini', 'gpt-4o');
ALTER TABLE cron_jobs DROP COLUMN IF EXISTS webhook_token;
ALTER TABLE cron_jobs DROP COLUMN IF EXISTS user_id;
ALTER TABLE cron_jobs DROP COLUMN IF EXISTS tenant_id;
