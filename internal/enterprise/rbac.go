package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/athenavi/minicc/internal/db"
	"github.com/redis/go-redis/v9"
)

const (
	// permsCacheKeyPrefix 是有效权限缓存键前缀，完整键为 ent:rbac:perms:{userID}
	permsCacheKeyPrefix = "ent:rbac:perms:"
	// permsCacheTTL 权限缓存过期时间
	permsCacheTTL = 5 * time.Minute
)

// LoadEffectivePerms 聚合用户的企业级有效权限：
// 用户直接角色权限 ∪ 用户所在群组→群组角色权限（ent_user_roles→ent_roles ∪
// ent_group_members→ent_group_roles→ent_roles），permissions text[] 并集去重。
//
// 返回值语义（防越权的关键区分）：
//   - nil, nil：用户没有任何 ent 角色记录（直接 + 群组均未关联），
//     调用方应回退旧权限体系（auth.HasPermission）。此状态不写缓存。
//   - 非 nil 空切片, nil：用户有角色记录但聚合权限为空，表示"明确无权限"，
//     调用方禁止回退旧体系。此状态以 "[]" 序列化缓存。
//   - 非空切片, nil：聚合后的权限点列表（已去重）。
//
// Redis 不可用或读取失败时降级直查 DB（slog.Warn，不返回错误）；
// DB 查询失败返回 error（由调用方决定 fail-open/fail-close 策略）。
func LoadEffectivePerms(ctx context.Context, userID string) ([]string, error) {
	// ── 1. 缓存读取（命中直接返回）──
	cacheKey := permsCacheKeyPrefix + userID
	if rdb := db.Redis; rdb != nil {
		cached, err := rdb.Get(ctx, cacheKey).Result()
		if err == nil {
			if perms, ok := decodePerms(cached); ok {
				return perms, nil
			}
			// 缓存内容损坏：删除脏键后回源 DB
			rdb.Del(ctx, cacheKey)
		} else if !errors.Is(err, redis.Nil) {
			slog.Warn("ent rbac: redis get failed, falling back to DB",
				"user_id", userID, "error", err)
		}
	} else {
		slog.Warn("ent rbac: redis unavailable, falling back to DB", "user_id", userID)
	}

	// ── 2. PG 聚合查询 ──
	perms, hasRoles, err := queryEffectivePerms(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !hasRoles {
		// 无 ent 角色记录：不缓存（nil 语义），交由调用方回退旧权限体系
		return nil, nil
	}

	// ── 3. 回写缓存（含空切片："[]" 保证"明确无权限"语义可缓存）──
	if rdb := db.Redis; rdb != nil {
		if encoded, err := encodePerms(perms); err == nil {
			if err := rdb.Set(ctx, cacheKey, encoded, permsCacheTTL).Err(); err != nil {
				slog.Warn("ent rbac: redis set failed", "user_id", userID, "error", err)
			}
		}
	}

	return perms, nil
}

// queryEffectivePerms 从 PG 聚合用户的有效权限。
// 返回 (并集去重后的权限列表, 是否存在任何角色关联, error)。
func queryEffectivePerms(ctx context.Context, userID string) ([]string, bool, error) {
	pool := db.ReadPool()
	if pool == nil {
		return nil, false, errors.New("ent rbac: postgres pool unavailable")
	}

	const query = `
SELECT COUNT(DISTINCT r.id)::int AS role_count,
       COALESCE(array_agg(DISTINCT p ORDER BY p) FILTER (WHERE p IS NOT NULL), '{}')::text[] AS perms
FROM (
    SELECT r.id, r.permissions
    FROM ent_user_roles ur
    JOIN ent_roles r ON r.id = ur.role_id
    WHERE ur.user_id = $1
    UNION ALL
    SELECT r.id, r.permissions
    FROM ent_group_members gm
    JOIN ent_group_roles gr ON gr.group_id = gm.group_id
    JOIN ent_roles r ON r.id = gr.role_id
    WHERE gm.user_id = $1
) AS role_grants
JOIN ent_roles r ON r.id = role_grants.id
CROSS JOIN LATERAL unnest(COALESCE(r.permissions, '{}')) AS p`

	var roleCount int
	var perms []string
	if err := pool.QueryRow(ctx, query, userID).Scan(&roleCount, &perms); err != nil {
		return nil, false, err
	}
	if roleCount == 0 {
		return nil, false, nil
	}
	return unionPerms(perms), true, nil
}

// InvalidateUserPerms 删除指定用户的有效权限缓存键。
// Redis 不可用或删除失败时仅记录日志（缓存最多 5 分钟自然过期）。
func InvalidateUserPerms(ctx context.Context, userID string) {
	rdb := db.Redis
	if rdb == nil {
		return
	}
	if err := rdb.Del(ctx, permsCacheKeyPrefix+userID).Err(); err != nil {
		slog.Warn("ent rbac: invalidate user perms cache failed",
			"user_id", userID, "error", err)
	}
}

// InvalidateGroupMembersPerms 批量失效群组所有成员的权限缓存。
// 供群组/角色写操作（修改群组成员、群组角色绑定、角色权限变更）后调用。
// 成员查询失败时仅记录日志（依赖 TTL 自然过期兜底，不阻断写路径）。
func InvalidateGroupMembersPerms(ctx context.Context, groupID string) {
	rdb := db.Redis
	if rdb == nil {
		return
	}

	pool := db.ReadPool()
	if pool == nil {
		slog.Warn("ent rbac: postgres pool unavailable, skip group member cache invalidation",
			"group_id", groupID)
		return
	}

	rows, err := pool.Query(ctx,
		`SELECT user_id FROM ent_group_members WHERE group_id = $1`, groupID)
	if err != nil {
		slog.Warn("ent rbac: query group members failed", "group_id", groupID, "error", err)
		return
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var memberID string
		if err := rows.Scan(&memberID); err != nil {
			slog.Warn("ent rbac: scan group member failed", "group_id", groupID, "error", err)
			continue
		}
		keys = append(keys, permsCacheKeyPrefix+memberID)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("ent rbac: iterate group members failed", "group_id", groupID, "error", err)
	}
	if len(keys) == 0 {
		return
	}

	if err := rdb.Del(ctx, keys...).Err(); err != nil {
		slog.Warn("ent rbac: batch invalidate member perms cache failed",
			"group_id", groupID, "members", len(keys), "error", err)
	}
}

// unionPerms 对权限列表去重（保持首次出现顺序）。纯函数，便于单元测试。
func unionPerms(perms []string) []string {
	if len(perms) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(perms))
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// encodePerms 将非 nil 权限切片序列化为缓存值（空切片 → "[]"）。
// nil 不应进入缓存，故入参约定为非 nil。
func encodePerms(perms []string) (string, error) {
	if perms == nil {
		perms = []string{}
	}
	data, err := json.Marshal(perms)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// decodePerms 反序列化缓存值。ok=false 表示缓存损坏，应按未命中处理。
// "[]" 解码为非 nil 空切片（明确无权限），与 nil（未配置、不入缓存）区分。
func decodePerms(raw string) ([]string, bool) {
	var perms []string
	if err := json.Unmarshal([]byte(raw), &perms); err != nil || perms == nil {
		return nil, false
	}
	return perms, true
}
