package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// 鈹€鈹€ 浼佷笟鍚堣绛栫暐锛氱被鍨嬩笌閿欒 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// ErrModelNotAllowed 琛ㄧず璇锋眰鐨勬ā鍨嬩笉鍦ㄧ鎴?瑙掕壊鐧藉悕鍗曞唴锛?03 璇箟锛夈€?
// 闆嗘垚浠诲姟锛堝 /submit 鎷︽埅锛夊簲灏嗚閿欒鏄犲皠涓?HTTP 403銆?
var ErrModelNotAllowed = errors.New("model not allowed by enterprise policy")

// EntTenantPolicy 瀵瑰簲 ent_tenant_policies 琛紙绉熸埛绾ч殣绉?鐣欏瓨绛栫暐锛夈€?
type EntTenantPolicy struct {
	TenantID          string          `json:"tenant_id"`
	PrivacyMode       bool            `json:"privacy_mode"`
	DataRetentionDays int             `json:"data_retention_days"`
	TrainingAllowed   bool            `json:"training_allowed"`
	RedactionRules    json.RawMessage `json:"redaction_rules"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// EntModelPolicy 瀵瑰簲 ent_model_policies 琛紙妯″瀷鐧藉悕鍗曚笌姣忔ā鍨嬮檺閫燂級銆?
// RoleID 涓?nil 琛ㄧず绉熸埛绾у厹搴曠瓥鐣ャ€?
type EntModelPolicy struct {
	ID               string                    `json:"id"`
	TenantID         string                    `json:"tenant_id"`
	RoleID           *string                   `json:"role_id"`
	AllowedModels    []string                  `json:"allowed_models"`
	PerModelLimits   map[string]map[string]int `json:"per_model_limits"`
	CreatedAt        time.Time                 `json:"created_at"`
	UpdatedAt        time.Time                 `json:"updated_at"`
}

// 鈹€鈹€ Handler 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// EntPolicyHandler 鎻愪緵浼佷笟鍚堣绛栫暐 API锛堢鎴烽殣绉佺瓥鐣?/ 妯″瀷璁块棶绛栫暐 CRUD锛夛紝
// 骞跺鍑轰緵闆嗘垚浠诲姟浣跨敤鐨勫己鍒剁偣鍑芥暟锛圗nforceEnterprisePolicy 绛夛級銆?
// 璺敱娉ㄥ唽鐢遍泦鎴愪换鍔＄粺涓€鎺ュ叆锛堟湰浠诲姟涓嶆敞鍐岋級锛?
//
//	policyHandler := api.NewEntPolicyHandler()
//	policyHandler.RegisterRoutes(mux, authMW)
type EntPolicyHandler struct {
	// resolveAllowed 瑙ｆ瀽鐢ㄦ埛閫傜敤鐨勬ā鍨嬬櫧鍚嶅崟銆?
	// 杩斿洖 (allowed, hasPolicy, err)锛歨asPolicy=false 琛ㄧず鏃犱换浣曠瓥鐣ワ紙鏀捐锛夈€?
	// nil 鏃朵娇鐢ㄩ粯璁?PG 瀹炵幇锛涙祴璇曞彲娉ㄥ叆 fake銆?
	resolveAllowed func(ctx context.Context, tenantID, userID string) ([]string, bool, error)
}

// NewEntPolicyHandler 鍒涘缓绛栫暐 handler锛堥粯璁?PG 瀹炵幇锛夈€?
func NewEntPolicyHandler() *EntPolicyHandler {
	return &EntPolicyHandler{resolveAllowed: defaultResolveAllowedModels}
}

// RegisterRoutes 鎸傝浇绛栫暐璺敱锛坅uthMW + RequireEntPerm("policy:manage")锛夈€?
func (h *EntPolicyHandler) RegisterRoutes(mux *http.ServeMux, authMW func(http.Handler) http.Handler) {
	permMW := RequireEntPerm("policy:manage")
	handle := func(pattern string, hf http.HandlerFunc) {
		mux.Handle(pattern, authMW(permMW(http.HandlerFunc(hf))))
	}
	handle("GET /v1/ent/privacy", h.GetPrivacy)
	handle("PUT /v1/ent/privacy", h.PutPrivacy)
	handle("GET /v1/ent/model-policies", h.ListModelPolicies)
	handle("POST /v1/ent/model-policies", h.CreateModelPolicy)
	handle("GET /v1/ent/model-policies/{id}", h.GetModelPolicy)
	handle("PUT /v1/ent/model-policies/{id}", h.UpdateModelPolicy)
	handle("DELETE /v1/ent/model-policies/{id}", h.DeleteModelPolicy)
}

// entPolicyTenantID 鍙栧綋鍓嶈姹傜殑绉熸埛 ID锛歝laims 浼樺厛锛岀己鐪佸洖閫€榛樿绉熸埛
// 锛堢幇琛岀櫥褰曢摼璺湭绛惧彂 tenant_id claim锛屽崟绉熸埛妯″紡涓嬫亽涓洪粯璁ょ鎴凤級銆?
func entPolicyTenantID(claims *auth.Claims) string {
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return DefaultTenantID
}

// 鈹€鈹€ 寮哄埗鐐瑰嚱鏁帮紙瀵煎嚭缁欓泦鎴愪换鍔★紝鐙珛鍙祴锛?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// EnforceEnterprisePolicy 鏍￠獙 modelID 鏄惁鍦ㄨ鐢ㄦ埛閫傜敤鐨勬ā鍨嬬櫧鍚嶅崟鍐呫€?
//
// 瑙ｆ瀽瑙勫垯锛歳ole 绮剧‘绛栫暐浼樺厛锛堢敤鎴风洿鎺ヨ鑹?鈭?缇ょ粍鎴愬憳瑙掕壊鐨勪换涓€鍛戒腑锛夛紝
// 缂哄け鍒欏洖閫€绉熸埛绾х瓥鐣ワ紙role_id IS NULL锛夛紱涓よ€呴兘鏃?鈫?鏀捐銆?
// 浠讳綍鏌ヨ澶辫触涓€寰?fail-open锛堟斁琛?+ slog.Warn锛夛紝淇濊瘉绛栫暐鍩虹璁炬柦鏁呴殰
// 涓嶉樆鏂笟鍔￠摼璺€俶odel 涓嶅湪鐧藉悕鍗曡繑鍥?ErrModelNotAllowed锛?03 璇箟锛夈€?
func (h *EntPolicyHandler) EnforceEnterprisePolicy(r *http.Request, claims *auth.Claims, modelID string) error {
	if modelID == "" {
		return nil
	}
	if claims == nil {
		slog.Warn("ent policy: missing claims, fail-open", "model", modelID)
		return nil
	}
	allowed, hasPolicy, err := h.resolveAllowed(r.Context(), claims.TenantID, claims.UserID)
	if err != nil {
		slog.Warn("ent policy: resolve allow-list failed, fail-open",
			"user", claims.UserID, "tenant", claims.TenantID, "model", modelID, "error", err)
		return nil
	}
	if !hasPolicy {
		return nil
	}
	for _, m := range allowed {
		if m == modelID {
			return nil
		}
	}
	return ErrModelNotAllowed
}

// defaultPolicyHandler 渚涘寘绾у己鍒剁偣鍑芥暟浣跨敤鐨勯粯璁ゅ疄渚嬨€?
var defaultPolicyHandler = NewEntPolicyHandler()

// EnforceEnterprisePolicy 鍖呯骇鍏ュ彛锛堢鍚嶅悓鏂规硶鐗堟湰锛夛紝闆嗘垚浠诲姟鐩存帴璋冪敤銆?
func EnforceEnterprisePolicy(r *http.Request, claims *auth.Claims, modelID string) error {
	return defaultPolicyHandler.EnforceEnterprisePolicy(r, claims, modelID)
}

// ResolveAllowedModels 杩斿洖鐢ㄦ埛瑙嗚鐨勫彲鐢ㄦā鍨嬫竻鍗曪細
// 鏈夌敓鏁堢瓥鐣ユ椂杩斿洖鐧藉悕鍗曪紱鏃犱换浣曠瓥鐣ユ椂杩斿洖 admin_model_configs 鍏ㄩ噺妯″瀷銆?
// 鏌ヨ澶辫触杩斿洖 error锛堜緵 GET /v1/models 鏀归€犲悗鎸?fail-open 澶勭悊锛夈€?
func (h *EntPolicyHandler) ResolveAllowedModels(ctx context.Context, tenantID, userID string) ([]string, error) {
	allowed, hasPolicy, err := h.resolveAllowed(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	if hasPolicy {
		if allowed == nil {
			allowed = []string{}
		}
		return allowed, nil
	}
	return h.allModelIDs(ctx)
}

// ResolveAllowedModels 鍖呯骇鍏ュ彛銆?
func ResolveAllowedModels(ctx context.Context, tenantID, userID string) ([]string, error) {
	return defaultPolicyHandler.ResolveAllowedModels(ctx, tenantID, userID)
}

// GetTenantPrivacyMode 杩斿洖绉熸埛鏄惁寮€鍚殣绉佹ā寮忥紙鏃犺褰?鈫?false锛夈€?
// 闆嗘垚浠诲姟鎹鍦ㄨ浆鍙?Python 寮曟搸璇锋眰鏃舵敞鍏?X-Privacy-Mode: no_retention 澶淬€?
func (h *EntPolicyHandler) GetTenantPrivacyMode(ctx context.Context, tenantID string) (bool, error) {
	pool := db.ReadPool()
	if pool == nil {
		return false, errors.New("ent policy: postgres pool unavailable")
	}
	var mode bool
	err := pool.QueryRow(ctx,
		`SELECT privacy_mode FROM ent_tenant_policies WHERE tenant_id = $1`, tenantID).Scan(&mode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return mode, nil
}

// GetTenantPrivacyMode 鍖呯骇鍏ュ彛銆?
func GetTenantPrivacyMode(ctx context.Context, tenantID string) (bool, error) {
	return defaultPolicyHandler.GetTenantPrivacyMode(ctx, tenantID)
}

// modelRateLua 鍗曢敭绐楀彛闄愭祦鍘熷瓙鑴氭湰锛氳秴闄愯繑鍥?0锛屽惁鍒欒鏁?+1 骞惰繑鍥?1銆?
const modelRateLua = `
local cur = tonumber(redis.call("GET", KEYS[1]) or "0")
if cur >= tonumber(ARGV[1]) then return 0 end
redis.call("INCR", KEYS[1])
redis.call("EXPIRE", KEYS[1], ARGV[2])
return 1
`

// CheckModelRate 姣忔ā鍨嬫瘡绉熸埛閫熺巼妫€鏌ワ紙RPM锛?0s 绐楀彛锛夈€?
// 闄愰€熷€间紭鍏堝彇 ent_model_policies.per_model_limits[model]["rpm"]锛堢鎴风骇绛栫暐锛夛紝
// 缂虹渷鍥為€€鍏ュ弬 rpmLimit锛堣皟鐢ㄦ柟浠?admin_model_configs.max_rpm 璇诲彇锛夈€?
// 閿牸寮?model:{model}:{tenant}锛沴imit<=0 鎴?Redis 涓嶅彲鐢?鍑洪敊鏃舵斁琛岋紙fail-open锛夈€?
func (h *EntPolicyHandler) CheckModelRate(ctx context.Context, tenantID, modelID string, rpmLimit int) error {
	limit := rpmLimit
	if policyLimit, err := h.policyModelRPM(ctx, tenantID, modelID); err == nil && policyLimit > 0 {
		limit = policyLimit
	}
	if limit <= 0 {
		return nil
	}
	rdb := db.Redis
	if rdb == nil {
		return nil
	}
	key := fmt.Sprintf("model:%s:%s", modelID, tenantID)
	ok, err := rdb.Eval(ctx, modelRateLua, []string{key}, limit, 60).Bool()
	if err != nil {
		slog.Warn("ent policy: model rate limit check failed, fail-open",
			"model", modelID, "tenant", tenantID, "error", err)
		return nil
	}
	if !ok {
		return fmt.Errorf("model %s rate limit exceeded for tenant %s", modelID, tenantID)
	}
	return nil
}

// CheckModelRate 鍖呯骇鍏ュ彛銆?
func CheckModelRate(ctx context.Context, tenantID, modelID string, rpmLimit int) error {
	return defaultPolicyHandler.CheckModelRate(ctx, tenantID, modelID, rpmLimit)
}

// 鈹€鈹€ 榛樿 PG 瑙ｆ瀽瀹炵幇 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// defaultResolveAllowedModels 浠?PG 瑙ｆ瀽鐢ㄦ埛閫傜敤鐨勬ā鍨嬬櫧鍚嶅崟銆?
// 姝ラ锛歵enantID 缂虹渷缁?users 琛ㄨВ鏋?鈫?鑱氬悎鐢ㄦ埛瑙掕壊锛堢洿鎺?鈭?缇ょ粍锛夆啋
// role 绮剧‘绛栫暐浼樺厛銆佺鎴风骇鍏滃簳銆傛棤绛栫暐杩斿洖 (nil, false, nil)銆?
func defaultResolveAllowedModels(ctx context.Context, tenantID, userID string) ([]string, bool, error) {
	pool := db.ReadPool()
	if pool == nil {
		return nil, false, errors.New("ent policy: postgres pool unavailable")
	}

	if tenantID == "" {
		if userID == "" {
			return nil, false, errors.New("ent policy: missing tenant_id and user_id")
		}
		if err := pool.QueryRow(ctx,
			`SELECT tenant_id FROM users WHERE id = $1`, userID).Scan(&tenantID); err != nil {
			return nil, false, fmt.Errorf("ent policy: resolve tenant: %w", err)
		}
	}

	// 鐢ㄦ埛瑙掕壊闆嗗悎锛氱洿鎺ヨ鑹?鈭?缇ょ粍鎴愬憳瑙掕壊
	roleIDs := []string{}
	if userID != "" {
		rows, err := pool.Query(ctx,
			`SELECT role_id FROM ent_user_roles WHERE user_id = $1
			 UNION
			 SELECT gr.role_id FROM ent_group_members gm
			 JOIN ent_group_roles gr ON gr.group_id = gm.group_id
			 WHERE gm.user_id = $1`, userID)
		if err != nil {
			return nil, false, fmt.Errorf("ent policy: query user roles: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, false, fmt.Errorf("ent policy: scan user roles: %w", err)
			}
			roleIDs = append(roleIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, false, fmt.Errorf("ent policy: iterate user roles: %w", err)
		}
	}

	// role 绮剧‘绛栫暐浼樺厛锛坮ole_id IS NULL 鎺掑悗锛夛紝鍛戒腑绗竴鏉″嵆鐢熸晥
	var allowed []string
	err := pool.QueryRow(ctx,
		`SELECT allowed_models FROM ent_model_policies
		 WHERE tenant_id = $1 AND (role_id IS NULL OR role_id = ANY($2::uuid[]))
		 ORDER BY (role_id IS NULL) ASC
		 LIMIT 1`, tenantID, roleIDs).Scan(&allowed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("ent policy: query model policy: %w", err)
	}
	return allowed, true, nil
}

// allModelIDs 杩斿洖 admin_model_configs 鍏ㄩ噺 model_id锛堟ā鍨嬫竻鍗曠洿鎺ヨ璇ヨ〃锛夈€?
func (h *EntPolicyHandler) allModelIDs(ctx context.Context) ([]string, error) {
	pool := db.ReadPool()
	if pool == nil {
		return nil, errors.New("ent policy: postgres pool unavailable")
	}
	rows, err := pool.Query(ctx, `SELECT model_id FROM admin_model_configs ORDER BY priority DESC, model_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	models := []string{}
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

// policyModelRPM 璇诲彇绉熸埛绾х瓥鐣?per_model_limits[model]["rpm"]锛堟棤 鈫?0锛夈€?
func (h *EntPolicyHandler) policyModelRPM(ctx context.Context, tenantID, modelID string) (int, error) {
	pool := db.ReadPool()
	if pool == nil {
		return 0, errors.New("ent policy: postgres pool unavailable")
	}
	var raw []byte
	err := pool.QueryRow(ctx,
		`SELECT per_model_limits FROM ent_model_policies WHERE tenant_id = $1 AND role_id IS NULL`,
		tenantID).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	limits := map[string]map[string]int{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &limits); err != nil {
			return 0, fmt.Errorf("ent policy: parse per_model_limits: %w", err)
		}
	}
	if lm, ok := limits[modelID]; ok {
		return lm["rpm"], nil
	}
	return 0, nil
}

// 鈹€鈹€ 绉熸埛闅愮绛栫暐绔偣 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// GetPrivacy GET /v1/ent/privacy 鈥斺€?璇诲彇褰撳墠绉熸埛绛栫暐锛堟棤璁板綍杩斿洖榛樿鍊硷級銆?
func (h *EntPolicyHandler) GetPrivacy(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	tenantID := entPolicyTenantID(claims)

	pool := db.ReadPool()
	if pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	var p EntTenantPolicy
	err := pool.QueryRow(r.Context(),
		`SELECT tenant_id, privacy_mode, data_retention_days, training_allowed, redaction_rules, updated_at
		 FROM ent_tenant_policies WHERE tenant_id = $1`, tenantID).
		Scan(&p.TenantID, &p.PrivacyMode, &p.DataRetentionDays, &p.TrainingAllowed, &p.RedactionRules, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			OK(w, EntTenantPolicy{
				TenantID:        tenantID,
				TrainingAllowed: true,
				RedactionRules:  json.RawMessage(`{}`),
			})
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, "get tenant policy failed")
		return
	}
	OK(w, p)
}

// PutPrivacy PUT /v1/ent/privacy 鈥斺€?UPSERT 褰撳墠绉熸埛绛栫暐銆?
func (h *EntPolicyHandler) PutPrivacy(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	tenantID := entPolicyTenantID(claims)

	var body struct {
		PrivacyMode       bool            `json:"privacy_mode"`
		DataRetentionDays int             `json:"data_retention_days"`
		TrainingAllowed   bool            `json:"training_allowed"`
		RedactionRules    json.RawMessage `json:"redaction_rules"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.DataRetentionDays < 0 {
		BadRequest(w, "data_retention_days must be >= 0")
		return
	}
	if !json.Valid(body.RedactionRules) {
		BadRequest(w, "redaction_rules must be valid JSON object")
		return
	}

	if db.Pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	var p EntTenantPolicy
	err := db.Pool.QueryRow(r.Context(),
		`INSERT INTO ent_tenant_policies (tenant_id, privacy_mode, data_retention_days, training_allowed, redaction_rules, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (tenant_id) DO UPDATE SET
		     privacy_mode = EXCLUDED.privacy_mode,
		     data_retention_days = EXCLUDED.data_retention_days,
		     training_allowed = EXCLUDED.training_allowed,
		     redaction_rules = EXCLUDED.redaction_rules,
		     updated_at = NOW()
		 RETURNING tenant_id, privacy_mode, data_retention_days, training_allowed, redaction_rules, updated_at`,
		tenantID, body.PrivacyMode, body.DataRetentionDays, body.TrainingAllowed, string(body.RedactionRules)).
		Scan(&p.TenantID, &p.PrivacyMode, &p.DataRetentionDays, &p.TrainingAllowed, &p.RedactionRules, &p.UpdatedAt)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "upsert tenant policy failed")
		return
	}
	OK(w, p)
}

// 鈹€鈹€ 妯″瀷璁块棶绛栫暐绔偣 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// validModelIDSet 浠?admin_model_configs 璇诲彇鍚堟硶 model_id 闆嗗悎锛堢櫧鍚嶅崟鏍￠獙鐢級銆?
func validModelIDSet(ctx context.Context) (map[string]bool, error) {
	pool := db.ReadPool()
	if pool == nil {
		return nil, errors.New("ent policy: postgres pool unavailable")
	}
	rows, err := pool.Query(ctx, `SELECT model_id FROM admin_model_configs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		set[m] = true
	}
	return set, rows.Err()
}

// validateAllowedModels 鏍￠獙鐧藉悕鍗曞厓绱犲潎瀛樺湪浜?admin_model_configs.model_id锛?
// 杩斿洖闈炴硶鍏冪礌鍒楄〃锛堢┖ = 鍚堟硶锛夈€?
func validateAllowedModels(ctx context.Context, models []string) ([]string, error) {
	if len(models) == 0 {
		return nil, nil
	}
	set, err := validModelIDSet(ctx)
	if err != nil {
		return nil, err
	}
	invalid := []string{}
	for _, m := range models {
		if !set[m] {
			invalid = append(invalid, m)
		}
	}
	return invalid, nil
}

const modelPolicyColumns = `id, tenant_id, role_id, allowed_models, per_model_limits, created_at, updated_at`

func scanModelPolicy(row interface{ Scan(...any) error }) (*EntModelPolicy, error) {
	var p EntModelPolicy
	var raw []byte
	if err := row.Scan(&p.ID, &p.TenantID, &p.RoleID, &p.AllowedModels, &raw, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.PerModelLimits = map[string]map[string]int{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &p.PerModelLimits)
	}
	return &p, nil
}

// ListModelPolicies GET /v1/ent/model-policies 鈥斺€?褰撳墠绉熸埛鍏ㄩ儴绛栫暐銆?
func (h *EntPolicyHandler) ListModelPolicies(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	tenantID := entPolicyTenantID(claims)

	pool := db.ReadPool()
	if pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	rows, err := pool.Query(r.Context(),
		`SELECT `+modelPolicyColumns+` FROM ent_model_policies
		 WHERE tenant_id = $1 ORDER BY (role_id IS NULL) DESC, created_at`, tenantID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list model policies failed")
		return
	}
	defer rows.Close()
	policies := []EntModelPolicy{}
	for rows.Next() {
		p, err := scanModelPolicy(rows)
		if err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "list model policies failed")
			return
		}
		policies = append(policies, *p)
	}
	if err := rows.Err(); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list model policies failed")
		return
	}
	OK(w, policies)
}

// decodeModelPolicyBody 瑙ｆ瀽鍒涘缓/鏇存柊璇锋眰浣撳苟鍋氬瓧娈垫牎楠岋紙鐧藉悕鍗曞厓绱犲悎娉曟€ч櫎澶栵級銆?
func decodeModelPolicyBody(w http.ResponseWriter, r *http.Request) (*EntModelPolicy, bool) {
	var body struct {
		RoleID         string                    `json:"role_id"`
		AllowedModels  []string                  `json:"allowed_models"`
		PerModelLimits map[string]map[string]int `json:"per_model_limits"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return nil, false
	}
	if body.AllowedModels == nil {
		body.AllowedModels = []string{}
	}
	for _, m := range body.AllowedModels {
		if strings.TrimSpace(m) == "" {
			BadRequest(w, "allowed_models must not contain empty entries")
			return nil, false
		}
	}
	if body.PerModelLimits == nil {
		body.PerModelLimits = map[string]map[string]int{}
	}
	p := &EntModelPolicy{
		AllowedModels:  body.AllowedModels,
		PerModelLimits: body.PerModelLimits,
	}
	if body.RoleID != "" {
		if _, err := uuid.Parse(body.RoleID); err != nil {
			BadRequest(w, "role_id must be a valid uuid (or omitted for tenant-level policy)")
			return nil, false
		}
		p.RoleID = &body.RoleID
	}
	return p, true
}

// CreateModelPolicy POST /v1/ent/model-policies
// allowed_models 鍏冪礌蹇呴』瀛樺湪浜?admin_model_configs.model_id锛堣繚鍙?422锛夛紱
// UNIQUE NULLS NOT DISTINCT(tenant_id, role_id) 鍐茬獊杩斿洖 409銆?
func (h *EntPolicyHandler) CreateModelPolicy(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	p, ok := decodeModelPolicyBody(w, r)
	if !ok {
		return
	}
	tenantID := entPolicyTenantID(claims)

	if invalid, err := validateAllowedModels(r.Context(), p.AllowedModels); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "validate allowed models failed")
		return
	} else if len(invalid) > 0 {
		JSON(w, http.StatusUnprocessableEntity, APIResponse{
			Success: false,
			Error:   "unknown model_id in allowed_models: " + strings.Join(invalid, ", "),
		})
		return
	}

	if db.Pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	limitsJSON, _ := json.Marshal(p.PerModelLimits)
	created, err := scanModelPolicy(db.Pool.QueryRow(r.Context(),
		`INSERT INTO ent_model_policies (tenant_id, role_id, allowed_models, per_model_limits)
		 VALUES ($1, $2, $3, $4) RETURNING `+modelPolicyColumns,
		tenantID, p.RoleID, p.AllowedModels, string(limitsJSON)))
	if err != nil {
		if isUniqueViolation(err) {
			JSON(w, http.StatusConflict, APIResponse{
				Success: false,
				Error:   "model policy already exists for this tenant/role",
			})
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, "create model policy failed")
		return
	}
	Created(w, created)
}

// GetModelPolicy GET /v1/ent/model-policies/{id}
func (h *EntPolicyHandler) GetModelPolicy(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		BadRequest(w, "invalid policy id")
		return
	}
	pool := db.ReadPool()
	if pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	p, err := scanModelPolicy(pool.QueryRow(r.Context(),
		`SELECT `+modelPolicyColumns+` FROM ent_model_policies WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			NotFound(w, ErrNotFound)
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, "get model policy failed")
		return
	}
	OK(w, p)
}

// UpdateModelPolicy PUT /v1/ent/model-policies/{id}锛堝叏閲忔浛鎹?allowed_models /
// per_model_limits / role_id锛涙牎楠岃鍒欏悓鍒涘缓锛夈€?
func (h *EntPolicyHandler) UpdateModelPolicy(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		BadRequest(w, "invalid policy id")
		return
	}
	p, ok := decodeModelPolicyBody(w, r)
	if !ok {
		return
	}
	if invalid, err := validateAllowedModels(r.Context(), p.AllowedModels); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "validate allowed models failed")
		return
	} else if len(invalid) > 0 {
		JSON(w, http.StatusUnprocessableEntity, APIResponse{
			Success: false,
			Error:   "unknown model_id in allowed_models: " + strings.Join(invalid, ", "),
		})
		return
	}
	if db.Pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	limitsJSON, _ := json.Marshal(p.PerModelLimits)
	updated, err := scanModelPolicy(db.Pool.QueryRow(r.Context(),
		`UPDATE ent_model_policies
		 SET role_id = $2, allowed_models = $3, per_model_limits = $4, updated_at = NOW()
		 WHERE id = $1 RETURNING `+modelPolicyColumns,
		id, p.RoleID, p.AllowedModels, string(limitsJSON)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			NotFound(w, ErrNotFound)
			return
		}
		if isUniqueViolation(err) {
			JSON(w, http.StatusConflict, APIResponse{
				Success: false,
				Error:   "model policy already exists for this tenant/role",
			})
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, "update model policy failed")
		return
	}
	OK(w, updated)
}

// DeleteModelPolicy DELETE /v1/ent/model-policies/{id}
func (h *EntPolicyHandler) DeleteModelPolicy(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		BadRequest(w, "invalid policy id")
		return
	}
	if db.Pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	tag, err := db.Pool.Exec(r.Context(), `DELETE FROM ent_model_policies WHERE id = $1`, id)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "delete model policy failed")
		return
	}
	if tag.RowsAffected() == 0 {
		NotFound(w, ErrNotFound)
		return
	}
	NoContent(w)
}
