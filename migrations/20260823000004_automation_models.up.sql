-- 20260823000004_automation_models: up
-- 定时自动化与模型路由
-- 1) cron_jobs 补充归属与触发信息
ALTER TABLE cron_jobs ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE cron_jobs ADD COLUMN IF NOT EXISTS user_id uuid;
ALTER TABLE cron_jobs ADD COLUMN IF NOT EXISTS webhook_token varchar(64) DEFAULT '' NOT NULL;

-- 2) llm_models 种子数据（默认模型池，admin 可增删）
INSERT INTO llm_models (provider, name, display_name, enabled, context_window) VALUES
    ('deepseek', 'deepseek-chat', 'DeepSeek Chat (V3)', true, 65536),
    ('deepseek', 'deepseek-reasoner', 'DeepSeek Reasoner (R1)', true, 65536),
    ('openai',  'gpt-4o-mini', 'GPT-4o mini', true, 128000),
    ('openai',  'gpt-4o', 'GPT-4o', false, 128000)
ON CONFLICT (provider, name) DO NOTHING;
