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
// 从 JWT Claims 中提取用户信息并注入 context
func TenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.GetClaims(r.Context())
		if claims == nil {
			Unauthorized(w, "authentication required")
			return
		}

		// 注入用户上下文（当前 auth.Claims 没有 TenantID，使用 UserID 作为隔离键）
		ctx := context.WithValue(r.Context(), CtxKeyUserID, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole 要求特定角色的中间件
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := auth.GetClaims(r.Context())
			if claims == nil {
				Unauthorized(w, "authentication required")
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
