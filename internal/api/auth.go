package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/id"
	"golang.org/x/crypto/bcrypt"
)

const tokenCookieName = "minicc_token"

// DefaultTenantID 是默认租户 ID（用于单租户模式），单一来源见 internal/db/seed.go。
const DefaultTenantID = db.DefaultTenantID

type AuthHandler struct {
	auth    *auth.Authenticator
	cfg     *config.Config
	captcha *CaptchaHandler // 可选：启用后人机验证 + 失败升级（nil = 跳过，单测用）
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		auth: auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration),
		cfg:  cfg,
	}
}

// SetCaptchaHandler 注入人机验证栅栏（网关装配时调用）。
func (h *AuthHandler) SetCaptchaHandler(c *CaptchaHandler) {
	h.captcha = c
}

type LoginRequest struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	CaptchaToken  string `json:"captcha_token"`
	CaptchaRandstr string `json:"captcha_randstr"`
}

type UserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

// SetTokenCookie sets the JWT as an HTTP-only secure cookie.
func SetTokenCookie(w http.ResponseWriter, token string, maxAge int, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     tokenCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure, // 生产 HTTPS（COOKIE_SECURE=true）下防止明文传输（S 安全修复）
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func ClearTokenCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     tokenCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
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

	// 人机验证栅栏：启用/失败升级时强制校验；达到硬上限直接 429
	if h.captcha != nil {
		if err := h.captcha.Enforce(w, r, &auth.CaptchaToken{
			Token:   req.CaptchaToken,
			Randstr: req.CaptchaRandstr,
		}); err != nil {
			return
		}
	}

	// No dev bypass — always validate against DB
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
	var tenantID string
	err = tx.QueryRow(ctx,
		`SELECT id, email, name, role, tenant_id, password_hash FROM users WHERE email = $1 AND tenant_id = $2`,
		req.Email, DefaultTenantID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &tenantID, &passwordHash)
	if err != nil {
		slog.Warn("login failed", "email", req.Email, "error", err)
		db.AuditLog(r.Context(), "", "login_failed", "/v1/auth/login", "email="+req.Email, r.RemoteAddr, nil)
		if h.captcha != nil {
			h.captcha.RecordFailure(r.Context(), r)
		}
		Unauthorized(w, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		db.AuditLog(r.Context(), "", "login_failed", "/v1/auth/login", "email="+req.Email, r.RemoteAddr, nil)
		if h.captcha != nil {
			h.captcha.RecordFailure(r.Context(), r)
		}
		Unauthorized(w, "invalid email or password")
		return
	}

	if h.captcha != nil {
		h.captcha.ClearFailures(r.Context(), r)
	}

	// 多租户隔离：将用户记录的 tenant_id 写入 JWT，后续所有 SQL 用 claims.TenantID
	if tenantID == "" {
		tenantID = DefaultTenantID // 单租户兼容：历史数据无 tenant_id 时回退默认租户
	}
	token, err := h.auth.GenerateToken(user.ID, user.Email, user.Role, tenantID, auth.RolePermissions[user.Role])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}

	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()), h.cfg.CookieSecure)
	db.AuditLog(r.Context(), user.ID, "login_success", "/v1/auth/login", "email="+req.Email, r.RemoteAddr, nil)
	OK(w, map[string]interface{}{
		"token": token,
		"user":  user,
	})
}

type RegisterRequest struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	Name           string `json:"name"`
	CaptchaToken   string `json:"captcha_token"`
	CaptchaRandstr string `json:"captcha_randstr"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if h.cfg.DisableRegistration {
		Forbidden(w, "registration is disabled on this instance")
		return
	}

	var req RegisterRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}

	// 注册接口防刷：人机验证栅栏
	if h.captcha != nil {
		if err := h.captcha.Enforce(w, r, &auth.CaptchaToken{
			Token:   req.CaptchaToken,
			Randstr: req.CaptchaRandstr,
		}); err != nil {
			return
		}
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
		logAndRespond(w, err, http.StatusInternalServerError, "registration failed")
		return
	}

	// 提交事务
	if err := tx.Commit(ctx); err != nil {
		slog.Error("commit tx", "error", err)
		InternalError(w, "registration failed")
		return
	}

	token, err := h.auth.GenerateToken(userID, req.Email, "user", DefaultTenantID, auth.RolePermissions["user"])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}

	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()), h.cfg.CookieSecure)
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
	ClearTokenCookie(w, h.cfg.CookieSecure)
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

// UpdateProfile updates the authenticated user's profile (name/email).
func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, "not authenticated")
		return
	}

	var body struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.Email == "" && body.Name == "" {
		BadRequest(w, "no fields to update")
		return
	}

	setClauses := ""
	args := []interface{}{}
	argIdx := 1
	if body.Email != "" {
		setClauses += fmt.Sprintf("email = $%d, ", argIdx)
		args = append(args, body.Email)
		argIdx++
	}
	if body.Name != "" {
		setClauses += fmt.Sprintf("name = $%d, ", argIdx)
		args = append(args, body.Name)
		argIdx++
	}
	setClauses = strings.TrimSuffix(setClauses, ", ")
	args = append(args, claims.UserID)

	if _, err := db.Pool.Exec(r.Context(),
		fmt.Sprintf("UPDATE users SET %s, updated_at = NOW() WHERE id = $%d", setClauses, argIdx),
		args...); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update profile failed")
		return
	}

	OK(w, map[string]string{"status": "updated"})
}

type RefreshRequest struct {
	Token string `json:"token"`
}

// Session GET /v1/auth/session（公开，凭 httpOnly cookie）
// SSO 回调只设置 JWT cookie；前端登录态基于 localStorage Bearer token。
// 本端点把 cookie 会话引导为与 login 相同 shape 的 {token, user} 响应，
// 供 SSO 登录回跳后前端建立本地会话。
func (h *AuthHandler) Session(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(tokenCookieName)
	if err != nil || cookie.Value == "" {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	claims, err := h.auth.ValidateToken(cookie.Value)
	if err != nil || claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}

	ctx := r.Context()
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		slog.Error("begin tx for tenant context", "error", err)
		InternalError(w, "session lookup failed")
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", DefaultTenantID); err != nil {
		slog.Error("set tenant context", "error", err)
		InternalError(w, "session lookup failed")
		return
	}

	var user UserResponse
	if err := tx.QueryRow(ctx,
		`SELECT id, email, name, role FROM users WHERE id = $1 AND tenant_id = $2`,
		claims.UserID, DefaultTenantID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role); err != nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}

	OK(w, map[string]interface{}{
		"token": cookie.Value,
		"user":  user,
	})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(tokenCookieName)
	if err != nil {
		Unauthorized(w, "not authenticated")
		return
	}

	// 1) 校验旧 token 未被加入黑名单（已登出/已撤销）
	oldClaims, err := h.auth.ValidateToken(cookie.Value)
	if err != nil {
		Unauthorized(w, "session expired")
		return
	}
	if oldClaims.ID != "" && db.Redis != nil {
		if n, err := db.Redis.Exists(r.Context(), "jwt:blacklist:"+oldClaims.ID).Result(); err == nil && n > 0 {
			Unauthorized(w, "token revoked")
			return
		}
	}

	// 2) 校验用户仍然存在（防止已删除用户刷 token）
	var userExists bool
	err = db.ReadPool().QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, oldClaims.UserID).Scan(&userExists)
	if err != nil || !userExists {
		Unauthorized(w, "user not found")
		return
	}

	newToken, err := h.auth.RefreshToken(cookie.Value)
	if err != nil {
		Unauthorized(w, "session expired")
		return
	}

	SetTokenCookie(w, newToken, int(h.cfg.JWTExpiration.Seconds()), h.cfg.CookieSecure)
	OK(w, map[string]string{"message": "token refreshed"})
}

func generateID() string {
	return id.NextID()
}
