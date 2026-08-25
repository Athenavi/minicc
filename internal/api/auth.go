package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/id"
	"golang.org/x/crypto/bcrypt"
)

const tokenCookieName = "chiron_token"

// DefaultTenantID 鏄粯璁ょ鎴?ID锛堢敤浜庡崟绉熸埛妯″紡锛夛紝鍗曚竴鏉ユ簮瑙?internal/db/seed.go銆?
const DefaultTenantID = db.DefaultTenantID

type AuthHandler struct {
	auth    *auth.Authenticator
	cfg     *config.Config
	captcha *CaptchaHandler // 鍙€夛細鍚敤鍚庝汉鏈洪獙璇?+ 澶辫触鍗囩骇锛坣il = 璺宠繃锛屽崟娴嬬敤锛?
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		auth: auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration),
		cfg:  cfg,
	}
}

// SetCaptchaHandler 娉ㄥ叆浜烘満楠岃瘉鏍呮爮锛堢綉鍏宠閰嶆椂璋冪敤锛夈€?
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
		Secure:   secure, // 鐢熶骇 HTTPS锛圕OOKIE_SECURE=true锛変笅闃叉鏄庢枃浼犺緭锛圫 瀹夊叏淇锛?
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

	// 浜烘満楠岃瘉鏍呮爮锛氬惎鐢?澶辫触鍗囩骇鏃跺己鍒舵牎楠岋紱杈惧埌纭笂闄愮洿鎺?429
	if h.captcha != nil {
		if err := h.captcha.Enforce(w, r, &auth.CaptchaToken{
			Token:   req.CaptchaToken,
			Randstr: req.CaptchaRandstr,
		}); err != nil {
			return
		}
	}

	// No dev bypass 鈥?always validate against DB
	ctx := r.Context()

	// 璁剧疆绉熸埛涓婁笅鏂囦互缁曡繃 RLS 鈥斺€?蹇呴』鍦ㄤ簨鍔′腑鎵嶈兘璁?SET LOCAL 鎸佺画鐢熸晥
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

	// 澶氱鎴烽殧绂伙細灏嗙敤鎴疯褰曠殑 tenant_id 鍐欏叆 JWT锛屽悗缁墍鏈?SQL 鐢?claims.TenantID
	// P1-5: tenant_id 涓虹┖鐩存帴鎷掔粷鐧诲綍锛屼笉鍐嶅洖閫€ DefaultTenantID銆?
	// 鍘嗗彶鏁版嵁涓?tenant_id=NULL 鐨?user 璧?DefaultTenantID 浼氳惤鍒伴粯璁ょ鎴凤紝
	// 閫犳垚璺ㄧ鎴锋暟鎹闂紱澶氱鎴烽儴缃插繀椤诲己鍒舵瘡涓敤鎴风粦瀹氱鎴枫€?
	if tenantID == "" {
		slog.Warn("login rejected: user has null tenant_id", "user_id", user.ID)
		Unauthorized(w, "user has no tenant binding; contact admin")
		return
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

	// 娉ㄥ唽鎺ュ彛闃插埛锛氫汉鏈洪獙璇佹爡鏍?
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

	// 璁剧疆绉熸埛涓婁笅鏂囦互缁曡繃 RLS 鈥斺€?蹇呴』鍦ㄤ簨鍔′腑鎵嶈兘璁?SET LOCAL 鎸佺画鐢熸晥
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

	// 浣跨敤 PostgreSQL 鐨?gen_random_uuid() 鐢熸垚 UUID
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

	// 鎻愪氦浜嬪姟
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
	// 鈹€鈹€ JWT 榛戝悕鍗曪細灏嗚 token 鍔犲叆 Redis 榛戝悕鍗曪紝TTL 绛変簬鍓╀綑鏈夋晥鏈?鈹€鈹€
	if claims := auth.GetClaims(r.Context()); claims != nil && claims.ID != "" && db.Redis != nil {
		remaining := time.Until(claims.ExpiresAt.Time)
		if remaining > 0 {
			db.Redis.Set(r.Context(), "jwt:blacklist:"+claims.ID, "1", remaining)
		}
	}
	// 鍚屾鏈湴姝ｇ紦瀛橈紝纭繚鏈疄渚嬪悗缁姹傜珛鍗虫嫆缁濊 token锛圥1 浼樺寲锛?
	if claims := auth.GetClaims(r.Context()); claims != nil {
		markJWTBlacklisted(claims.ID)
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

	var settings map[string]interface{}
	if err := db.ReadPool().QueryRow(r.Context(),
		`SELECT COALESCE(settings, '{}'::jsonb) FROM users WHERE id = $1`, claims.UserID).Scan(&settings); err != nil {
		settings = map[string]interface{}{}
	}
	OK(w, map[string]interface{}{
		"user_id":  claims.UserID,
		"email":    claims.Email,
		"role":     claims.Role,
		"perms":    claims.Perms,
		"settings": settings,
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
		Email    string                 `json:"email"`
		Name     string                 `json:"name"`
		Settings map[string]interface{} `json:"settings"` // 鑷畾涔夋崲鑲ょ瓑鐢ㄦ埛璁剧疆锛堝眬閮ㄥ悎骞讹級
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.Email == "" && body.Name == "" && body.Settings == nil {
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
	if body.Settings != nil {
		settingsJSON, err := json.Marshal(body.Settings)
		if err != nil {
			InternalError(w, "invalid settings")
			return
		}
		// 灞€閮ㄥ悎骞讹細settings = settings || $n锛坖sonb 鍚堝苟淇濈暀鏈彁鍙婇敭锛?
		setClauses += fmt.Sprintf("settings = COALESCE(settings, '{}'::jsonb) || $%d::jsonb, ", argIdx)
		args = append(args, string(settingsJSON))
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

// Session GET /v1/auth/session锛堝叕寮€锛屽嚟 httpOnly cookie锛?
// SSO 鍥炶皟鍙缃?JWT cookie锛涘墠绔櫥褰曟€佸熀浜?localStorage Bearer token銆?
// 鏈鐐规妸 cookie 浼氳瘽寮曞涓轰笌 login 鐩稿悓 shape 鐨?{token, user} 鍝嶅簲锛?
// 渚?SSO 鐧诲綍鍥炶烦鍚庡墠绔缓绔嬫湰鍦颁細璇濄€?
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

	// 1) 鏍￠獙鏃?token 鏈鍔犲叆榛戝悕鍗曪紙宸茬櫥鍑?宸叉挙閿€锛?
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

	// 2) 鏍￠獙鐢ㄦ埛浠嶇劧瀛樺湪锛堥槻姝㈠凡鍒犻櫎鐢ㄦ埛鍒?token锛?
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
