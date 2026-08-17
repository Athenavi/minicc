-- Tenant & Infrastructure Management Migration
-- Version: 1.0
-- Date: 2026-08-17
-- Description: 创建租户管理与基础设施管理所需的数据库表

-- ── 1. 租户管理表 ────────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    
    -- 企业信息
    company_name VARCHAR(200),
    contact_email VARCHAR(100),
    contact_phone VARCHAR(20),
    
    -- 配额控制
    max_api_keys INT DEFAULT 10,
    max_models INT DEFAULT 5,
    monthly_quota BIGINT DEFAULT 0,  -- 月度 credits 配额 (0 = 无限制)
    max_concurrent_sessions INT DEFAULT 10,
    
    -- 状态管理
    status VARCHAR(20) DEFAULT 'active',  -- active/suspended/expired
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_by VARCHAR(50),
    
    -- 高级特性
    features JSONB DEFAULT '{}',  -- {"workflows": true, "custom_domains": true}
    
    CONSTRAINT chk_tenant_status CHECK (status IN ('active', 'suspended', 'expired')),
    CONSTRAINT chk_quota CHECK (monthly_quota >= 0)
);

-- 索引
CREATE INDEX idx_tenants_status ON admin_tenants(status);
CREATE INDEX idx_tenants_created ON admin_tenants(created_at DESC);

COMMENT ON TABLE admin_tenants IS 'Multi-tenant management with quota control';

-- ── 2. 租户用量统计表 ────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_tenant_usage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(50) NOT NULL REFERENCES admin_tenants(tenant_id),
    
    -- 每日统计
    stat_date DATE NOT NULL,
    
    -- 用量指标
    api_calls BIGINT DEFAULT 0,
    tokens_used BIGINT DEFAULT 0,
    credits_consumed BIGINT DEFAULT 0,
    storage_mb FLOAT DEFAULT 0,
    
    -- 时间戳
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    UNIQUE(tenant_id, stat_date)
);

CREATE INDEX idx_tenant_usage_date ON admin_tenant_usage(stat_date DESC);

COMMENT ON TABLE admin_tenant_usage IS 'Daily usage statistics per tenant';

-- ── 3. 域名管理表 ────────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain VARCHAR(100) NOT NULL UNIQUE,
    tenant_id VARCHAR(50) NOT NULL REFERENCES admin_tenants(tenant_id),
    
    -- DNS 配置
    dns_provider VARCHAR(50),  -- cloudflare / aliyun / tencent
    dns_record_id VARCHAR(100),
    cname_target VARCHAR(200),
    
    -- SSL 证书
    ssl_status VARCHAR(20) DEFAULT 'pending',  -- pending/active/expired/failed
    ssl_expires_at TIMESTAMP WITH TIME ZONE,
    auto_renew BOOLEAN DEFAULT TRUE,
    
    -- 状态管理
    status VARCHAR(20) DEFAULT 'active',  -- active/inactive/verifying
    verified_at TIMESTAMP WITH TIME ZONE,
    verified_by VARCHAR(50),
    
    -- 元数据
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    CONSTRAINT chk_domain_status CHECK (status IN ('active', 'inactive', 'verifying')),
    CONSTRAINT chk_ssl_status CHECK (ssl_status IN ('pending', 'active', 'expired', 'failed'))
);

CREATE INDEX idx_domains_tenant ON admin_domains(tenant_id);
CREATE INDEX idx_domains_status ON admin_domains(status);

COMMENT ON TABLE admin_domains IS 'Domain management with SSL certificate tracking';

-- ── 4. Redis 配置表 ────────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_redis_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- 连接信息
    host VARCHAR(100) NOT NULL,
    port INT DEFAULT 6379,
    password_hash VARCHAR(256),
    db_index INT DEFAULT 0,
    
    -- 连接池配置
    pool_size INT DEFAULT 100,
    min_idle_connections INT DEFAULT 10,
    max_conn_age INTERVAL DEFAULT '300s',
    
    -- 运行时状态
    status VARCHAR(20) DEFAULT 'active',
    last_health_check TIMESTAMP WITH TIME ZONE,
    avg_latency_ms FLOAT DEFAULT 0,
    
    -- 统计信息
    memory_used_mb FLOAT DEFAULT 0,
    connected_clients INT DEFAULT 0,
    hits BIGINT DEFAULT 0,
    misses BIGINT DEFAULT 0,
    
    -- 元数据
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    UNIQUE(host, port)
);

COMMENT ON TABLE admin_redis_configs IS 'Redis connection and pool configuration';

-- ── 5. 数据库配置表 ────────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_db_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- 连接信息
    dsn VARCHAR(500) NOT NULL,
    host VARCHAR(100) NOT NULL,
    port INT DEFAULT 5432,
    dbname VARCHAR(100) NOT NULL,
    
    -- 连接池配置
    max_open_connections INT DEFAULT 25,
    max_idle_connections INT DEFAULT 5,
    conn_max_lifetime INTERVAL DEFAULT '300s',
    
    -- 运行时状态
    status VARCHAR(20) DEFAULT 'active',
    last_health_check TIMESTAMP WITH TIME ZONE,
    avg_query_time_ms FLOAT DEFAULT 0,
    
    -- 统计信息
    database_size_mb FLOAT DEFAULT 0,
    total_tables INT DEFAULT 0,
    sequential_scans BIGINT DEFAULT 0,
    
    -- 元数据
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    UNIQUE(host, port, dbname)
);

COMMENT ON TABLE admin_db_configs IS 'PostgreSQL connection and pool configuration';

-- ── 6. 数据库备份表 ────────────────────────────────────────

CREATE TABLE IF NOT EXISTS admin_database_backups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- 备份信息
    backup_type VARCHAR(20) DEFAULT 'manual',  -- manual/scheduled
    description TEXT,
    file_path VARCHAR(500),
    file_size_mb FLOAT,
    
    -- 状态管理
    status VARCHAR(20) DEFAULT 'running',  -- running/completed/failed/deleted
    error_message TEXT,
    
    -- 时间戳
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_seconds INT,
    
    created_by VARCHAR(50),
    
    CONSTRAINT chk_backup_type CHECK (backup_type IN ('manual', 'scheduled')),
    CONSTRAINT chk_backup_status CHECK (status IN ('running', 'completed', 'failed', 'deleted'))
);

CREATE INDEX idx_backups_status ON admin_database_backups(status);
CREATE INDEX idx_backups_created ON admin_database_backups(started_at DESC);

COMMENT ON TABLE admin_database_backups IS 'Database backup history with status tracking';

-- ── 初始化数据 ────────────────────────────────────────────

-- 插入默认租户 (超级管理员租户)
INSERT INTO admin_tenants (
    tenant_id, name, company_name, contact_email,
    max_api_keys, max_models, monthly_quota, max_concurrent_sessions,
    features, status
) VALUES (
    'super_admin', '超级管理员', 'MiniCC Platform', 'admin@minicc.ai',
    100, 50, 0, 500,
    '{"workflows": true, "custom_domains": true, "priority_support": true, "analytics": true}'::jsonb,
    'active'
) ON CONFLICT (tenant_id) DO NOTHING;

-- 插入示例租户
INSERT INTO admin_tenants (
    tenant_id, name, company_name, contact_email,
    max_api_keys, max_models, monthly_quota, max_concurrent_sessions,
    features, status
) VALUES
('acme_corp', 'ACME Corporation', 'ACME Corp', 'admin@acme.com',
 20, 10, 100000, 50,
 '{"workflows": true, "custom_domains": true}'::jsonb,
 'active'),
('beta_tester', 'Beta Testing Tenant', 'Beta Inc', 'test@beta.com',
 5, 3, 10000, 10,
 '{"workflows": false, "custom_domains": false}'::jsonb,
 'active')
ON CONFLICT (tenant_id) DO NOTHING;

-- 插入 Redis 配置 (从环境变量读取)
-- 注意: 实际部署时应通过配置管理工具注入
-- INSERT INTO admin_redis_configs (host, port, pool_size, min_idle_connections)
-- VALUES ('localhost', 6379, 100, 10)
-- ON CONFLICT (host, port) DO NOTHING;

-- 插入数据库配置 (当前数据库)
INSERT INTO admin_db_configs (dsn, host, port, dbname, max_open_connections, max_idle_connections)
VALUES (
    'postgresql://postgres:password@localhost:5432/minicc?sslmode=disable',
    'localhost', 5432, 'minicc',
    25, 5
)
ON CONFLICT (host, port, dbname) DO NOTHING;
