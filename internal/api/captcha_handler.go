package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/jackc/pgx/v5"
)

// ── 人机验证配置管理 + 登录防滥用栅栏 ──────────────────────
//
// 防滥用双保险：
//  1. 管理员启用验证码后，登录/注册必须携带有效 captcha token；
//  2. 未启用时，同一 IP 连续失败达到阈值会"升级"为强制验证码，
//     继续失败达到硬上限则直接 429 拒绝（Redis 计数，15 分钟窗口）。

// errCaptchaHandled 表示 Enforce 已写响应，调用方应直接 return。
var errCaptchaHandled = errors.New("captcha gate: response already written")

const (
	captchaFailThreshold = 5   // 连续失败 N 次后强制验证码
	captchaHardLimit     = 30  // 连续失败 N 次后直接拒绝
	captchaFailWindow    = 15 * time.Minute
	captchaFailKeyPrefix = "login:fail:"
)

// failCounterStore 抽象登录失败计数存储（生产 Redis，测试内存 fake）。
type failCounterStore interface {
	incr(ctx context.Context, ip string, window time.Duration)
	get(ctx context.Context, ip string) int
	clear(ctx context.Context, ip string)
}

// redisFailCounter 是 Redis 实现：key = login:fail:{ip}，TTL = 窗口期。
type redisFailCounter struct {
	rdb db.RedisClient
}

func (c redisFailCounter) incr(ctx context.Context, ip string, window time.Duration) {
	if c.rdb == nil {
		return
	}
	key := captchaFailKeyPrefix + ip
	if err := c.rdb.Incr(ctx, key).Err(); err != nil {
		slog.Warn("captcha fail counter incr failed", "error", err)
		return
	}
	c.rdb.Expire(ctx, key, window)
}

func (c redisFailCounter) get(ctx context.Context, ip string) int {
	if c.rdb == nil {
		return 0
	}
	n, err := c.rdb.Get(ctx, captchaFailKeyPrefix+ip).Int()
	if err != nil {
		return 0
	}
	return n
}

func (c redisFailCounter) clear(ctx context.Context, ip string) {
	if c.rdb == nil {
		return
	}
	c.rdb.Del(ctx, captchaFailKeyPrefix+ip)
}

// captchaConfigRow 是 ent_captcha_config 的内存形态（secret 保留密文）。
type captchaConfigRow struct {
	Provider  string
	SiteKey   string
	SecretEnc string
	VerifyURL string
	Enabled   bool
}

// CaptchaHandler 提供验证码配置 CRUD、公开配置下发与防滥用栅栏。
type CaptchaHandler struct {
	db       entQuerier
	encKey   []byte
	verifier auth.CaptchaVerifier
	counter  failCounterStore // nil 时失败计数降级跳过（仍有 rlMW 兜底）
}

// NewCaptchaHandler 构造验证码 handler；密钥沿用 SSO 加密密钥。
func NewCaptchaHandler(cfg *config.Config) *CaptchaHandler {
	return &CaptchaHandler{
		db:       pgEntStore{},
		encKey:   auth.LoadOIDCEncryptionKey(),
		verifier: auth.NewHTTPCaptchaVerifier(),
		counter:  redisFailCounter{rdb: db.Redis},
	}
}

// RegisterPublicRoutes 挂载公开路由（无 authMW，供登录页拉取前端组件参数；须套 rlMW）。
func (h *CaptchaHandler) RegisterPublicRoutes(mux *http.ServeMux, rlMW func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/auth/captcha/config", rlMW(http.HandlerFunc(h.PublicConfig)))
}

// RegisterAdminRoutes 挂载管理路由（authMW + RequireEntPerm("sso:manage")）。
func (h *CaptchaHandler) RegisterAdminRoutes(mux *http.ServeMux, authMW func(http.Handler) http.Handler) {
	guard := func(hf http.HandlerFunc) http.Handler {
		return authMW(RequireEntPerm("sso:manage")(hf))
	}
	mux.Handle("GET /v1/ent/captcha/config", guard(h.GetConfig))
	mux.Handle("PUT /v1/ent/captcha/config", guard(h.UpdateConfig))
}

// ── 公开配置下发 ────────────────────────────────────────

// PublicConfig GET /v1/auth/captcha/config
// 仅下发前端渲染验证码组件所需的非敏感字段；未启用/未配置返回 enabled=false。
func (h *CaptchaHandler) PublicConfig(w http.ResponseWriter, r *http.Request) {
	row, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if row == nil || !row.Enabled {
		OK(w, map[string]any{"enabled": false})
		return
	}
	OK(w, map[string]any{
		"enabled":   true,
		"provider":  row.Provider,
		"site_key":  row.SiteKey,
		"verify_url": row.VerifyURL, // custom 前端组件可能需要；不含 secret
	})
}

// ── 管理端 ──────────────────────────────────────────────

// GetConfig GET /v1/ent/captcha/config（secret 脱敏）。
func (h *CaptchaHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	row, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if row == nil {
		OK(w, map[string]any{
			"provider": auth.CaptchaTurnstile,
			"site_key": "",
			"secret":   "",
			"enabled":  false,
		})
		return
	}
	OK(w, map[string]any{
		"provider":   row.Provider,
		"site_key":   row.SiteKey,
		"secret":     maskedSecret,
		"verify_url": row.VerifyURL,
		"enabled":    row.Enabled,
	})
}

type updateCaptchaRequest struct {
	Provider  *string `json:"provider"`
	SiteKey   *string `json:"site_key"`
	Secret    *string `json:"secret"` // 空串/脱敏占位 = 保留原值
	VerifyURL *string `json:"verify_url"`
	Enabled   *bool   `json:"enabled"`
}

// UpdateConfig PUT /v1/ent/captcha/config（单租户单行 upsert）。
func (h *CaptchaHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req updateCaptchaRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}

	existing, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	provider := auth.CaptchaTurnstile
	if existing != nil {
		provider = existing.Provider
	}
	siteKey, verifyURL, enabled := "", "", false
	if existing != nil {
		siteKey, verifyURL, enabled = existing.SiteKey, existing.VerifyURL, existing.Enabled
	}

	if req.Provider != nil {
		provider = strings.TrimSpace(*req.Provider)
	}
	if !auth.IsKnownCaptchaProvider(provider) {
		BadRequest(w, "unknown captcha provider: "+provider)
		return
	}
	if req.SiteKey != nil {
		siteKey = strings.TrimSpace(*req.SiteKey)
	}
	if req.VerifyURL != nil {
		verifyURL = strings.TrimSpace(*req.VerifyURL)
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// secret：新值加密；空串/占位保留原密文
	secretEnc := ""
	if existing != nil {
		secretEnc = existing.SecretEnc
	}
	if req.Secret != nil && *req.Secret != "" && *req.Secret != maskedSecret {
		if h.encKey == nil {
			ServiceUnavailable(w, "captcha is not configured: "+auth.EnvOIDCSecretKey+" missing")
			return
		}
		enc, err := auth.EncryptAESGCM(h.encKey, *req.Secret)
		if err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "encrypt captcha secret failed")
			return
		}
		secretEnc = enc
	}

	// 启用前置校验：site_key（custom 除外）与 secret 必须齐备，fail-loud
	if enabled {
		if secretEnc == "" {
			BadRequest(w, "captcha secret is required before enabling")
			return
		}
		if provider != auth.CaptchaCustom && siteKey == "" {
			BadRequest(w, "captcha site_key is required before enabling")
			return
		}
		if provider == auth.CaptchaCustom && verifyURL == "" {
			BadRequest(w, "verify_url is required for custom captcha provider")
			return
		}
	}
	if len(siteKey) > 256 || len(verifyURL) > 512 {
		BadRequest(w, "field too long")
		return
	}

	if _, err := h.db.Exec(r.Context(),
		`INSERT INTO ent_captcha_config (tenant_id, provider, site_key, secret_enc, verify_url, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id) DO UPDATE SET
		   provider = EXCLUDED.provider, site_key = EXCLUDED.site_key,
		   secret_enc = EXCLUDED.secret_enc, verify_url = EXCLUDED.verify_url,
		   enabled = EXCLUDED.enabled, updated_at = NOW()`,
		db.DefaultTenantID, provider, siteKey, secretEnc, nullString(verifyURL), enabled); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	updated, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if updated == nil {
		// 并发删除配置的极端竞态：fail-loud 而非空指针
		logAndRespond(w, errors.New("captcha config vanished after upsert"),
			http.StatusInternalServerError, "captcha config unavailable")
		return
	}
	OK(w, map[string]any{
		"provider":   updated.Provider,
		"site_key":   updated.SiteKey,
		"secret":     maskedSecret,
		"verify_url": updated.VerifyURL,
		"enabled":    updated.Enabled,
	})
}

// ── 防滥用栅栏（登录/注册调用）─────────────────────────

// Enforce 在登录/注册等敏感接口的凭据校验前执行：
//   - 未启用且未达失败阈值 → nil 放行；
//   - 需要验证码但 token 缺失/校验失败/服务商不可达 → 写响应并返回 errCaptchaHandled；
//   - 达硬上限 → 429。
func (h *CaptchaHandler) Enforce(w http.ResponseWriter, r *http.Request, tok *auth.CaptchaToken) error {
	ip := clientIP(r)

	fails := h.failureCount(r.Context(), ip)
	if fails >= captchaHardLimit {
		db.AuditLog(r.Context(), "", "captcha_block", r.URL.Path, "ip="+ip, r.RemoteAddr, nil)
		TooManyRequests(w)
		return errCaptchaHandled
	}

	row, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return errCaptchaHandled
	}

	required := row != nil && row.Enabled
	if !required && fails >= captchaFailThreshold && row != nil && row.SecretEnc != "" {
		// 未全局启用但已配置 → 失败升级为强制验证码
		required = true
	}

	if !required {
		return nil
	}

	if row == nil || row.SecretEnc == "" {
		// 配置缺失却要求验证 → fail-loud，绝不静默放行
		ServiceUnavailable(w, "captcha is not configured")
		return errCaptchaHandled
	}

	if tok == nil || strings.TrimSpace(tok.Token) == "" {
		JSON(w, http.StatusPreconditionRequired, APIResponse{
			Success: false,
			Error:   "captcha_required",
		})
		return errCaptchaHandled
	}

	secret, err := auth.DecryptAESGCM(h.encKey, row.SecretEnc)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "captcha secret unavailable")
		return errCaptchaHandled
	}

	cfg := &auth.CaptchaConfig{
		Provider:  row.Provider,
		SiteKey:   row.SiteKey,
		Secret:    secret,
		VerifyURL: row.VerifyURL,
	}
	if err := h.verifier.Verify(r.Context(), cfg, tok, ip); err != nil {
		if errors.Is(err, auth.ErrCaptchaFailed) {
			db.AuditLog(r.Context(), "", "captcha_failed", r.URL.Path, "ip="+ip, r.RemoteAddr, nil)
			Forbidden(w, "captcha verification failed")
			return errCaptchaHandled
		}
		// 服务商不可达等系统级错误 → fail-loud 502
		logAndRespond(w, err, http.StatusBadGateway, "captcha provider unavailable")
		return errCaptchaHandled
	}
	return nil
}

// RecordFailure 登录失败后调用：计数 + 窗口续期。
func (h *CaptchaHandler) RecordFailure(ctx context.Context, r *http.Request) {
	if h.counter == nil {
		return
	}
	h.counter.incr(ctx, clientIP(r), captchaFailWindow)
}

// ClearFailures 登录成功后调用：清除失败计数。
func (h *CaptchaHandler) ClearFailures(ctx context.Context, r *http.Request) {
	if h.counter == nil {
		return
	}
	h.counter.clear(ctx, clientIP(r))
}

func (h *CaptchaHandler) failureCount(ctx context.Context, ip string) int {
	if h.counter == nil {
		return 0
	}
	return h.counter.get(ctx, ip)
}

// ── 内部 ────────────────────────────────────────────────

func (h *CaptchaHandler) loadConfig(ctx context.Context) (*captchaConfigRow, error) {
	var row captchaConfigRow
	var verifyURL *string
	err := h.db.QueryRow(ctx,
		`SELECT provider, site_key, secret_enc, verify_url, enabled
		 FROM ent_captcha_config WHERE tenant_id = $1`, db.DefaultTenantID).
		Scan(&row.Provider, &row.SiteKey, &row.SecretEnc, &verifyURL, &row.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if verifyURL != nil {
		row.VerifyURL = *verifyURL
	}
	return &row, nil
}

// clientIP 提取客户端 IP（realIPHeader 中间件已把 RemoteAddr 规整为真实 IP）。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
