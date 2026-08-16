-- Seed the default tenant.
--
-- 注册/会话代码硬编码 DefaultTenantID = '00000000-0000-0000-0000-000000000001'
-- （internal/api/auth.go、internal/session/manager.go），而 users/sessions 等表
-- 均有 users_tenant_id_fkey → tenants(id) 外键。若无此行，注册时
-- INSERT INTO users 会违反外键约束（SQLSTATE 23503）。
INSERT INTO tenants (id, name)
VALUES ('00000000-0000-0000-0000-000000000001', 'default')
ON CONFLICT (id) DO NOTHING;
