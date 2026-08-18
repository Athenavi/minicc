package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// maskedSecret 是 provider 读响应中 client_secret 的唯一脱敏形态。
const maskedSecret = "********"

// ssoStateTTL 是 SSO state 令牌有效期（防重放窗口）。
const ssoStateTTL = 10 * time.Minute

// ── 数据模型 ────────────────────────────────────────────

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
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ssoUser struct {
	ID    string
	Email string
	Name  string
	Role  string
}

// sanitizeProvider 构造 provider 的对外响应：client_secret 一律脱敏。
func sanitizeProvider(p *ssoProvider) map[string]any {
	roleMapping := p.RoleMapping
	if roleMapping == nil {
		roleMapping = map[string]string{}
	}
	scopes := p.Scopes
	if scopes == nil {
		scopes = []string{}
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
		"created_at":     p.CreatedAt,
		"updated_at":     p.UpdatedAt,
	}
}

type scanner interface{ Scan(dest ...any) error }

const ssoProviderColumns = `id, tenant_id, name, issuer, client_id, client_secret_enc,
       scopes, enabled, auto_provision, role_mapping, created_at, updated_at`

func scanSSOProvider(row scanner) (*ssoProvider, error) {
	var p ssoProvider
	var scopes []string
	var roleMappingRaw []byte
	if err := row.Scan(&p.ID, &p.TenantID, &p.Name, &p.Issuer, &p.ClientID, &p.ClientSecretEnc,
		&scopes, &p.Enabled, &p.AutoProvision, &roleMappingRaw, &p.CreatedAt, &p.UpdatedAt); err != nil {
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
	return &p, nil
}

// ── Handler ─────────────────────────────────────────────

// SSOHandler 负责 OIDC SSO 公开流程与管理端 CRUD。
// db 抽象为 entQuerier 便于测试注入 fake；exchanger 抽象 IdP 交互。
type SSOHandler struct {
	auth        *auth.Authenticator
	cfg         *config.Config
	db          entQuerier
	exchanger   auth.OIDCExchanger
	codec       *auth.StateCodec
	encKey      []byte
	redirectURL string // OAuth2 redirect_uri（授权回调地址）
	successURL  string // 登录成功后 302 回前端的目标
}

func NewSSOHandler(authenticator *auth.Authenticator, cfg *config.Config) *SSOHandler {
	h := &SSOHandler{
		auth:        authenticator,
		cfg:         cfg,
		db:          pgEntStore{},
		exchanger:   auth.NewRemoteOIDCExchanger(),
		encKey:      auth.LoadOIDCEncryptionKey(),
		redirectURL: cfg.PublicBaseURL + "/v1/auth/sso/callback",
		successURL:  "/",
	}
	if h.encKey != nil {
		h.codec = auth.NewStateCodec(h.encKey, ssoStateTTL)
	}
	return h
}

// RegisterPublicRoutes 挂载公开 SSO 路由（无 authMW）：
// 发现列表 / 登录跳转 / IdP 回调。供 Phase 7 集成任务调用。
func (h *SSOHandler) RegisterPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/auth/sso/providers", h.ListPublicProviders)
	mux.HandleFunc("GET /v1/auth/sso/login/{providerID}", h.Login)
	mux.HandleFunc("GET /v1/auth/sso/callback", h.Callback)
}

// RegisterAdminRoutes 挂载 SSO 管理路由：authMW + RequireEntPerm("sso:manage")。
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

// ── 公开路由 ────────────────────────────────────────────

// ListPublicProviders GET /v1/auth/sso/providers
// 仅返回 enabled provider 的 id/name（不含任何敏感字段）。
func (h *SSOHandler) ListPublicProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		`SELECT id, name FROM ent_oidc_providers WHERE enabled = TRUE ORDER BY name`)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	defer rows.Close()

	items := make([]map[string]string, 0)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
			return
		}
		items = append(items, map[string]string{"id": id, "name": name})
	}
	if err := rows.Err(); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	OK(w, items)
}

// Login GET /v1/auth/sso/login/{providerID}
// 生成 state+nonce（nonce 内嵌于 HMAC 签名 state payload）→ 302 到 IdP 授权页。
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

	nonce, err := auth.RandomNonce()
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "sso login failed")
		return
	}
	state, err := h.codec.Issue(providerID, nonce)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "sso login failed")
		return
	}

	authURL, err := h.exchanger.AuthURL(r.Context(), h.oidcConfig(provider, clientSecret), state, nonce)
	if err != nil {
		logAndRespond(w, err, http.StatusBadGateway, "sso provider unreachable")
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback GET /v1/auth/sso/callback?code=&state=
// 校验 state → 授权码换 token → 校验 id_token（含 nonce）→ 绑定/自动建号 → 颁发会话。
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

	// 1. state 校验（HMAC 防 CSRF/篡改，TTL 防重放）——失败一律 400，且不触碰 DB
	payload, err := h.codec.Verify(state)
	if err != nil {
		logAndRespond(w, err, http.StatusBadRequest, "invalid sso state")
		return
	}

	// 2. 加载 provider（必须 enabled）
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

	// 3. 授权码交换 + id_token 校验（含 nonce 比对）
	idToken, err := h.exchanger.ExchangeAndVerify(r.Context(), h.oidcConfig(provider, clientSecret), code, payload.Nonce)
	if err != nil {
		logAndRespond(w, err, http.StatusUnauthorized, "sso authentication failed")
		return
	}
	if idToken.Subject == "" {
		BadRequest(w, "id_token missing subject")
		return
	}

	// 4. 按 (provider_id, subject) 查绑定
	user, err := h.findIdentityUser(r.Context(), provider.ID, idToken.Subject)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	// 5. 未绑定：auto_provision 决定建号或拒绝
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

	// 6. 复用现有登录的会话颁发逻辑（GenerateToken + SetTokenCookie）
	token, err := h.auth.GenerateToken(user.ID, user.Email, user.Role, auth.RolePermissions[user.Role])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}
	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()), h.cfg.CookieSecure)
	http.Redirect(w, r, h.successURL, http.StatusFound)
}

// ── 管理路由（authMW + sso:manage）──────────────────────

// ListProviders GET /v1/ent/sso/providers
func (h *SSOHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		`SELECT `+ssoProviderColumns+` FROM ent_oidc_providers ORDER BY created_at`)
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
}

// CreateProvider POST /v1/ent/sso/providers
// client_secret 以 AES-GCM 加密入库；密钥未配置时返回 503。
func (h *SSOHandler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	var req createProviderRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if req.Name == "" || req.Issuer == "" || req.ClientID == "" || req.ClientSecret == "" {
		BadRequest(w, "name, issuer, client_id and client_secret are required")
		return
	}
	if len(req.Name) > 64 || len(req.Issuer) > 512 || len(req.ClientID) > 256 {
		BadRequest(w, "field too long")
		return
	}
	if h.encKey == nil {
		ServiceUnavailable(w, "SSO is not configured: "+auth.EnvOIDCSecretKey+" missing")
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
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "email", "profile"}
	}
	roleMapping := req.RoleMapping
	if roleMapping == nil {
		roleMapping = map[string]string{}
	}
	mappingJSON, err := json.Marshal(roleMapping)
	if err != nil {
		BadRequest(w, "invalid role_mapping")
		return
	}

	var id string
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO ent_oidc_providers
		   (tenant_id, name, issuer, client_id, client_secret_enc, scopes, enabled, auto_provision, role_mapping)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		db.DefaultTenantID, req.Name, req.Issuer, req.ClientID, secretEnc,
		scopes, enabled, autoProvision, mappingJSON).Scan(&id)
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
}

// UpdateProvider PUT /v1/ent/sso/providers/{id}
// client_secret 为空串或脱敏占位符时保留原密文；提供新值时重新加密（需密钥）。
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

	if req.Name != nil {
		if *req.Name == "" || len(*req.Name) > 64 {
			BadRequest(w, "invalid name")
			return
		}
		name = *req.Name
	}
	if req.Issuer != nil {
		if *req.Issuer == "" || len(*req.Issuer) > 512 {
			BadRequest(w, "invalid issuer")
			return
		}
		issuer = *req.Issuer
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
	var mappingJSON []byte
	if req.RoleMapping != nil {
		mapping = *req.RoleMapping
		if mapping == nil {
			mapping = map[string]string{}
		}
		mappingJSON, err = json.Marshal(mapping)
		if err != nil {
			BadRequest(w, "invalid role_mapping")
			return
		}
	} else {
		mappingJSON, err = json.Marshal(mapping)
		if err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
			return
		}
	}

	if _, err := h.db.Exec(ctx,
		`UPDATE ent_oidc_providers
		 SET name = $2, issuer = $3, client_id = $4, client_secret_enc = $5,
		     scopes = $6, enabled = $7, auto_provision = $8, role_mapping = $9, updated_at = NOW()
		 WHERE id = $1`,
		id, name, issuer, clientID, secretEnc, scopes, enabled, autoProvision, mappingJSON); err != nil {
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

// ── 内部辅助 ────────────────────────────────────────────

func (h *SSOHandler) getProvider(ctx context.Context, id string) (*ssoProvider, error) {
	return scanSSOProvider(h.db.QueryRow(ctx,
		`SELECT `+ssoProviderColumns+` FROM ent_oidc_providers WHERE id = $1`, id))
}

func (h *SSOHandler) getEnabledProvider(ctx context.Context, id string) (*ssoProvider, error) {
	return scanSSOProvider(h.db.QueryRow(ctx,
		`SELECT `+ssoProviderColumns+` FROM ent_oidc_providers WHERE id = $1 AND enabled = TRUE`, id))
}

func (h *SSOHandler) oidcConfig(p *ssoProvider, clientSecret string) *auth.OIDCProviderConfig {
	return &auth.OIDCProviderConfig{
		Issuer:       p.Issuer,
		ClientID:     p.ClientID,
		ClientSecret: clientSecret,
		Scopes:       p.Scopes,
		RedirectURL:  h.redirectURL,
	}
}

// findIdentityUser 按 (provider_id, subject) 查绑定用户；未绑定返回 pgx.ErrNoRows。
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

// provisionAndBind 自动建号并写入外部身份绑定。
// 租户取 provider 配置；email 取 id_token claim（缺失时以 subject 兜底）；
// password_hash 写入随机不可碰撞 bcrypt；role 按 role_mapping 匹配、缺省 "user"。
func (h *SSOHandler) provisionAndBind(r *http.Request, provider *ssoProvider, idToken *auth.IDTokenResult) (*ssoUser, error) {
	ctx := r.Context()

	email := idToken.Email
	if email == "" {
		email = idToken.Subject + "@sso.local"
	}
	name := email
	role := resolveRole(provider.RoleMapping, idToken.Roles)

	// 随机 32 字节密码 → bcrypt；SSO 用户永不走口令登录
	randomPassword := make([]byte, 32)
	if _, err := rand.Read(randomPassword); err != nil {
		return nil, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(randomPassword)), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// 同租户同 email 已有本地账号时直接绑定，避免重复建号
	var user ssoUser
	err = h.db.QueryRow(ctx,
		`SELECT id, email, name, role FROM users WHERE tenant_id = $1 AND email = $2`,
		provider.TenantID, email).Scan(&user.ID, &user.Email, &user.Name, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		err = h.db.QueryRow(ctx,
			`INSERT INTO users (tenant_id, email, name, password_hash, role)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			provider.TenantID, email, name, string(passwordHash), role).Scan(&user.ID)
		if err != nil {
			return nil, err
		}
		user.Email, user.Name, user.Role = email, name, role
	} else if err != nil {
		return nil, err
	}

	if _, err := h.db.Exec(ctx,
		`INSERT INTO ent_user_identities (user_id, provider_id, subject, email)
		 VALUES ($1, $2, $3, $4)`,
		user.ID, provider.ID, idToken.Subject, email); err != nil {
		if isUniqueViolation(err) {
			// 并发场景：另一请求已完成绑定，直接复用既有绑定用户
			if bound, lookupErr := h.findIdentityUser(ctx, provider.ID, idToken.Subject); lookupErr == nil {
				return bound, nil
			}
		}
		return nil, err
	}
	return &user, nil
}

// resolveRole 按 provider.role_mapping 将 IdP "roles" claim 映射为本地角色；
// 无命中时缺省 "user"。纯函数，便于单元测试。
func resolveRole(mapping map[string]string, idpRoles []string) string {
	for _, r := range idpRoles {
		if mapped, ok := mapping[r]; ok && mapped != "" {
			return mapped
		}
	}
	return "user"
}
