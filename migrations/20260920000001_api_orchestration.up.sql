-- API Orchestration Engine Migration
-- Version: 1.0
-- Date: 2026-08-17
-- Description: 创建 API 编排引擎所需的数据库表

-- ── 1. API Key 管理表 ────────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash VARCHAR(64) NOT NULL UNIQUE,  -- SHA256 hash of the actual key
    
    -- 基本信息
    name VARCHAR(100) NOT NULL,
    tenant_id VARCHAR(50) NOT NULL,
    user_id VARCHAR(50),
    
    -- 配额控制
    monthly_quota INT DEFAULT 0,  -- 月度配额 (0 = 无限制)
    used_count BIGINT DEFAULT 0,
    used_credits BIGINT DEFAULT 0,
    
    -- 状态管理
    status VARCHAR(20) DEFAULT 'active',  -- active/expired/suspended
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- 元数据
    created_by VARCHAR(50),
    description TEXT,
    allowed_models TEXT[],  -- ["gpt-4", "claude-3"]
    rate_limit_qps INT DEFAULT 10,
    
    CONSTRAINT chk_status CHECK (status IN ('active', 'expired', 'suspended')),
    CONSTRAINT chk_quota CHECK (monthly_quota >= 0)
);

-- 索引
CREATE INDEX idx_api_keys_hash ON admin_api_keys(key_hash);
CREATE INDEX idx_api_keys_tenant_status ON admin_api_keys(tenant_id, status);
CREATE INDEX idx_api_keys_expires ON admin_api_keys(expires_at) WHERE status = 'active';
CREATE INDEX idx_api_keys_created ON admin_api_keys(created_at DESC);

COMMENT ON TABLE admin_api_keys IS 'API Key management with quota and lifecycle control';

-- ── 2. 模型配置表 ────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_model_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- 模型标识
    model_id VARCHAR(50) NOT NULL UNIQUE,  -- "gpt-4-turbo"
    display_name VARCHAR(100) NOT NULL,    -- "GPT-4 Turbo"
    provider VARCHAR(50) NOT NULL,         -- "openai" / "anthropic" / "deepseek"
    
    -- 路由策略
    priority INT DEFAULT 0,                -- 优先级 (越高越优先)
    weight INT DEFAULT 100,                -- 负载均衡权重 (1-100)
    fallback_chain TEXT[],                 -- ["gpt-4", "gpt-3.5-turbo"]
    
    -- 配额控制
    max_rpm INT DEFAULT 1000,              -- 每分钟请求数
    max_tpm INT DEFAULT 500000,            -- 每分钟 Token 数
    concurrent_limit INT DEFAULT 50,       -- 并发限制
    
    -- 状态管理
    status VARCHAR(20) DEFAULT 'active',   -- active/deprecated/maintenance
    is_default BOOLEAN DEFAULT FALSE,
    
    -- 成本信息 (每百万 Token 的 credits 消耗)
    input_cost_per_1m FLOAT DEFAULT 0,
    output_cost_per_1m FLOAT DEFAULT 0,
    
    -- Provider 特定配置
    config_json JSONB DEFAULT '{}',
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    CONSTRAINT chk_model_status CHECK (status IN ('active', 'deprecated', 'maintenance')),
    CONSTRAINT chk_weight CHECK (weight >= 1 AND weight <= 100)
);

-- 索引
CREATE INDEX idx_model_configs_status ON admin_model_configs(status);
CREATE INDEX idx_model_configs_provider ON admin_model_configs(provider);

COMMENT ON TABLE admin_model_configs IS 'Model configuration with routing strategies';

-- ── 3. 工作流编排表 ──────────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_workflows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    
    -- DAG 定义
    nodes JSONB NOT NULL,                  -- [{id, type, api_endpoint, params}]
    edges JSONB NOT NULL,                  -- [{source, target}]
    error_handling_strategy VARCHAR(20) DEFAULT 'fail_fast',
    
    -- 执行配置
    timeout_ms INT DEFAULT 30000,
    max_retries INT DEFAULT 3,
    
    -- 版本管理
    version INT DEFAULT 1,
    published_version INT DEFAULT 0,
    
    -- 状态管理
    status VARCHAR(20) DEFAULT 'draft',    -- draft/testing/published/archived
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    published_at TIMESTAMP WITH TIME ZONE,
    
    CONSTRAINT chk_workflow_status CHECK (status IN ('draft', 'testing', 'published', 'archived')),
    CONSTRAINT chk_error_strategy CHECK (error_handling_strategy IN ('fail_fast', 'continue', 'skip'))
);

-- 索引
CREATE INDEX idx_workflows_status ON admin_workflows(status);
CREATE INDEX idx_workflows_created ON admin_workflows(created_at DESC);

COMMENT ON TABLE admin_workflows IS 'Workflow DAG orchestration with version control';

-- ── 4. 工作流执行日志表 ──────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_workflow_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id VARCHAR(50) NOT NULL,
    workflow_version INT NOT NULL,
    
    -- 执行状态
    status VARCHAR(20) DEFAULT 'running',  -- running/completed/failed/cancelled
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_ms INT,
    
    -- 输入输出
    input_data JSONB,
    output_data JSONB,
    error_message TEXT,
    
    -- 元数据
    triggered_by VARCHAR(50),
    node_results JSONB DEFAULT '[]',       -- [{node_id, status, duration_ms, output}]
    
    CONSTRAINT chk_execution_status CHECK (status IN ('running', 'completed', 'failed', 'cancelled'))
);

-- 索引
CREATE INDEX idx_workflow_executions_workflow ON admin_workflow_executions(workflow_id, workflow_version);
CREATE INDEX idx_workflow_executions_status ON admin_workflow_executions(status);
CREATE INDEX idx_workflow_executions_started ON admin_workflow_executions(started_at DESC);

COMMENT ON TABLE admin_workflow_executions IS 'Workflow execution history with node-level results';

-- ── 5. API 调用日志表 ────────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_api_call_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- 调用信息
    api_key_id UUID REFERENCES admin_api_keys(id),
    model_id VARCHAR(50),
    workflow_id VARCHAR(50),
    
    -- 请求详情
    endpoint VARCHAR(100) NOT NULL,
    method VARCHAR(10) DEFAULT 'POST',
    request_size_bytes INT,
    response_size_bytes INT,
    
    -- 性能指标
    duration_ms INT,
    status_code INT,
    retry_count INT DEFAULT 0,
    
    -- 成本统计
    input_tokens INT DEFAULT 0,
    output_tokens INT DEFAULT 0,
    credits_consumed BIGINT DEFAULT 0,
    
    -- 时间戳
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 索引 (高频查询优化)
CREATE INDEX idx_api_logs_key ON admin_api_call_logs(api_key_id) WHERE api_key_id IS NOT NULL;
CREATE INDEX idx_api_logs_model ON admin_api_call_logs(model_id) WHERE model_id IS NOT NULL;
CREATE INDEX idx_api_logs_workflow ON admin_api_call_logs(workflow_id) WHERE workflow_id IS NOT NULL;
CREATE INDEX idx_api_logs_created ON admin_api_call_logs(created_at DESC);
CREATE INDEX idx_api_logs_date ON admin_api_call_logs(((created_at AT TIME ZONE 'UTC')::date));

COMMENT ON TABLE admin_api_call_logs IS 'API call audit log with performance and cost metrics';

-- ── 6. 定时任务记录表 ────────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_cron_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id VARCHAR(50) NOT NULL UNIQUE,    -- "cleanup_expired_keys"
    name VARCHAR(100) NOT NULL,
    schedule VARCHAR(50) NOT NULL,         -- cron expression ("0 2 * * *")
    
    -- 执行状态
    last_run_at TIMESTAMP WITH TIME ZONE,
    last_run_status VARCHAR(20),           -- success/failed/running
    last_error TEXT,
    next_run_at TIMESTAMP WITH TIME ZONE,
    
    -- 配置
    enabled BOOLEAN DEFAULT TRUE,
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 索引
CREATE UNIQUE INDEX idx_cron_jobs_id ON admin_cron_jobs(job_id);

COMMENT ON TABLE admin_cron_jobs IS 'Cron job scheduler registry';

-- ── 初始化数据 ───────────────────────────────────────────────

-- 插入默认模型配置
INSERT INTO admin_model_configs (
    model_id, display_name, provider, priority, weight,
    max_rpm, max_tpm, concurrent_limit,
    input_cost_per_1m, output_cost_per_1m,
    is_default, status
) VALUES
('gpt-4-turbo', 'GPT-4 Turbo', 'openai', 100, 50, 1000, 500000, 50, 20, 60, FALSE, 'active'),
('gpt-3.5-turbo', 'GPT-3.5 Turbo', 'openai', 50, 30, 3000, 2000000, 100, 3, 6, FALSE, 'active'),
('claude-3-opus', 'Claude 3 Opus', 'anthropic', 90, 20, 500, 200000, 30, 30, 150, FALSE, 'active'),
('claude-3-sonnet', 'Claude 3 Sonnet', 'anthropic', 60, 40, 1000, 1000000, 50, 8, 24, FALSE, 'active'),
('deepseek-coder', 'DeepSeek Coder', 'deepseek', 40, 60, 2000, 1000000, 80, 5, 10, TRUE, 'active')
ON CONFLICT (model_id) DO NOTHING;

-- 插入示例定时任务
INSERT INTO admin_cron_jobs (job_id, name, schedule, enabled) VALUES
('cleanup_expired_keys', '清理过期 API Key', '0 2 * * *', TRUE),
('weekly_usage_report', '生成用量统计报表', '0 6 * * 0', TRUE),
('refresh_model_cache', '刷新模型配置缓存', '*/5 * * * *', TRUE),
('health_check', '模型健康检查', '*/30 * * * *', TRUE)
ON CONFLICT (job_id) DO NOTHING;

-- ── 权限设置 ─────────────────────────────────────────────────

-- 假设有一个 admin_role 角色
-- GRANT ALL ON ALL TABLES IN SCHEMA public TO admin_role;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO admin_role;
