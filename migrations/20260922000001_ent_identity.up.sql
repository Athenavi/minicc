-- Enterprise Identity (RBAC & Groups) Migration
-- Version: 1.0
-- Date: 2026-09-22
-- Description: 企业级身份管理：角色(RBAC)、用户组及成员/角色关联表
-- 注意: 所有外键一律锚定真实表 tenants(id) / users(id)，不引用影子表 admin_tenants

-- ── 1. 角色表 ────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(64) NOT NULL,
    display_name VARCHAR(128),
    is_builtin BOOLEAN DEFAULT FALSE,
    permissions TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(tenant_id, name)
);

COMMENT ON TABLE ent_roles IS 'Enterprise RBAC roles with permission points (resource:action)';

-- ── 2. 用户-角色关联表 ───────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES ent_roles(id) ON DELETE CASCADE,

    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_ent_user_roles_role ON ent_user_roles(role_id);

COMMENT ON TABLE ent_user_roles IS 'User-to-role assignments';

-- ── 3. 用户组表 ──────────────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(128) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(tenant_id, name)
);

COMMENT ON TABLE ent_groups IS 'Enterprise user groups';

-- ── 4. 用户组成员表 ──────────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_group_members (
    group_id UUID NOT NULL REFERENCES ent_groups(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_ent_group_members_user ON ent_group_members(user_id);

COMMENT ON TABLE ent_group_members IS 'Group membership (group -> user)';

-- ── 5. 用户组-角色关联表 ─────────────────────────────────

CREATE TABLE IF NOT EXISTS ent_group_roles (
    group_id UUID NOT NULL REFERENCES ent_groups(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES ent_roles(id) ON DELETE CASCADE,

    PRIMARY KEY (group_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_ent_group_roles_role ON ent_group_roles(role_id);

COMMENT ON TABLE ent_group_roles IS 'Group-to-role assignments (roles granted to all group members)';

-- ── 初始化数据: 默认租户内置角色 ─────────────────────────

-- 权限点风格与 internal/auth/jwt.go 一致 (resource:action)
-- 企业新增权限点: ent:manage / cost:manage / audit:read / market:manage / sso:manage / policy:manage
INSERT INTO ent_roles (tenant_id, name, display_name, is_builtin, permissions) VALUES
('00000000-0000-0000-0000-000000000001', 'platform_admin', '平台管理员', TRUE,
 ARRAY['chat:read','chat:write','admin:read','admin:write','tools:execute','users:manage',
       'ent:manage','cost:manage','audit:read','market:manage','sso:manage','policy:manage']),
('00000000-0000-0000-0000-000000000001', 'tenant_admin', '租户管理员', TRUE,
 ARRAY['admin:read','admin:write','users:manage','ent:manage','cost:manage',
       'audit:read','market:manage','policy:manage']),
('00000000-0000-0000-0000-000000000001', 'member', '成员', TRUE,
 ARRAY['chat:read','chat:write','tools:execute'])
ON CONFLICT (tenant_id, name) DO NOTHING;
