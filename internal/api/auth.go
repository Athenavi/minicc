package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/id"
	"golang.org/x/crypto/bcrypt"
)

const tokenCookieName = "minicc_token"

// DefaultTenantID 是默认租户 ID（用于单租户模式）
const DefaultTenantID = "00000000-0000-0000-0000-000000000001"

type AuthHandler struct {
	auth *auth.Authenticator
	cfg  *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		auth: auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration),
		cfg:  cfg,
	}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

// SetTokenCookie sets the JWT as an HTTP-only secure cookie.
func SetTokenCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     tokenCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // 允许 JS 读取以便前端使用
		Secure:   false, // 本地开发不需要 HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func ClearTokenCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     tokenCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		BadRequest(w, "email and password are required")
		return
	}
	if len(req.Email) > 255 {
		BadRequest(w, "email too long")
		return
	}
	if len(req.Password) > 128 {
		BadRequest(w, "password too long")
		return
	}

	// No dev bypass — always validate against DB
	if db.Pool == nil {
		InternalError(w, "database not available")
		return
	}

	ctx := r.Context()

	// 设置租户上下文以绕过 RLS —— 必须在事务中才能让 SET LOCAL 持续生效
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		slog.Error("begin tx for tenant context", "error", err)
		InternalError(w, "login failed")
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", DefaultTenantID); err != nil {
		slog.Error("set tenant context", "error", err)
		InternalError(w, "login failed")
		return
	}

	var user UserResponse
	var passwordHash string
	err = tx.QueryRow(ctx,
		`SELECT id, email, name, role, password_hash FROM users WHERE email = $1 AND tenant_id = $2`,
		req.Email, DefaultTenantID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &passwordHash)
	if err != nil {
		slog.Warn("login failed", "email", req.Email, "error", err)
		Unauthorized(w, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		Unauthorized(w, "invalid email or password")
		return
	}

	token, err := h.auth.GenerateToken(user.ID, user.Email, user.Role, auth.RolePermissions[user.Role])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}

	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()))
	OK(w, map[string]interface{}{
		"token": token,
		"user":  user,
	})
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" || req.Name == "" {
		BadRequest(w, "email, password, and name are required")
		return
	}
	if len(req.Password) < 8 {
		BadRequest(w, "password must be at least 8 characters")
		return
	}
	if len(req.Password) > 128 {
		BadRequest(w, "password too long (max 128 characters)")
		return
	}
	if len(req.Email) > 255 || len(req.Name) > 128 {
		BadRequest(w, "email or name too long")
		return
	}

	if db.Pool == nil {
		InternalError(w, "database not available")
		return
	}

	ctx := r.Context()

	// 设置租户上下文以绕过 RLS —— 必须在事务中才能让 SET LOCAL 持续生效
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		slog.Error("begin tx for tenant context", "error", err)
		InternalError(w, "registration failed")
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", DefaultTenantID); err != nil {
		slog.Error("set tenant context", "error", err)
		InternalError(w, "registration failed")
		return
	}

	// Check for existing user
	var exists int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE email = $1 AND tenant_id = $2`,
		req.Email, DefaultTenantID,
	).Scan(&exists); err != nil {
		slog.Error("check existing user", "error", err)
		InternalError(w, "registration failed")
		return
	}
	if exists > 0 {
		BadRequest(w, "email already registered")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		InternalError(w, "registration failed")
		return
	}

	// 使用 PostgreSQL 的 gen_random_uuid() 生成 UUID
	var userID string
	err = tx.QueryRow(ctx,
		`INSERT INTO users (id, tenant_id, email, name, password_hash, role, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, 'user', NOW(), NOW())
		 RETURNING id`,
		DefaultTenantID, req.Email, req.Name, string(hash),
	).Scan(&userID)
	if err != nil {
		slog.Error("insert user", "error", err)
		InternalError(w, "registration failed: "+err.Error())
		return
	}

	// 提交事务
	if err := tx.Commit(ctx); err != nil {
		slog.Error("commit tx", "error", err)
		InternalError(w, "registration failed")
		return
	}

	token, err := h.auth.GenerateToken(userID, req.Email, "user", auth.RolePermissions["user"])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}

	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()))
	Created(w, map[string]interface{}{
		"token": token,
		"user":  UserResponse{ID: userID, Email: req.Email, Name: req.Name, Role: "user"},
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// ── JWT 黑名单：将该 token 加入 Redis 黑名单，TTL 等于剩余有效期 ──
	if claims := auth.GetClaims(r.Context()); claims != nil && claims.ID != "" && db.Redis != nil {
		remaining := time.Until(claims.ExpiresAt.Time)
		if remaining > 0 {
			db.Redis.Set(r.Context(), "jwt:blacklist:"+claims.ID, "1", remaining)
		}
	}
	ClearTokenCookie(w)
	OK(w, map[string]string{"message": "logged out"})
}

func (h *AuthHandler) Profile(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, "not authenticated")
		return
	}

	OK(w, map[string]interface{}{
		"user_id": claims.UserID,
		"email":   claims.Email,
		"role":    claims.Role,
		"perms":   claims.Perms,
	})
}

type RefreshRequest struct {
	Token string `json:"token"`
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	// Read token from cookie
	cookie, err := r.Cookie(tokenCookieName)
	if err != nil {
		Unauthorized(w, "not authenticated")
		return
	}

	newToken, err := h.auth.RefreshToken(cookie.Value)
	if err != nil {
		Unauthorized(w, "session expired")
		return
	}

	SetTokenCookie(w, newToken, int(h.cfg.JWTExpiration.Seconds()))
	OK(w, map[string]string{"message": "token refreshed"})
}

func generateID() string {
	return id.NextID()
}
