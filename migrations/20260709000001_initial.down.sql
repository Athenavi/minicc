-- MiniCC V2.0 企业版数据库回滚迁移
-- 删除所有表（逆序，处理外键依赖）

-- 先删除 RLS 策略
DROP POLICY IF EXISTS tenant_isolation ON marketing_campaigns;
DROP POLICY IF EXISTS tenant_isolation ON kb_articles;
DROP POLICY IF EXISTS tenant_isolation ON support_tickets;
DROP POLICY IF EXISTS tenant_isolation ON meeting_notes;
DROP POLICY IF EXISTS tenant_isolation ON okrs;
DROP POLICY IF EXISTS tenant_isolation ON wiki_pages;
DROP POLICY IF EXISTS tenant_isolation ON enterprise_tasks;
DROP POLICY IF EXISTS tenant_isolation ON media_assets;
DROP POLICY IF EXISTS tenant_isolation ON audit_logs;
DROP POLICY IF EXISTS tenant_isolation ON billing_records;
DROP POLICY IF EXISTS tenant_isolation ON agents;
DROP POLICY IF EXISTS tenant_isolation ON messages;
DROP POLICY IF EXISTS tenant_isolation ON sessions;
DROP POLICY IF EXISTS tenant_isolation ON users;

-- 删除表
DROP TABLE IF EXISTS audit_logs CASCADE;
DROP TABLE IF EXISTS marketing_campaigns CASCADE;
DROP TABLE IF EXISTS kb_articles CASCADE;
DROP TABLE IF EXISTS support_tickets CASCADE;
DROP TABLE IF EXISTS meeting_notes CASCADE;
DROP TABLE IF EXISTS okrs CASCADE;
DROP TABLE IF EXISTS wiki_pages CASCADE;
DROP TABLE IF EXISTS enterprise_tasks CASCADE;
DROP TABLE IF EXISTS media_assets CASCADE;
DROP TABLE IF EXISTS credit_transactions CASCADE;
DROP TABLE IF EXISTS stripe_payments CASCADE;
DROP TABLE IF EXISTS billing_records CASCADE;
DROP TABLE IF EXISTS tasks CASCADE;
DROP TABLE IF EXISTS workflow_graphs CASCADE;
DROP TABLE IF EXISTS tool_calls CASCADE;
DROP TABLE IF EXISTS messages CASCADE;
DROP TABLE IF EXISTS sessions CASCADE;
DROP TABLE IF EXISTS episodes CASCADE;
DROP TABLE IF EXISTS agent_sessions CASCADE;
DROP TABLE IF EXISTS agent_registry CASCADE;
DROP TABLE IF EXISTS agents CASCADE;
DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS guest_storage CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS tenants CASCADE;
