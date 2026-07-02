ALTER TABLE workflow_definitions ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE workflow_executions ALTER COLUMN user_id DROP NOT NULL;
