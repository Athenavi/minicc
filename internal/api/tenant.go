package api

import (
	"context"
	"net/http"

	"github.com/athenavi/minicc/internal/auth"
)

// contextKey 类型安全的 context key
type contextKey string

const (
	// CtxKeyTenantID 租户 ID 的 context key
	CtxKeyTenantID contextKey = "tenant_id"
	// CtxKeyUserID 用户 ID 的 context key
	CtxKeyUserID contextKey = "user_id"
)

// TenantMiddleware 租户隔离中间件
// 从 JWT Claims 中提取 tenant_id 与 user_id 并注入 context。
// claims.TenantID 必须非空（由 GenerateToken 在签发时填入），
// 空值表示上游签发链路异常，按未认证处理。
func TenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.GetClaims(r.Context())
		if claims == nil {
			Unauthorized(w, ErrAuthRequired)
			return
		}
		if claims.TenantID == "" {
			Unauthorized(w, "missing tenant context")
			return
		}

		ctx := context.WithValue(r.Context(), CtxKeyTenantID, claims.TenantID)
		ctx = context.WithValue(ctx, CtxKeyUserID, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole 要求特定角色的中间件
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := auth.GetClaims(r.Context())
			if claims == nil {
				Unauthorized(w, ErrAuthRequired)
				return
			}

			hasRole := false
			for _, role := range roles {
				if claims.Role == role {
					hasRole = true
					break
				}
			}

			if !hasRole {
				Forbidden(w, "insufficient role")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetUserID 从请求中获取用户 ID
func GetUserID(r *http.Request) string {
	userID, _ := r.Context().Value(CtxKeyUserID).(string)
	return userID
}

// GetTenantID 从请求中获取租户 ID（多租户隔离键）
func GetTenantID(r *http.Request) string {
	tenantID, _ := r.Context().Value(CtxKeyTenantID).(string)
	return tenantID
}
