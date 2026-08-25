package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// maskedSecret 鏄?provider 璇诲搷搴斾腑 client_secret 鐨勫敮涓€鑴辨晱褰㈡€併€?
const maskedSecret = "********"

// ssoStateTTL 鏄?SSO state 浠ょ墝鏈夋晥鏈燂紙闃查噸鏀剧獥鍙ｏ級銆?
const ssoStateTTL = 10 * time.Minute

// 鈹€鈹€ 鏁版嵁妯″瀷 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

type ssoProvider struct {
	ID              string
	TenantID        string
	Name            string
	Issuer          string
	ClientID        string
	ClientSecretEnc string
	Scopes          []string
	Enabled         bool
	AutoProvision   bool
	RoleMapping     map[string]string
	// OAuth2 鎵╁睍
	Protocol     string
	ProviderType string
	DisplayName  string
	Icon         string
	SortOrder    int
	AuthURL      string
	TokenURL     string
	UserinfoURL  string
	Extra        map[string]string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ssoUser struct {
	ID    string
	Email string
	Name  string
	Role  string
}

// sanitizeProvider 鏋勯€?provider 鐨勫澶栧搷搴旓細client_secret 涓€寰嬭劚鏁忋€?
func sanitizeProvider(p *ssoProvider) map[string]any {
	roleMapping := p.RoleMapping
	if roleMapping == nil {
		roleMapping = map[string]string{}
	}
	scopes := p.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	extra := p.Extra
	if extra == nil {
		extra = map[string]string{}
	}
	return map[string]any{
		"id":             p.ID,
		"tenant_id":      p.TenantID,
		"name":           p.Name,
		"issuer":         p.Issuer,
		"client_id":      p.ClientID,
		"client_secret":  maskedSecret,
		"scopes":         scopes,
		"enabled":        p.Enabled,
		"auto_provision": p.AutoProvision,
		"role_mapping":   roleMapping,
		"protocol":       p.Protocol,
		"provider_type":  p.ProviderType,
		"display_name":   p.DisplayName,
		"icon":           p.Icon,
		"sort_order":     p.SortOrder,
		"auth_url":       p.AuthURL,
		"token_url":      p.TokenURL,
		"userinfo_url":   p.UserinfoURL,
		"extra":          extra,
		"created_at":     p.CreatedAt,
		"updated_at":     p.UpdatedAt,
	}
}

type scanner interface{ Scan(dest ...any) error }

// 鍙┖鍒楃粺涓€ COALESCE 涓虹┖涓诧細鏈鐩栵紙''锛夊嵆"鐢ㄦā鏉跨己鐪佸€?锛岃涔変笌 OAuth2 绔偣瑕嗙洊涓€鑷淬€?
const ssoProviderColumns = `id, tenant_id, name, COALESCE(issuer, ''), client_id, client_secret_enc,
       scopes, enabled, auto_provision, role_mapping,
       COALESCE(protocol, 'oidc'), COALESCE(provider_type, 'custom'),
       COALESCE(display_name, ''), COALESCE(icon, ''), COALESCE(sort_order, 100),
       COALESCE(auth_url, ''), COALESCE(token_url, ''), COALESCE(userinfo_url, ''),
       extra, created_at, updated_at`

func scanSSOProvider(row scanner) (*ssoProvider, error) {
	var p ssoProvider
	var scopes []string
	var roleMappingRaw []byte
	var extraRaw []byte
	if err := row.Scan(&p.ID, &p.TenantID, &p.Name, &p.Issuer, &p.ClientID, &p.ClientSecretEnc,
		&scopes, &p.Enabled, &p.AutoProvision, &roleMappingRaw,
		&p.Protocol, &p.ProviderType, &p.DisplayName, &p.Icon, &p.SortOrder,
		&p.AuthURL, &p.TokenURL, &p.UserinfoURL, &extraRaw, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.Scopes = scopes
	if p.Scopes == nil {
		p.Scopes = []string{}
	}
	p.RoleMapping = map[string]string{}
	if len(roleMappingRaw) > 0 {
		if err := json.Unmarshal(roleMappingRaw, &p.RoleMapping); err != nil {
			return nil, err
		}
	}
	p.Extra = map[string]string{}
	if len(extraRaw) > 0 {
		if err := json.Unmarshal(extraRaw, &p.Extra); err != nil {
			return nil, err
		}
	}
	if p.Protocol == "" {
		p.Protocol = auth.ProtocolOIDC
	}
	if p.ProviderType == "" {
		p.ProviderType = auth.ProviderCustom
	}
	return &p, nil
}

// 鈹€鈹€ Handler 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// SSOHandler 璐熻矗 OIDC/OAuth2 SSO 鍏紑娴佺▼銆佺敤鎴疯嚜鍔╃粦瀹氫笌绠＄悊绔?CRUD銆?
// db 鎶借薄涓?entQuerier 渚夸簬娴嬭瘯娉ㄥ叆 fake锛沞xchanger 鎶借薄 IdP 浜や簰銆?
type SSOHandler struct {
	auth       *auth.Authenticator
	cfg        *config.Config
	db         entQuerier
	exchanger  auth.OIDCExchanger // OIDC 鍗忚浜ゆ崲鍣紙go-oidc锛?
	oauth2     auth.OIDCExchanger // OAuth2 鍗忚浜ゆ崲鍣紙github/寰俊/閽夐拤/椋炰功/qq锛?
	codec      *auth.StateCodec
	encKey     []byte
	redirectURL string // OAuth2 redirect_uri锛堟巿鏉冨洖璋冨湴鍧€锛?
	successURL  string // 鐧诲綍鎴愬姛鍚?302 鍥炲墠绔殑鐩爣
	bindURL     string // 缁戝畾鎴愬姛鍚?302 鍥炲墠绔殑鐩爣
}

func NewSSOHandler(authenticator *auth.Authenticator, cfg *config.Config) *SSOHandler {
	// 鍓嶇鐙珛閮ㄧ讲锛坉ev 5173 / 鐙珛鍩熷悕锛夋椂 302 鍒扮粷瀵瑰湴鍧€锛涘悓婧愰儴缃蹭繚鎸佺浉瀵硅矾寰?
	feBase := strings.TrimRight(cfg.FrontendURL, "/")
	h := &SSOHandler{
		auth:        authenticator,
		cfg:         cfg,
		db:          pgEntStore{},
		exchanger:   auth.NewRemoteOIDCExchanger(),
		oauth2:      auth.NewOAuth2Exchanger(),
		encKey:      auth.LoadOIDCEncryptionKey(),
		redirectURL: cfg.PublicBaseURL + "/v1/auth/sso/callback",
		successURL:  feBase + "/?sso=ok",
		bindURL:     feBase + "/profile?bind=ok",
	}
	if h.encKey != nil {
		h.codec = auth.NewStateCodec(h.encKey, ssoStateTTL)
	}
	return h
}

// RegisterPublicRoutes 鎸傝浇鍏紑 SSO 璺敱锛堟棤 authMW锛涘灞傞』濂?rlMW 闄愭祦锛夛細
// 鍙戠幇鍒楄〃 / 鐧诲綍璺宠浆 / IdP 鍥炶皟銆俠ind 妯″紡鐨勭櫥褰曟€佷粠 JWT cookie 瑙ｆ瀽銆?
func (h *SSOHandler) RegisterPublicRoutes(mux *http.ServeMux, rlMW func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/auth/sso/providers", rlMW(http.HandlerFunc(h.ListPublicProviders)))
	mux.Handle("GET /v1/auth/sso/login/{providerID}", rlMW(http.HandlerFunc(h.Login)))
	mux.Handle("GET /v1/auth/sso/callback", rlMW(http.HandlerFunc(h.Callback)))
}

// RegisterUserRoutes 鎸傝浇鐢ㄦ埛鑷姪璺敱锛坅uthMW锛夛細缁戝畾鍒楄〃 / 瑙ｇ粦 / 璁剧疆瀵嗙爜銆?
func (h *SSOHandler) RegisterUserRoutes(mux *http.ServeMux, authMW func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/auth/sso/identities", authMW(http.HandlerFunc(h.ListIdentities)))
	mux.Handle("DELETE /v1/auth/sso/identities/{id}", authMW(http.HandlerFunc(h.DeleteIdentity)))
	mux.Handle("POST /v1/auth/password", authMW(http.HandlerFunc(h.SetPassword)))
}

// RegisterAdminRoutes 鎸傝浇 SSO 绠＄悊璺敱锛歛uthMW + RequireEntPerm("sso:manage")銆?
func (h *SSOHandler) RegisterAdminRoutes(mux *http.ServeMux, authMW func(http.Handler) http.Handler) {
	sso := func(hf http.HandlerFunc) http.Handler {
		return authMW(RequireEntPerm("sso:manage")(hf))
	}
	mux.Handle("GET /v1/ent/sso/providers", sso(h.ListProviders))
	mux.Handle("POST /v1/ent/sso/providers", sso(h.CreateProvider))
	mux.Handle("GET /v1/ent/sso/providers/{id}", sso(h.GetProvider))
	mux.Handle("PUT /v1/ent/sso/providers/{id}", sso(h.UpdateProvider))
	mux.Handle("DELETE /v1/ent/sso/providers/{id}", sso(h.DeleteProvider))
}

// 鈹€鈹€ 鍏紑璺敱 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// ListPublicProviders GET /v1/auth/sso/providers
// 杩斿洖 enabled provider 鐨勫睍绀哄瓧娈碉紙涓嶅惈浠讳綍鏁忔劅淇℃伅锛夛紝鎸?sort_order 鎺掑簭銆?
func (h *SSOHandler) ListPublicProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		`SELECT id, name, display_name, provider_type, icon, sort_order, protocol
		 FROM ent_oidc_providers WHERE enabled = TRUE ORDER BY sort_order, name`)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, providerType, protocol string
		var displayName, icon *string
		var sortOrder int
		if err := rows.Scan(&id, &name, &displayName, &providerType, &icon, &sortOrder, &protocol); err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
			return
		}
		display := name
		if displayName != nil && *displayName != "" {
			display = *displayName
		}
		iconKey := providerType
		if icon != nil && *icon != "" {
			iconKey = *icon
		}
		items = append(items, map[string]any{
			"id":            id,
			"name":          name,
			"display_name":  display,
			"provider_type": providerType,
			"icon":          iconKey,
			"sort_order":    sortOrder,
			"protocol":      protocol,
		})
	}
	if err := rows.Err(); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	OK(w, items)
}

// Login GET /v1/auth/sso/login/{providerID}[?mode=bind]
// 鐢熸垚 state+nonce 鈫?302 鍒?IdP 鎺堟潈椤点€?
// mode=bind 闇€鐧诲綍鎬侊紙authMW 淇濊瘉锛夛紝uid 鍐欏叆 HMAC 绛惧悕 state銆?
func (h *SSOHandler) Login(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("providerID")
	if !isValidUUID(providerID) {
		BadRequest(w, "invalid provider id")
		return
	}
	if h.encKey == nil || h.codec == nil {
		ServiceUnavailable(w, "SSO is not configured: "+auth.EnvOIDCSecretKey+" missing")
		return
	}

	provider, err := h.getEnabledProvider(r.Context(), providerID)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, "provider not found or disabled")
		return
	}
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	clientSecret, err := auth.DecryptAESGCM(h.encKey, provider.ClientSecretEnc)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "sso provider secret unavailable")
		return
	}

	// bind 妯″紡锛氬繀椤绘惡甯︾櫥褰曟€侊紙浠?JWT cookie 瑙ｆ瀽锛屽叕寮€璺敱鏃?authMW锛?
	mode := auth.StateModeLogin
	uid := ""
	if r.URL.Query().Get("mode") == auth.StateModeBind {
		claims := auth.GetClaims(r.Context())
		if claims == nil {
			if c, cookieErr := r.Cookie(tokenCookieName); cookieErr == nil && c.Value != "" {
				if parsed, tokenErr := h.auth.ValidateToken(c.Value); tokenErr == nil {
					claims = parsed
				}
			}
		}
		if claims == nil || claims.UserID == "" {
			Unauthorized(w, ErrAuthRequired)
			return
		}
		mode = auth.StateModeBind
		uid = claims.UserID
	}

	nonce, err := auth.RandomNonce()
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "sso login failed")
		return
	}
	state, err := h.codec.IssueMode(providerID, nonce, mode, uid)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "sso login failed")
		return
	}

	exchanger, cfg, err := h.exchangerFor(provider, clientSecret)
	if err != nil {
		BadRequest(w, err.Error())
		return
	}
	authURL, err := exchanger.AuthURL(r.Context(), cfg, state, nonce)
	if err != nil {
		logAndRespond(w, err, http.StatusBadGateway, "sso provider unreachable")
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback GET /v1/auth/sso/callback?code=&state=
// 鏍￠獙 state 鈫?鎺堟潈鐮佹崲韬唤 鈫?鐧诲綍缁戝畾/鑷姩寤哄彿锛堟垨 bind 妯″紡缁戝畾褰撳墠璐﹀彿锛夆啋 棰佸彂浼氳瘽銆?
func (h *SSOHandler) Callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		BadRequest(w, "missing code or state")
		return
	}
	if h.encKey == nil || h.codec == nil {
		ServiceUnavailable(w, "SSO is not configured: "+auth.EnvOIDCSecretKey+" missing")
		return
	}

	// 1. state 鏍￠獙锛圚MAC 闃?CSRF/绡℃敼锛孴TL 闃查噸鏀撅級鈥斺€斿け璐ヤ竴寰?400锛屼笖涓嶈Е纰?DB
	payload, err := h.codec.Verify(state)
	if err != nil {
		logAndRespond(w, err, http.StatusBadRequest, "invalid sso state")
		return
	}

	// 2. 鍔犺浇 provider锛堝繀椤?enabled锛?
	provider, err := h.getEnabledProvider(r.Context(), payload.ProviderID)
	if errors.Is(err, pgx.ErrNoRows) {
		BadRequest(w, "unknown sso provider in state")
		return
	}
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	clientSecret, err := auth.DecryptAESGCM(h.encKey, provider.ClientSecretEnc)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "sso provider secret unavailable")
		return
	}

	// 3. 鎺堟潈鐮佷氦鎹?+ 韬唤鏍￠獙锛圤IDC 鍚?nonce 姣斿锛汷Auth2 鐢?state HMAC 闃查噸鏀撅級
	exchanger, cfg, err := h.exchangerFor(provider, clientSecret)
	if err != nil {
		BadRequest(w, err.Error())
		return
	}
	idToken, err := exchanger.ExchangeAndVerify(r.Context(), cfg, code, payload.Nonce)
	if err != nil {
		logAndRespond(w, err, http.StatusUnauthorized, "sso authentication failed")
		return
	}
	if idToken.Subject == "" {
		BadRequest(w, "id_token missing subject")
		return
	}

	// 3.5 bind 妯″紡锛氭妸璇ヤ笁鏂硅韩浠界粦瀹氬埌 state.uid 璐﹀彿
	if payload.Mode == auth.StateModeBind {
		h.handleBindCallback(w, r, provider, idToken, payload)
		return
	}

	// 4. 鎸?(provider_id, subject) 鏌ョ粦瀹?
	user, err := h.findIdentityUser(r.Context(), provider.ID, idToken.Subject)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	// 5. 鏈粦瀹氾細auto_provision 鍐冲畾寤哄彿鎴栨嫆缁?
	if errors.Is(err, pgx.ErrNoRows) {
		if !provider.AutoProvision {
			logAndRespond(w, errors.New("sso subject not bound and auto_provision disabled"),
				http.StatusForbidden, "account not provisioned for this SSO provider")
			return
		}
		user, err = h.provisionAndBind(r, provider, idToken)
		if err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "sso provisioning failed")
			return
		}
	}

	// 6. 澶嶇敤鐜版湁鐧诲綍鐨勪細璇濋鍙戦€昏緫锛圙enerateToken + SetTokenCookie锛?	// 澶氱鎴烽殧绂伙細SSO 鐢ㄦ埛鐨?tenant_id 鏉ヨ嚜 sso_provider 琛ㄩ厤缃紙ent_oidc_providers.tenant_id锛夛紝
	// 涓庤嚜鍔ㄥ缓鍙?provisionAndBind 浣跨敤鐨?provider.TenantID 涓€鑷达紝淇濊瘉 JWT 涓?DB 琛屼负瀵归綈
	if provider.TenantID == "" {
		InternalError(w, "sso provider missing tenant_id configuration")
		return
	}
	token, err := h.auth.GenerateToken(user.ID, user.Email, user.Role, provider.TenantID, auth.RolePermissions[user.Role])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}
	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()), h.cfg.CookieSecure)
	db.AuditLog(r.Context(), user.ID, "sso_login", "/v1/auth/sso/callback", "provider="+provider.Name, r.RemoteAddr, nil)
	http.Redirect(w, r, h.successURL, http.StatusFound)
}

// handleBindCallback bind 妯″紡鍥炶皟锛氳涓夋柟韬唤 鈫?state.uid 鏈湴璐﹀彿銆?
// 鍐茬獊锛堝凡缁戝畾浠栦汉锛夆啋 409锛涘凡缁戝畾鏈汉 鈫?骞傜瓑鎴愬姛銆?
func (h *SSOHandler) handleBindCallback(w http.ResponseWriter, r *http.Request, provider *ssoProvider, idToken *auth.IDTokenResult, payload *auth.StatePayload) {
	ctx := r.Context()

	existing, err := h.findIdentityUser(ctx, provider.ID, idToken.Subject)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if existing != nil {
		if existing.ID != payload.UID {
			db.AuditLog(ctx, payload.UID, "sso_bind_conflict", "/v1/auth/sso/callback",
				"provider="+provider.Name, r.RemoteAddr, nil)
			JSON(w, http.StatusConflict, APIResponse{
				Success: false,
				Error:   "璇ヤ笁鏂硅处鍙峰凡缁戝畾鍏朵粬鐢ㄦ埛锛岃鏀圭敤鍏剁櫥褰?,
			})
			return
		}
		// 宸茬粦瀹氭湰浜?鈫?骞傜瓑
		http.Redirect(w, r, h.bindURL, http.StatusFound)
		return
	}

	if _, err := h.db.Exec(ctx,
		`INSERT INTO ent_user_identities (user_id, provider_id, subject, email)
		 VALUES ($1, $2, $3, $4)`,
		payload.UID, provider.ID, idToken.Subject, idToken.Email); err != nil {
		if isUniqueViolation(err) {
			// 骞跺彂缁戝畾鍐茬獊
			JSON(w, http.StatusConflict, APIResponse{
				Success: false,
				Error:   "璇ヤ笁鏂硅处鍙峰凡缁戝畾鍏朵粬鐢ㄦ埛锛岃鏀圭敤鍏剁櫥褰?,
			})
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	db.AuditLog(ctx, payload.UID, "sso_bind", "/v1/auth/sso/callback",
		"provider="+provider.Name, r.RemoteAddr, nil)
	http.Redirect(w, r, h.bindURL, http.StatusFound)
}

// 鈹€鈹€ 鐢ㄦ埛鑷姪锛氱粦瀹氬垪琛?/ 瑙ｇ粦 / 璁剧疆瀵嗙爜 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// ListIdentities GET /v1/auth/sso/identities锛坅uthMW锛?
func (h *SSOHandler) ListIdentities(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	rows, err := h.db.Query(r.Context(),
		`SELECT ui.id, p.name, COALESCE(p.display_name, p.name), p.provider_type,
		        ui.subject, ui.email, ui.created_at
		 FROM ent_user_identities ui
		 JOIN ent_oidc_providers p ON p.id = ui.provider_id
		 WHERE ui.user_id = $1
		 ORDER BY ui.created_at`, claims.UserID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, display, providerType, subject string
		var email *string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &display, &providerType, &subject, &email, &createdAt); err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
			return
		}
		emailStr := ""
		if email != nil {
			emailStr = *email
		}
		items = append(items, map[string]any{
			"id":            id,
			"provider_name": display,
			"provider_type": providerType,
			"subject":       subject,
			"email":         emailStr,
			"created_at":    createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	OK(w, items)
}

// DeleteIdentity DELETE /v1/auth/sso/identities/{id}锛坅uthMW锛?
// 瀹堝崼锛氱敤鎴锋棤鍙彛浠ゅ瘑鐮侊紙password_set=false锛変笖杩欐槸鏈€鍚庝竴涓笁鏂硅韩浠?鈫?403銆?
func (h *SSOHandler) DeleteIdentity(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	id := r.PathValue("id")
	if !isValidUUID(id) {
		BadRequest(w, "invalid identity id")
		return
	}
	ctx := r.Context()

	// 瀹堝崼锛氱‘淇濊В缁戝悗鑷冲皯淇濈暀涓€绉嶇櫥褰曟柟寮?
	var passwordSet bool
	var count int
	err := h.db.QueryRow(ctx,
		`SELECT u.password_set,
		        (SELECT COUNT(*) FROM ent_user_identities ui WHERE ui.user_id = u.id)
		 FROM users u WHERE u.id = $1`, claims.UserID).Scan(&passwordSet, &count)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, ErrNotFound)
		return
	}
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	// 鐩爣韬唤灞炰簬鏈汉锛?
	var subject string
	err = h.db.QueryRow(ctx,
		`SELECT subject FROM ent_user_identities WHERE id = $1 AND user_id = $2`,
		id, claims.UserID).Scan(&subject)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, ErrNotFound)
		return
	}
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	if !passwordSet && count <= 1 {
		Forbidden(w, "鏃犳硶瑙ｇ粦锛氳鍏堣缃瘑鐮侊紝淇濊瘉鑷冲皯淇濈暀涓€绉嶇櫥褰曟柟寮?)
		return
	}

	tag, err := h.db.Exec(ctx,
		`DELETE FROM ent_user_identities WHERE id = $1 AND user_id = $2`, id, claims.UserID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if tag.RowsAffected() == 0 {
		NotFound(w, ErrNotFound)
		return
	}
	db.AuditLog(ctx, claims.UserID, "sso_unbind", "/v1/auth/sso/identities/"+id,
		"subject="+subject, r.RemoteAddr, nil)
	OK(w, map[string]string{"status": "deleted"})
}

type setPasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// SetPassword POST /v1/auth/password锛坅uthMW锛?
// SSO 寤哄彿鐢ㄦ埛锛坧assword_set=false锛夐璁惧瘑鐮佸厤鏃у瘑鐮侊紱宸茶缃€呭繀椤绘牎楠屾棫瀵嗙爜銆?
func (h *SSOHandler) SetPassword(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	var req setPasswordRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if len(req.NewPassword) < 8 || len(req.NewPassword) > 128 {
		BadRequest(w, "password must be 8-128 characters")
		return
	}
	ctx := r.Context()

	var passwordHash string
	var passwordSet bool
	err := h.db.QueryRow(ctx,
		`SELECT password_hash, password_set FROM users WHERE id = $1`, claims.UserID).
		Scan(&passwordHash, &passwordSet)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, ErrNotFound)
		return
	}
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	if passwordSet {
		if req.CurrentPassword == "" {
			BadRequest(w, "current_password is required")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.CurrentPassword)); err != nil {
			Unauthorized(w, "invalid current password")
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		InternalError(w, "set password failed")
		return
	}
	if _, err := h.db.Exec(ctx,
		`UPDATE users SET password_hash = $2, password_set = TRUE, updated_at = NOW() WHERE id = $1`,
		claims.UserID, string(hash)); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	db.AuditLog(ctx, claims.UserID, "password_set", "/v1/auth/password", "", r.RemoteAddr, nil)
	OK(w, map[string]string{"status": "updated"})
}

// 鈹€鈹€ 绠＄悊璺敱锛坅uthMW + sso:manage锛夆攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// ListProviders GET /v1/ent/sso/providers
func (h *SSOHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		`SELECT `+ssoProviderColumns+` FROM ent_oidc_providers ORDER BY sort_order, created_at`)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		provider, err := scanSSOProvider(rows)
		if err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
			return
		}
		items = append(items, sanitizeProvider(provider))
	}
	if err := rows.Err(); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	OK(w, items)
}

type createProviderRequest struct {
	Name          string            `json:"name"`
	Issuer        string            `json:"issuer"`
	ClientID      string            `json:"client_id"`
	ClientSecret  string            `json:"client_secret"`
	Scopes        []string          `json:"scopes"`
	Enabled       *bool             `json:"enabled"`
	AutoProvision *bool             `json:"auto_provision"`
	RoleMapping   map[string]string `json:"role_mapping"`
	// OAuth2 鎵╁睍
	Protocol     string            `json:"protocol"`
	ProviderType string            `json:"provider_type"`
	DisplayName  string            `json:"display_name"`
	Icon         string            `json:"icon"`
	SortOrder    *int              `json:"sort_order"`
	AuthURL      string            `json:"auth_url"`
	TokenURL     string            `json:"token_url"`
	UserinfoURL  string            `json:"userinfo_url"`
	Extra        map[string]string `json:"extra"`
}

// normalizeProviderInput 鏍￠獙骞惰ˉ榻?provider 鍗忚瀛楁锛堟ā鏉跨鐐硅嚜鍔ㄥ～鍏咃級銆?
// 杩斿洖瑙勮寖鍖栫殑 issuer/protocol/providerType/authURL/tokenURL/userinfoURL/scopes銆?
func normalizeProviderInput(protocol, providerType, issuer, authURL, tokenURL, userinfoURL string, scopes []string) (
	string, string, string, string, string, string, []string, error) {
	// protocol 鏈樉寮忔寚瀹氭椂鎸?provider_type 妯℃澘鎺ㄦ柇锛坓ithub/wechat 绛夊師鐢?OAuth2锛?
	if protocol == "" {
		if profile, ok := auth.GetProviderProfile(providerType); ok {
			protocol = profile.Protocol
		} else {
			protocol = auth.ProtocolOIDC
		}
	}
	providerType = defaultStr(providerType, auth.ProviderCustom)
	if !auth.ValidProtocol(protocol) {
		return "", "", "", "", "", "", nil, errors.New("protocol must be oidc or oauth2")
	}
	if !auth.IsKnownProviderType(providerType) {
		return "", "", "", "", "", "", nil, errors.New("unknown provider_type: " + providerType)
	}
	issuer, authURL, tokenURL, userinfoURL = auth.ResolveEndpoints(providerType, issuer, authURL, tokenURL, userinfoURL)
	if len(scopes) == 0 {
		scopes = auth.DefaultScopes(providerType)
	}
	return issuer, protocol, providerType, authURL, tokenURL, userinfoURL, scopes, nil
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// CreateProvider POST /v1/ent/sso/providers
// client_secret 浠?AES-GCM 鍔犲瘑鍏ュ簱锛涘瘑閽ユ湭閰嶇疆鏃惰繑鍥?503銆?
func (h *SSOHandler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	var req createProviderRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if req.Name == "" || req.ClientID == "" || req.ClientSecret == "" {
		BadRequest(w, "name, client_id and client_secret are required")
		return
	}
	if len(req.Name) > 64 || len(req.Issuer) > 512 || len(req.ClientID) > 256 ||
		len(req.DisplayName) > 64 || len(req.Icon) > 64 ||
		len(req.AuthURL) > 512 || len(req.TokenURL) > 512 || len(req.UserinfoURL) > 512 {
		BadRequest(w, "field too long")
		return
	}
	if h.encKey == nil {
		ServiceUnavailable(w, "SSO is not configured: "+auth.EnvOIDCSecretKey+" missing")
		return
	}

	issuer, protocol, providerType, authURL, tokenURL, userinfoURL, scopes, err :=
		normalizeProviderInput(req.Protocol, req.ProviderType, req.Issuer, req.AuthURL, req.TokenURL, req.UserinfoURL, req.Scopes)
	if err != nil {
		BadRequest(w, err.Error())
		return
	}
	if protocol == auth.ProtocolOIDC && issuer == "" {
		BadRequest(w, "issuer is required for oidc providers")
		return
	}

	secretEnc, err := auth.EncryptAESGCM(h.encKey, req.ClientSecret)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "encrypt client_secret failed")
		return
	}

	enabled, autoProvision := true, true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.AutoProvision != nil {
		autoProvision = *req.AutoProvision
	}
	sortOrder := 100
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	roleMapping := req.RoleMapping
	if roleMapping == nil {
		roleMapping = map[string]string{}
	}
	extra := req.Extra
	if extra == nil {
		extra = map[string]string{}
	}
	mappingJSON, err := json.Marshal(roleMapping)
	if err != nil {
		BadRequest(w, "invalid role_mapping")
		return
	}
	extraJSON, err := json.Marshal(extra)
	if err != nil {
		BadRequest(w, "invalid extra")
		return
	}

	var id string
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO ent_oidc_providers
		   (tenant_id, name, issuer, client_id, client_secret_enc, scopes, enabled, auto_provision, role_mapping,
		    protocol, provider_type, display_name, icon, sort_order, auth_url, token_url, userinfo_url, extra)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18) RETURNING id`,
		db.DefaultTenantID, req.Name, nullString(issuer), req.ClientID, secretEnc,
		scopes, enabled, autoProvision, mappingJSON,
		protocol, providerType, nullString(req.DisplayName), nullString(req.Icon), sortOrder,
		nullString(authURL), nullString(tokenURL), nullString(userinfoURL), extraJSON).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			logAndRespond(w, err, http.StatusConflict, "provider name already exists")
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	provider, err := h.getProvider(r.Context(), id)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	Created(w, sanitizeProvider(provider))
}

// GetProvider GET /v1/ent/sso/providers/{id}
func (h *SSOHandler) GetProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidUUID(id) {
		BadRequest(w, "invalid provider id")
		return
	}
	provider, err := h.getProvider(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, ErrNotFound)
		return
	}
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	OK(w, sanitizeProvider(provider))
}

type updateProviderRequest struct {
	Name          *string            `json:"name"`
	Issuer        *string            `json:"issuer"`
	ClientID      *string            `json:"client_id"`
	ClientSecret  *string            `json:"client_secret"`
	Scopes        *[]string          `json:"scopes"`
	Enabled       *bool              `json:"enabled"`
	AutoProvision *bool              `json:"auto_provision"`
	RoleMapping   *map[string]string `json:"role_mapping"`
	// OAuth2 鎵╁睍
	Protocol     *string            `json:"protocol"`
	ProviderType *string            `json:"provider_type"`
	DisplayName  *string            `json:"display_name"`
	Icon         *string            `json:"icon"`
	SortOrder    *int               `json:"sort_order"`
	AuthURL      *string            `json:"auth_url"`
	TokenURL     *string            `json:"token_url"`
	UserinfoURL  *string            `json:"userinfo_url"`
	Extra        *map[string]string `json:"extra"`
}

// UpdateProvider PUT /v1/ent/sso/providers/{id}
// client_secret 涓虹┖涓叉垨鑴辨晱鍗犱綅绗︽椂淇濈暀鍘熷瘑鏂囷紱鎻愪緵鏂板€兼椂閲嶆柊鍔犲瘑锛堥渶瀵嗛挜锛夈€?
func (h *SSOHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidUUID(id) {
		BadRequest(w, "invalid provider id")
		return
	}
	var req updateProviderRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	ctx := r.Context()

	existing, err := h.getProvider(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, ErrNotFound)
		return
	}
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	name, issuer, clientID := existing.Name, existing.Issuer, existing.ClientID
	secretEnc, scopes := existing.ClientSecretEnc, existing.Scopes
	enabled, autoProvision := existing.Enabled, existing.AutoProvision
	mapping := existing.RoleMapping
	protocol, providerType := existing.Protocol, existing.ProviderType
	displayName, icon, sortOrder := existing.DisplayName, existing.Icon, existing.SortOrder
	authURL, tokenURL, userinfoURL := existing.AuthURL, existing.TokenURL, existing.UserinfoURL
	extra := existing.Extra

	if req.Name != nil {
		if *req.Name == "" || len(*req.Name) > 64 {
			BadRequest(w, "invalid name")
			return
		}
		name = *req.Name
	}
	if req.Protocol != nil {
		protocol = *req.Protocol
	}
	if req.ProviderType != nil {
		providerType = *req.ProviderType
	}
	// 鍗忚/妯℃澘鍙樻洿鍚庨噸鏂拌В鏋愮鐐癸紙鏄惧紡瑕嗙洊浼樺厛锛?
	if req.Protocol != nil || req.ProviderType != nil || req.Issuer != nil ||
		req.AuthURL != nil || req.TokenURL != nil || req.UserinfoURL != nil {
		var iss, au, tu, uu string
		if req.Issuer != nil {
			iss = *req.Issuer
		}
		if req.AuthURL != nil {
			au = *req.AuthURL
		}
		if req.TokenURL != nil {
			tu = *req.TokenURL
		}
		if req.UserinfoURL != nil {
			uu = *req.UserinfoURL
		}
		// 鏈樉寮忚鐩栫殑瀛楁娌跨敤鏃㈡湁鍊硷紙鍙兘宸叉槸绠＄悊鍛樿嚜瀹氫箟绔偣锛?
		if iss == "" {
			iss = issuer
		}
		if au == "" {
			au = authURL
		}
		if tu == "" {
			tu = tokenURL
		}
		if uu == "" {
			uu = userinfoURL
		}
		issuer, protocol, providerType, authURL, tokenURL, userinfoURL, _, err =
			normalizeProviderInput(protocol, providerType, iss, au, tu, uu, scopes)
		if err != nil {
			BadRequest(w, err.Error())
			return
		}
	}
	if protocol == auth.ProtocolOIDC && issuer == "" {
		BadRequest(w, "issuer is required for oidc providers")
		return
	}
	if req.DisplayName != nil {
		displayName = *req.DisplayName
		if len(displayName) > 64 {
			BadRequest(w, "invalid display_name")
			return
		}
	}
	if req.Icon != nil {
		icon = *req.Icon
		if len(icon) > 64 {
			BadRequest(w, "invalid icon")
			return
		}
	}
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	if req.ClientID != nil {
		if *req.ClientID == "" || len(*req.ClientID) > 256 {
			BadRequest(w, "invalid client_id")
			return
		}
		clientID = *req.ClientID
	}
	if req.ClientSecret != nil && *req.ClientSecret != "" && *req.ClientSecret != maskedSecret {
		if h.encKey == nil {
			ServiceUnavailable(w, "SSO is not configured: "+auth.EnvOIDCSecretKey+" missing")
			return
		}
		secretEnc, err = auth.EncryptAESGCM(h.encKey, *req.ClientSecret)
		if err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "encrypt client_secret failed")
			return
		}
	}
	if req.Scopes != nil && len(*req.Scopes) > 0 {
		scopes = *req.Scopes
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.AutoProvision != nil {
		autoProvision = *req.AutoProvision
	}
	if req.Extra != nil {
		extra = *req.Extra
		if extra == nil {
			extra = map[string]string{}
		}
	}
	var mappingJSON, extraJSON []byte
	if req.RoleMapping != nil {
		mapping = *req.RoleMapping
		if mapping == nil {
			mapping = map[string]string{}
		}
	}
	mappingJSON, err = json.Marshal(mapping)
	if err != nil {
		BadRequest(w, "invalid role_mapping")
		return
	}
	extraJSON, err = json.Marshal(extra)
	if err != nil {
		BadRequest(w, "invalid extra")
		return
	}

	if _, err := h.db.Exec(ctx,
		`UPDATE ent_oidc_providers
		 SET name = $2, issuer = $3, client_id = $4, client_secret_enc = $5,
		     scopes = $6, enabled = $7, auto_provision = $8, role_mapping = $9,
		     protocol = $10, provider_type = $11, display_name = $12, icon = $13, sort_order = $14,
		     auth_url = $15, token_url = $16, userinfo_url = $17, extra = $18,
		     updated_at = NOW()
		 WHERE id = $1`,
		id, name, nullString(issuer), clientID, secretEnc, scopes, enabled, autoProvision, mappingJSON,
		protocol, providerType, nullString(displayName), nullString(icon), sortOrder,
		nullString(authURL), nullString(tokenURL), nullString(userinfoURL), extraJSON); err != nil {
		if isUniqueViolation(err) {
			logAndRespond(w, err, http.StatusConflict, "provider name already exists")
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	updated, err := h.getProvider(ctx, id)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	OK(w, sanitizeProvider(updated))
}

// DeleteProvider DELETE /v1/ent/sso/providers/{id}
func (h *SSOHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isValidUUID(id) {
		BadRequest(w, "invalid provider id")
		return
	}
	tag, err := h.db.Exec(r.Context(), `DELETE FROM ent_oidc_providers WHERE id = $1`, id)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if tag.RowsAffected() == 0 {
		NotFound(w, ErrNotFound)
		return
	}
	OK(w, map[string]string{"status": "deleted"})
}

// 鈹€鈹€ 鍐呴儴杈呭姪 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func (h *SSOHandler) getProvider(ctx context.Context, id string) (*ssoProvider, error) {
	return scanSSOProvider(h.db.QueryRow(ctx,
		`SELECT `+ssoProviderColumns+` FROM ent_oidc_providers WHERE id = $1`, id))
}

func (h *SSOHandler) getEnabledProvider(ctx context.Context, id string) (*ssoProvider, error) {
	return scanSSOProvider(h.db.QueryRow(ctx,
		`SELECT `+ssoProviderColumns+` FROM ent_oidc_providers WHERE id = $1 AND enabled = TRUE`, id))
}

// exchangerFor 鎸?provider 鍗忚閫夋嫨浜ゆ崲鍣ㄥ苟鏋勯€犲畬鏁撮厤缃紙鍚ā鏉跨鐐硅В鏋愶級銆?
func (h *SSOHandler) exchangerFor(p *ssoProvider, clientSecret string) (auth.OIDCExchanger, *auth.OIDCProviderConfig, error) {
	protocol := p.Protocol
	if protocol == "" {
		protocol = auth.ProtocolOIDC
	}
	// 妯℃澘绔偣琛ラ綈锛氭樉寮忚鐩栵紙DB 鍒楅潪绌猴級浼樺厛锛屽叾娆?provider 妯℃澘缂虹渷鍊?
	issuer, authURL, tokenURL, userinfoURL := auth.ResolveEndpoints(
		p.ProviderType, p.Issuer, p.AuthURL, p.TokenURL, p.UserinfoURL)
	scopes := p.Scopes
	if len(scopes) == 0 {
		scopes = auth.DefaultScopes(p.ProviderType)
	}

	cfg := &auth.OIDCProviderConfig{
		Issuer:       issuer,
		ClientID:     p.ClientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
		RedirectURL:  h.redirectURL,
		Protocol:     protocol,
		ProviderType: p.ProviderType,
		AuthURL:      authURL,
		TokenURL:     tokenURL,
		UserinfoURL:  userinfoURL,
		Extra:        p.Extra,
	}

	if protocol == auth.ProtocolOAuth2 {
		if h.oauth2 == nil {
			return nil, nil, errors.New("oauth2 exchanger not configured")
		}
		if cfg.AuthURL == "" || cfg.TokenURL == "" || cfg.UserinfoURL == "" {
			return nil, nil, errors.New("oauth2 provider missing endpoints (auth_url/token_url/userinfo_url)")
		}
		return h.oauth2, cfg, nil
	}
	if cfg.Issuer == "" {
		return nil, nil, errors.New("oidc provider missing issuer")
	}
	return h.exchanger, cfg, nil
}

// findIdentityUser 鎸?(provider_id, subject) 鏌ョ粦瀹氱敤鎴凤紱鏈粦瀹氳繑鍥?pgx.ErrNoRows銆?
func (h *SSOHandler) findIdentityUser(ctx context.Context, providerID, subject string) (*ssoUser, error) {
	var user ssoUser
	err := h.db.QueryRow(ctx,
		`SELECT u.id, u.email, u.name, u.role
		 FROM ent_user_identities ui JOIN users u ON u.id = ui.user_id
		 WHERE ui.provider_id = $1 AND ui.subject = $2`, providerID, subject).
		Scan(&user.ID, &user.Email, &user.Name, &user.Role)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// provisionAndBind 鑷姩寤哄彿骞跺啓鍏ュ閮ㄨ韩浠界粦瀹氥€?
// 绉熸埛鍙?provider 閰嶇疆锛沞mail 鍙栬韩浠戒俊鎭紙缂哄け鏃朵互 subject 鍏滃簳锛夛紱
// password_hash 鍐欏叆闅忔満涓嶅彲纰版挒 bcrypt 涓?password_set=FALSE锛堜笉鍙彛浠ょ櫥褰曪級锛?
// role 鎸?role_mapping 鍖归厤銆佺己鐪?"user"锛涙惡甯︽墜鏈哄彿鏃跺洖濉?users.phone銆?
func (h *SSOHandler) provisionAndBind(r *http.Request, provider *ssoProvider, idToken *auth.IDTokenResult) (*ssoUser, error) {
	ctx := r.Context()

	email := idToken.Email
	if email == "" {
		email = idToken.Subject + "@sso.local"
	}
	name := idToken.Name
	if name == "" {
		name = email
	}
	role := resolveRole(provider.RoleMapping, idToken.Roles)

	// 闅忔満 32 瀛楄妭瀵嗙爜 鈫?bcrypt锛汼SO 寤哄彿鐢ㄦ埛涓嶅彲鐢ㄥ彛浠ょ櫥褰曪紝鐩村埌涓诲姩璁剧疆瀵嗙爜
	randomPassword := make([]byte, 32)
	if _, err := rand.Read(randomPassword); err != nil {
		return nil, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(randomPassword)), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// 鍚岀鎴峰悓 email 宸叉湁鏈湴璐﹀彿鏃剁洿鎺ョ粦瀹氾紝閬垮厤閲嶅寤哄彿
	var user ssoUser
	err = h.db.QueryRow(ctx,
		`SELECT id, email, name, role FROM users WHERE tenant_id = $1 AND email = $2`,
		provider.TenantID, email).Scan(&user.ID, &user.Email, &user.Name, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		err = h.db.QueryRow(ctx,
			`INSERT INTO users (tenant_id, email, name, password_hash, role, password_set)
			 VALUES ($1, $2, $3, $4, $5, FALSE) RETURNING id`,
			provider.TenantID, email, name, string(passwordHash), role).Scan(&user.ID)
		if err != nil {
			return nil, err
		}
		user.Email, user.Name, user.Role = email, name, role
	} else if err != nil {
		return nil, err
	}

	// 鎼哄甫鎵嬫満鍙凤紙寰俊/閽夐拤绛夛級鏃跺洖濉紱澶辫触浠呰褰曚笉闃绘柇
	if idToken.Phone != "" {
		if _, err := h.db.Exec(ctx,
			`UPDATE users SET phone = $2, updated_at = NOW() WHERE id = $1 AND (phone IS NULL OR phone = '')`,
			user.ID, idToken.Phone); err != nil {
			// 鎵嬫満鍙峰彲鑳戒笌浠栦汉鍐茬獊锛堝敮涓€绱㈠紩锛夛紝涓嶉樆鏂櫥褰曚富娴佺▼
			_ = err
		}
	}

	if _, err := h.db.Exec(ctx,
		`INSERT INTO ent_user_identities (user_id, provider_id, subject, email)
		 VALUES ($1, $2, $3, $4)`,
		user.ID, provider.ID, idToken.Subject, email); err != nil {
		if isUniqueViolation(err) {
			// 骞跺彂鍦烘櫙锛氬彟涓€璇锋眰宸插畬鎴愮粦瀹氾紝鐩存帴澶嶇敤鏃㈡湁缁戝畾鐢ㄦ埛
			if bound, lookupErr := h.findIdentityUser(ctx, provider.ID, idToken.Subject); lookupErr == nil {
				return bound, nil
			}
		}
		return nil, err
	}
	db.AuditLog(ctx, user.ID, "sso_provision", "/v1/auth/sso/callback",
		"provider="+provider.Name+" email="+email, r.RemoteAddr, nil)
	return &user, nil
}

// resolveRole 鎸?provider.role_mapping 灏?IdP "roles" claim 鏄犲皠涓烘湰鍦拌鑹诧紱
// 鏃犲懡涓椂缂虹渷 "user"銆傜函鍑芥暟锛屼究浜庡崟鍏冩祴璇曘€?
func resolveRole(mapping map[string]string, idpRoles []string) string {
	for _, r := range idpRoles {
		if mapped, ok := mapping[r]; ok && mapped != "" {
			return mapped
		}
	}
	return "user"
}
