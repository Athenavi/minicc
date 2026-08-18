package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/enterprise"
)

// RequireEntPerm 企业版权限中间件，签名与 RequirePermission 保持一致。
//
// 决策流程：
//  1. 从 auth.GetClaims 取 UserID，调用 enterprise.LoadEffectivePerms 聚合
//     用户的企业级有效权限（直接角色 ∪ 群组成员角色，带 Redis 缓存）。
//  2. 返回 nil（用户无 ent 角色配置）→ 回退旧权限体系 auth.HasPermission。
//  3. 返回非 nil（含空切片）→ 仅当 perm 在切片内放行，否则 403；
//     空切片表示"明确无权限"，禁止回退旧体系（防越权）。
//  4. LoadEffectivePerms 出错（ent 基础设施故障）→ fail-open 回退
//     auth.HasPermission + slog.Warn，保证故障不阻断管理面。
//
// 注意：本中间件仅供后续任务挂载到 /v1/ent/* 路由使用，当前不注册任何路由。
func RequireEntPerm(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := auth.GetClaims(r.Context())
			if claims == nil || claims.UserID == "" {
				logAndRespond(w, errors.New("missing auth claims"),
					http.StatusUnauthorized, ErrAuthRequired)
				return
			}

			perms, err := enterprise.LoadEffectivePerms(r.Context(), claims.UserID)
			if err != nil {
				// ent 基础设施故障（如 PG 查询失败）：fail-open 回退旧权限体系
				slog.Warn("ent rbac: load effective perms failed, falling back to legacy permissions",
					"user_id", claims.UserID, "perm", perm, "error", err)
				if !auth.HasPermission(claims, perm) {
					logAndRespond(w, errors.New("insufficient permissions"),
						http.StatusForbidden, "insufficient permissions")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if perms == nil {
				// 用户无 ent 角色配置：回退旧权限体系
				if !auth.HasPermission(claims, perm) {
					logAndRespond(w, errors.New("insufficient permissions"),
						http.StatusForbidden, "insufficient permissions")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// 用户有 ent 配置（含空切片 = 明确无权限）：仅以聚合权限为准，禁止回退
			for _, p := range perms {
				if p == perm {
					next.ServeHTTP(w, r)
					return
				}
			}
			logAndRespond(w, errors.New("insufficient enterprise permissions"),
				http.StatusForbidden, "insufficient permissions")
		})
	}
}
