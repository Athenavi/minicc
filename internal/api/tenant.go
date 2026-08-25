package api

import (
	"context"
	"net/http"

	"github.com/athenavi/chiron/internal/auth"
)

// contextKey 绫诲瀷瀹夊叏鐨?context key
type contextKey string

const (
	// CtxKeyTenantID 绉熸埛 ID 鐨?context key
	CtxKeyTenantID contextKey = "tenant_id"
	// CtxKeyUserID 鐢ㄦ埛 ID 鐨?context key
	CtxKeyUserID contextKey = "user_id"
)

// TenantMiddleware 绉熸埛闅旂涓棿浠?
// 浠?JWT Claims 涓彁鍙?tenant_id 涓?user_id 骞舵敞鍏?context銆?
// claims.TenantID 蹇呴』闈炵┖锛堢敱 GenerateToken 鍦ㄧ鍙戞椂濉叆锛夛紝
// 绌哄€艰〃绀轰笂娓哥鍙戦摼璺紓甯革紝鎸夋湭璁よ瘉澶勭悊銆?
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

// RequireRole 瑕佹眰鐗瑰畾瑙掕壊鐨勪腑闂翠欢
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

// GetUserID 浠庤姹備腑鑾峰彇鐢ㄦ埛 ID
func GetUserID(r *http.Request) string {
	userID, _ := r.Context().Value(CtxKeyUserID).(string)
	return userID
}

// GetTenantID 浠庤姹備腑鑾峰彇绉熸埛 ID锛堝绉熸埛闅旂閿級
func GetTenantID(r *http.Request) string {
	tenantID, _ := r.Context().Value(CtxKeyTenantID).(string)
	return tenantID
}
