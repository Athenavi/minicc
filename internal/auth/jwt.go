package auth

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/athenavi/chiron/internal/id"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   string   `json:"uid"`
	Email    string   `json:"email"`
	Role     string   `json:"role"` // owner / admin / user
	TenantID string   `json:"tenant_id,omitempty"`
	Perms    []string `json:"perms,omitempty"`
	jwt.RegisteredClaims
}

type Authenticator struct {
	secret     []byte
	expiration time.Duration
}

// SigningSecret 杩斿洖绛惧悕瀵嗛挜锛堜緵濯掍綋绛惧悕 URL 绛夊鐢ㄥ悓涓€瀵嗛挜锛夈€?
func (a *Authenticator) SigningSecret() []byte {
	return a.secret
}

func NewAuthenticator(secret string, expiration time.Duration) *Authenticator {
	return &Authenticator{
		secret:     []byte(secret),
		expiration: expiration,
	}
}

// GenerateToken 绛惧彂 JWT銆倀enantID 蹇呴』鐢辫皟鐢ㄦ柟浠庣敤鎴疯褰曚腑浼犲叆锛堝绉熸埛闅旂閿級銆?
func (a *Authenticator) GenerateToken(userID, email, role, tenantID string, perms []string) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:   userID,
		Email:    email,
		Role:     role,
		TenantID: tenantID,
		Perms:    perms,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(a.expiration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "chiron",
			ID:        generateID(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.secret)
}

func (a *Authenticator) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return a.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("token validation: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

func (a *Authenticator) RefreshToken(tokenStr string) (string, error) {
	claims, err := a.ValidateToken(tokenStr)
	if err != nil {
		return "", err
	}
	return a.GenerateToken(claims.UserID, claims.Email, claims.Role, claims.TenantID, claims.Perms)
}

type contextKey string

const ClaimsKey contextKey = "claims"

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, ClaimsKey, claims)
}

func GetClaims(ctx context.Context) *Claims {
	if ctx == nil {
		return nil
	}
	claims, _ := ctx.Value(ClaimsKey).(*Claims)
	return claims
}

func generateID() string {
	return id.NextID()
}

// Global JWT authenticator instance for use in middleware.
var jwtAuth *Authenticator

// InitJWTAuth initializes the global JWT authenticator from environment variables.
func InitJWTAuth() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		slog.Error("JWT_SECRET environment variable is required and must be at least 32 characters")
		os.Exit(1)
	}
	if len(secret) < 32 {
		slog.Error("JWT_SECRET must be at least 32 characters long", "length", len(secret))
		os.Exit(1)
	}
	expiration := 24 * time.Hour
	if v := os.Getenv("JWT_EXPIRATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			expiration = d
		}
	}
	jwtAuth = NewAuthenticator(secret, expiration)
}

// ParseJWT is a convenience function that parses and validates a JWT token string.
func ParseJWT(token string) (*Claims, error) {
	if jwtAuth == nil {
		return nil, fmt.Errorf("JWT authenticator not initialized, call InitJWTAuth() first")
	}
	return jwtAuth.ValidateToken(token)
}

// Built-in permissions
var (
	PermChatWrite   = "chat:write"
	PermChatRead    = "chat:read"
	PermAdminRead   = "admin:read"
	PermAdminWrite  = "admin:write"
	PermToolsExec   = "tools:execute"
	PermUsersManage = "users:manage"
	// 浼佷笟鍔熻兘鏉冮檺鐐癸紙owner/admin 榛樿鎷ユ湁锛?
	PermAuditRead   = "audit:read"
	PermEntManage   = "ent:manage"
	PermPolicyManage = "policy:manage"
	PermMarketManage = "market:manage"
	PermSSOManage   = "sso:manage"
)

// RolePermissions maps roles to permission sets
var RolePermissions = map[string][]string{
	"owner": {PermChatWrite, PermChatRead, PermAdminRead, PermAdminWrite, PermToolsExec, PermUsersManage, PermAuditRead, PermEntManage, PermPolicyManage, PermMarketManage, PermSSOManage},
	"admin": {PermChatWrite, PermChatRead, PermAdminRead, PermAdminWrite, PermToolsExec, PermAuditRead, PermEntManage, PermPolicyManage, PermMarketManage, PermSSOManage},
	"user":  {PermChatWrite, PermChatRead, PermToolsExec},
}

func HasPermission(claims *Claims, perm string) bool {
	if claims == nil {
		return false
	}
	// 濡傛灉 claims.Perms 琚樉寮忚缃紙闈炵┖锛夛紝浠ュ畠涓哄敮涓€鏉冮檺鐧藉悕鍗?
	if len(claims.Perms) > 0 {
		for _, p := range claims.Perms {
			if p == perm {
				return true
			}
		}
		return false
	}
	// Fallback 鍒?RolePermissions锛堜粎褰?Perms 涓虹┖鏃讹級
	perms, ok := RolePermissions[claims.Role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}
