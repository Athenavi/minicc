package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/jackc/pgx/v5"
)

// 鈹€鈹€ 浜烘満楠岃瘉閰嶇疆绠＄悊 + 鐧诲綍闃叉互鐢ㄦ爡鏍?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
//
// 闃叉互鐢ㄥ弻淇濋櫓锛?
//  1. 绠＄悊鍛樺惎鐢ㄩ獙璇佺爜鍚庯紝鐧诲綍/娉ㄥ唽蹇呴』鎼哄甫鏈夋晥 captcha token锛?
//  2. 鏈惎鐢ㄦ椂锛屽悓涓€ IP 杩炵画澶辫触杈惧埌闃堝€间細"鍗囩骇"涓哄己鍒堕獙璇佺爜锛?
//     缁х画澶辫触杈惧埌纭笂闄愬垯鐩存帴 429 鎷掔粷锛圧edis 璁℃暟锛?5 鍒嗛挓绐楀彛锛夈€?

// errCaptchaHandled 琛ㄧず Enforce 宸插啓鍝嶅簲锛岃皟鐢ㄦ柟搴旂洿鎺?return銆?
var errCaptchaHandled = errors.New("captcha gate: response already written")

const (
	captchaFailThreshold = 5   // 杩炵画澶辫触 N 娆″悗寮哄埗楠岃瘉鐮?
	captchaHardLimit     = 30  // 杩炵画澶辫触 N 娆″悗鐩存帴鎷掔粷
	captchaFailWindow    = 15 * time.Minute
	captchaFailKeyPrefix = "login:fail:"
)

// failCounterStore 鎶借薄鐧诲綍澶辫触璁℃暟瀛樺偍锛堢敓浜?Redis锛屾祴璇曞唴瀛?fake锛夈€?
type failCounterStore interface {
	incr(ctx context.Context, ip string, window time.Duration)
	get(ctx context.Context, ip string) int
	clear(ctx context.Context, ip string)
}

// redisFailCounter 鏄?Redis 瀹炵幇锛歬ey = login:fail:{ip}锛孴TL = 绐楀彛鏈熴€?
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

// captchaConfigRow 鏄?ent_captcha_config 鐨勫唴瀛樺舰鎬侊紙secret 淇濈暀瀵嗘枃锛夈€?
type captchaConfigRow struct {
	Provider  string
	SiteKey   string
	SecretEnc string
	VerifyURL string
	Enabled   bool
}

// CaptchaHandler 鎻愪緵楠岃瘉鐮侀厤缃?CRUD銆佸叕寮€閰嶇疆涓嬪彂涓庨槻婊ョ敤鏍呮爮銆?
type CaptchaHandler struct {
	db       entQuerier
	encKey   []byte
	verifier auth.CaptchaVerifier
	counter  failCounterStore // nil 鏃跺け璐ヨ鏁伴檷绾ц烦杩囷紙浠嶆湁 rlMW 鍏滃簳锛?
}

// NewCaptchaHandler 鏋勯€犻獙璇佺爜 handler锛涘瘑閽ユ部鐢?SSO 鍔犲瘑瀵嗛挜銆?
func NewCaptchaHandler(cfg *config.Config) *CaptchaHandler {
	return &CaptchaHandler{
		db:       pgEntStore{},
		encKey:   auth.LoadOIDCEncryptionKey(),
		verifier: auth.NewHTTPCaptchaVerifier(),
		counter:  redisFailCounter{rdb: db.Redis},
	}
}

// RegisterPublicRoutes 鎸傝浇鍏紑璺敱锛堟棤 authMW锛屼緵鐧诲綍椤垫媺鍙栧墠绔粍浠跺弬鏁帮紱椤诲 rlMW锛夈€?
func (h *CaptchaHandler) RegisterPublicRoutes(mux *http.ServeMux, rlMW func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/auth/captcha/config", rlMW(http.HandlerFunc(h.PublicConfig)))
}

// RegisterAdminRoutes 鎸傝浇绠＄悊璺敱锛坅uthMW + RequireEntPerm("sso:manage")锛夈€?
func (h *CaptchaHandler) RegisterAdminRoutes(mux *http.ServeMux, authMW func(http.Handler) http.Handler) {
	guard := func(hf http.HandlerFunc) http.Handler {
		return authMW(RequireEntPerm("sso:manage")(hf))
	}
	mux.Handle("GET /v1/ent/captcha/config", guard(h.GetConfig))
	mux.Handle("PUT /v1/ent/captcha/config", guard(h.UpdateConfig))
}

// 鈹€鈹€ 鍏紑閰嶇疆涓嬪彂 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// PublicConfig GET /v1/auth/captcha/config
// 浠呬笅鍙戝墠绔覆鏌撻獙璇佺爜缁勪欢鎵€闇€鐨勯潪鏁忔劅瀛楁锛涙湭鍚敤/鏈厤缃繑鍥?enabled=false銆?
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
		"verify_url": row.VerifyURL, // custom 鍓嶇缁勪欢鍙兘闇€瑕侊紱涓嶅惈 secret
	})
}

// 鈹€鈹€ 绠＄悊绔?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// GetConfig GET /v1/ent/captcha/config锛坰ecret 鑴辨晱锛夈€?
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
	Secret    *string `json:"secret"` // 绌轰覆/鑴辨晱鍗犱綅 = 淇濈暀鍘熷€?
	VerifyURL *string `json:"verify_url"`
	Enabled   *bool   `json:"enabled"`
}

// UpdateConfig PUT /v1/ent/captcha/config锛堝崟绉熸埛鍗曡 upsert锛夈€?
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

	// secret锛氭柊鍊煎姞瀵嗭紱绌轰覆/鍗犱綅淇濈暀鍘熷瘑鏂?
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

	// 鍚敤鍓嶇疆鏍￠獙锛歴ite_key锛坈ustom 闄ゅ锛変笌 secret 蹇呴』榻愬锛宖ail-loud
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
		// 骞跺彂鍒犻櫎閰嶇疆鐨勬瀬绔珵鎬侊細fail-loud 鑰岄潪绌烘寚閽?
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

// 鈹€鈹€ 闃叉互鐢ㄦ爡鏍忥紙鐧诲綍/娉ㄥ唽璋冪敤锛夆攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// Enforce 鍦ㄧ櫥褰?娉ㄥ唽绛夋晱鎰熸帴鍙ｇ殑鍑嵁鏍￠獙鍓嶆墽琛岋細
//   - 鏈惎鐢ㄤ笖鏈揪澶辫触闃堝€?鈫?nil 鏀捐锛?
//   - 闇€瑕侀獙璇佺爜浣?token 缂哄け/鏍￠獙澶辫触/鏈嶅姟鍟嗕笉鍙揪 鈫?鍐欏搷搴斿苟杩斿洖 errCaptchaHandled锛?
//   - 杈剧‖涓婇檺 鈫?429銆?
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
		// 鏈叏灞€鍚敤浣嗗凡閰嶇疆 鈫?澶辫触鍗囩骇涓哄己鍒堕獙璇佺爜
		required = true
	}

	if !required {
		return nil
	}

	if row == nil || row.SecretEnc == "" {
		// 閰嶇疆缂哄け鍗磋姹傞獙璇?鈫?fail-loud锛岀粷涓嶉潤榛樻斁琛?
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
		// 鏈嶅姟鍟嗕笉鍙揪绛夌郴缁熺骇閿欒 鈫?fail-loud 502
		logAndRespond(w, err, http.StatusBadGateway, "captcha provider unavailable")
		return errCaptchaHandled
	}
	return nil
}

// RecordFailure 鐧诲綍澶辫触鍚庤皟鐢細璁℃暟 + 绐楀彛缁湡銆?
func (h *CaptchaHandler) RecordFailure(ctx context.Context, r *http.Request) {
	if h.counter == nil {
		return
	}
	h.counter.incr(ctx, clientIP(r), captchaFailWindow)
}

// ClearFailures 鐧诲綍鎴愬姛鍚庤皟鐢細娓呴櫎澶辫触璁℃暟銆?
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

// 鈹€鈹€ 鍐呴儴 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

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

// clientIP 鎻愬彇瀹㈡埛绔?IP锛坮ealIPHeader 涓棿浠跺凡鎶?RemoteAddr 瑙勬暣涓虹湡瀹?IP锛夈€?
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
