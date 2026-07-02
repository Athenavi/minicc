package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string   `json:"uid"`
	Email  string   `json:"email"`
	Role   string   `json:"role"` // owner / admin / user
	Perms  []string `json:"perms,omitempty"`
	jwt.RegisteredClaims
}

type Authenticator struct {
	secret     []byte
	expiration time.Duration
}

func NewAuthenticator(secret string, expiration time.Duration) *Authenticator {
	return &Authenticator{
		secret:     []byte(secret),
		expiration: expiration,
	}
}

func (a *Authenticator) GenerateToken(userID, email, role string, perms []string) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		Perms:  perms,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(a.expiration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "minicc",
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
	return a.GenerateToken(claims.UserID, claims.Email, claims.Role, claims.Perms)
}

type contextKey string

const ClaimsKey contextKey = "claims"

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, ClaimsKey, claims)
}

func GetClaims(ctx context.Context) *Claims {
	claims, _ := ctx.Value(ClaimsKey).(*Claims)
	return claims
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Built-in permissions
var (
	PermChatWrite   = "chat:write"
	PermChatRead    = "chat:read"
	PermAdminRead   = "admin:read"
	PermAdminWrite  = "admin:write"
	PermToolsExec   = "tools:execute"
	PermUsersManage = "users:manage"
)

// RolePermissions maps roles to permission sets
var RolePermissions = map[string][]string{
	"owner": {PermChatWrite, PermChatRead, PermAdminRead, PermAdminWrite, PermToolsExec, PermUsersManage},
	"admin": {PermChatWrite, PermChatRead, PermAdminRead, PermAdminWrite, PermToolsExec},
	"user":  {PermChatWrite, PermChatRead, PermToolsExec},
}

func HasPermission(claims *Claims, perm string) bool {
	if claims == nil {
		return false
	}
	for _, p := range claims.Perms {
		if p == perm {
			return true
		}
	}
	// Fallback to role-based
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
