-- 工作流执行历史（持久化）
-- execute 后台任务完成后写入；前端"执行历史"与 status 轮询回退查询
CREATE TABLE workflow_instances (
    id varchar(64) PRIMARY KEY,
    user_id varchar(64) NOT NULL DEFAULT '',
    workflow_id varchar(64) NOT NULL DEFAULT '',
    workflow_name varchar(255) NOT NULL DEFAULT '',
    status varchar(16) NOT NULL DEFAULT 'running',
    results jsonb NOT NULL DEFAULT '{}',
    error text,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);
CREATE INDEX idx_workflow_instances_user ON workflow_instances (user_id, created_at DESC);
