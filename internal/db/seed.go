package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultTenantID 是单租户部署的默认租户 ID。
// 与 internal/api/auth.go、internal/session/manager.go 中定义的常量保持一致，
// 注册与会话落库均硬编码引用该 ID。
const DefaultTenantID = "00000000-0000-0000-0000-000000000001"

// EnsureDefaultTenant 幂等确保默认租户存在。
//
// users/sessions 等表均有 tenants(id) 外键（users_tenant_id_fkey 等），
// 而 tenants 表本身没有任何种子数据；若默认租户缺失，注册会直接违反
// 外键约束（SQLSTATE 23503）。该函数不依赖迁移状态（schema_migrations），
// 对通过任何方式初始化的库都能生效。
func EnsureDefaultTenant(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name) VALUES ($1, 'default') ON CONFLICT (id) DO NOTHING`,
		DefaultTenantID)
	return err
}
