package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// 鈹€鈹€ 鑳藉姏甯傚満锛氱被鍨嬩笌甯搁噺 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// MarketItem 瀵瑰簲 ent_catalog_items 琛紙甯傚満鐩綍鏉＄洰锛歱lugin/skill锛夈€?
type MarketItem struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"` // plugin / skill
	Name       string          `json:"name"`
	Version    string          `json:"version"`
	Manifest   json.RawMessage `json:"manifest"`
	Status     string          `json:"status"` // draft / published / retired
	CreatedBy  *string         `json:"created_by,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// MarketGrant 瀵瑰簲 ent_catalog_installs 琛紙绉熸埛瀹夎/鍚敤璁板綍锛夈€?
type MarketGrant struct {
	ItemID      string    `json:"item_id"`
	TenantID    string    `json:"tenant_id"`
	Enabled     bool      `json:"enabled"`
	InstalledAt time.Time `json:"installed_at"`
}

// 鍚堟硶鏋氫妇锛堜笌杩佺Щ CHECK 绾︽潫涓€鑷达級
var (
	validMarketItemTypes = map[string]bool{"plugin": true, "skill": true, "agent": true, "mcp": true}
)

var validMarketItemName = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,128}$`)

// canCatalogTransition 鐘舵€佹満鏍￠獙锛氫粎鍏佽 draft鈫抪ublished鈫抮etired锛?
// retired 涓虹粓鎬佷笉鍙洖 published銆?
func canCatalogTransition(from, to string) bool {
	switch {
	case from == "draft" && to == "published":
		return true
	case from == "published" && to == "retired":
		return true
	default:
		return false
	}
}

// 鈹€鈹€ Handler 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// MarketHandler 鎻愪緵浼佷笟鑳藉姏甯傚満 API锛堢洰褰曟潯鐩?CRUD / 鍙戝竷鐘舵€佹満 / 绉熸埛鎺堟潈锛夈€?
// 璺敱娉ㄥ唽鐢遍泦鎴愪换鍔＄粺涓€鎺ュ叆锛堟湰浠诲姟涓嶆敞鍐岋級锛?
//
//	marketHandler := api.NewMarketHandler()
//	marketHandler.RegisterRoutes(mux, authMW)
type MarketHandler struct{}

// NewMarketHandler 鍒涘缓甯傚満 handler銆?
func NewMarketHandler() *MarketHandler { return &MarketHandler{} }

// RegisterRoutes 鎸傝浇甯傚満璺敱锛坅uthMW + RequireEntPerm("market:manage")锛夈€?
func (h *MarketHandler) RegisterRoutes(mux *http.ServeMux, authMW func(http.Handler) http.Handler) {
	permMW := RequireEntPerm("market:manage")
	handle := func(pattern string, hf http.HandlerFunc) {
		mux.Handle(pattern, authMW(permMW(http.HandlerFunc(hf))))
	}
	handle("GET /v1/ent/market/items", h.ListItems)
	handle("POST /v1/ent/market/items", h.CreateItem)
	handle("GET /v1/ent/market/items/{id}", h.GetItem)
	handle("PUT /v1/ent/market/items/{id}", h.UpdateItem)
	handle("DELETE /v1/ent/market/items/{id}", h.DeleteItem)
	handle("POST /v1/ent/market/items/{id}/publish", h.PublishItem)
	handle("POST /v1/ent/market/items/{id}/retire", h.RetireItem)
	handle("GET /v1/ent/market/grants", h.ListGrants)
	handle("POST /v1/ent/market/grants", h.GrantItem)
	handle("PUT /v1/ent/market/grants/{itemID}/{tenantID}", h.UpdateGrant)
	handle("DELETE /v1/ent/market/grants/{itemID}/{tenantID}", h.DeleteGrant)
}

// 鈹€鈹€ 闂ㄦ帶鍑芥暟锛堝鍑虹粰 plugin/skill 闆嗘垚鐐癸紝鐙珛鍙祴锛?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// marketItemLookup 鏌ヨ甯傚満鏉＄洰鐘舵€侊紱娴嬭瘯鍙暣浣撴浛鎹€?
// 杩斿洖 (鏄惁瀛樺湪鍚屽悕 published 鏉＄洰, 璇ョ鎴锋槸鍚﹀凡瀹夎涓斿惎鐢? error)銆?
var marketItemLookup = func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error) {
	pool := db.ReadPool()
	if pool == nil {
		return false, false, errors.New("market: postgres pool unavailable")
	}
	var published bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM ent_catalog_items
		 WHERE type = $1 AND name = $2 AND status = 'published')`,
		itemType, itemName).Scan(&published); err != nil {
		return false, false, err
	}
	if !published {
		return false, false, nil
	}
	var enabled bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM ent_catalog_installs i
		 JOIN ent_catalog_items c ON c.id = i.item_id
		 WHERE c.type = $1 AND c.name = $2 AND c.status = 'published'
		   AND i.tenant_id = $3 AND i.enabled)`,
		itemType, itemName, tenantID).Scan(&enabled)
	if err != nil {
		return false, false, err
	}
	return true, enabled, nil
}

// IsItemEnabledForTenant 鍒ゆ柇绉熸埛鏄惁鍙娇鐢ㄦ寚瀹氬競鍦鸿兘鍔涳紙plugin/skill 闂ㄦ帶锛夛細
//   - 甯傚満鏃犱换浣曞悓鍚?published 鏉＄洰 鈫?true锛堟湭涓婃灦鑳藉姏涓嶅彈甯傚満绠℃帶褰卞搷锛夛紱
//   - 鏈夊悓鍚?published 鏉＄洰 鈫?璇ョ鎴峰凡瀹夎涓?enabled 鎵?true锛?
//   - 鏌ヨ澶辫触 鈫?fail-open锛坱rue + slog.Warn锛夛紝淇濊瘉甯傚満鍩虹璁炬柦鏁呴殰涓嶉樆鏂兘鍔涗娇鐢ㄣ€?
func IsItemEnabledForTenant(ctx context.Context, itemType, itemName, tenantID string) (bool, error) {
	published, enabled, err := marketItemLookup(ctx, itemType, itemName, tenantID)
	if err != nil {
		slog.Warn("market gate: lookup failed, fail-open",
			"type", itemType, "name", itemName, "tenant", tenantID, "error", err)
		return true, nil
	}
	if !published {
		return true, nil
	}
	return enabled, nil
}

// ListEnabledMarketItems 杩斿洖绉熸埛宸插畨瑁呬笖鍚敤鐨?published 甯傚満鏉＄洰
// 锛堜緵鎻掍欢/鎶€鑳藉垪琛ㄥ彔鍔犲競鍦哄凡鎺堟潈椤癸級銆?
func ListEnabledMarketItems(ctx context.Context, itemType, tenantID string) ([]MarketItem, error) {
	pool := db.ReadPool()
	if pool == nil {
		return nil, errors.New("market: postgres pool unavailable")
	}
	rows, err := pool.Query(ctx,
		`SELECT c.id, c.type, c.name, c.version, c.manifest, c.status, c.created_by, c.created_at, c.updated_at
		 FROM ent_catalog_items c
		 JOIN ent_catalog_installs i ON i.item_id = c.id
		 WHERE c.type = $1 AND c.status = 'published' AND i.tenant_id = $2 AND i.enabled
		 ORDER BY c.name`, itemType, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []MarketItem{}
	for rows.Next() {
		it, err := scanMarketItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *it)
	}
	return items, rows.Err()
}

// 鈹€鈹€ 鍐呴儴杈呭姪 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

const catalogItemColumns = `id, type, name, version, manifest, status, created_by, created_at, updated_at`

func scanMarketItem(row interface{ Scan(...any) error }) (*MarketItem, error) {
	var it MarketItem
	if err := row.Scan(&it.ID, &it.Type, &it.Name, &it.Version, &it.Manifest,
		&it.Status, &it.CreatedBy, &it.CreatedAt, &it.UpdatedAt); err != nil {
		return nil, err
	}
	return &it, nil
}

// marketTenantID 涓庣瓥鐣?handler 涓€鑷寸殑绉熸埛瑙ｆ瀽锛坈laims 浼樺厛锛屽洖閫€榛樿绉熸埛锛夈€?
func marketTenantID(claims *auth.Claims) string {
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return DefaultTenantID
}

// 鈹€鈹€ 鐩綍鏉＄洰 CRUD 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// ListItems GET /v1/ent/market/items?type=&status=
func (h *MarketHandler) ListItems(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	itemType := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	if itemType != "" && !validMarketItemTypes[itemType] {
		BadRequest(w, "type must be plugin or skill")
		return
	}
	query := `SELECT ` + catalogItemColumns + ` FROM ent_catalog_items WHERE TRUE`
	args := []any{}
	if itemType != "" {
		args = append(args, itemType)
		query += ` AND type = $` + string(rune('0'+len(args)))
	}
	if status != "" {
		args = append(args, status)
		query += ` AND status = $` + string(rune('0'+len(args)))
	}
	query += ` ORDER BY created_at DESC LIMIT 500`

	pool := db.ReadPool()
	if pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	rows, err := pool.Query(r.Context(), query, args...)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list market items failed")
		return
	}
	defer rows.Close()
	items := []MarketItem{}
	for rows.Next() {
		it, err := scanMarketItem(rows)
		if err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "list market items failed")
			return
		}
		items = append(items, *it)
	}
	if err := rows.Err(); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list market items failed")
		return
	}
	OK(w, items)
}

// CreateItem POST /v1/ent/market/items锛堝垵濮嬬姸鎬?draft锛夈€?
func (h *MarketHandler) CreateItem(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	var body struct {
		Type     string          `json:"type"`
		Name     string          `json:"name"`
		Version  string          `json:"version"`
		Manifest json.RawMessage `json:"manifest"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if !validMarketItemTypes[body.Type] {
		BadRequest(w, "type must be plugin or skill")
		return
	}
	if !validMarketItemName.MatchString(body.Name) {
		BadRequest(w, "invalid item name")
		return
	}
	if body.Version == "" {
		body.Version = "1.0.0"
	}
	if len(body.Manifest) == 0 {
		body.Manifest = json.RawMessage(`{}`)
	}
	if !json.Valid(body.Manifest) {
		BadRequest(w, "manifest must be valid JSON")
		return
	}
	if db.Pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	created, err := scanMarketItem(db.Pool.QueryRow(r.Context(),
		`INSERT INTO ent_catalog_items (type, name, version, manifest, status, created_by)
		 VALUES ($1, $2, $3, $4, 'draft', $5) RETURNING `+catalogItemColumns,
		body.Type, body.Name, body.Version, string(body.Manifest), claims.UserID))
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create market item failed")
		return
	}
	Created(w, created)
}

// GetItem GET /v1/ent/market/items/{id}
func (h *MarketHandler) GetItem(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		BadRequest(w, "invalid item id")
		return
	}
	pool := db.ReadPool()
	if pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	it, err := scanMarketItem(pool.QueryRow(r.Context(),
		`SELECT `+catalogItemColumns+` FROM ent_catalog_items WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			NotFound(w, ErrNotFound)
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, "get market item failed")
		return
	}
	OK(w, it)
}

// UpdateItem PUT /v1/ent/market/items/{id}锛堜粎 name/version/manifest锛?
// 鐘舵€佹祦杞彧缁?publish/retire 绔偣锛夈€?
func (h *MarketHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		BadRequest(w, "invalid item id")
		return
	}
	var body struct {
		Name     *string          `json:"name"`
		Version  *string          `json:"version"`
		Manifest *json.RawMessage `json:"manifest"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.Name == nil && body.Version == nil && body.Manifest == nil {
		BadRequest(w, "nothing to update")
		return
	}
	if body.Name != nil && !validMarketItemName.MatchString(*body.Name) {
		BadRequest(w, "invalid item name")
		return
	}
	if body.Manifest != nil && !json.Valid(*body.Manifest) {
		BadRequest(w, "manifest must be valid JSON")
		return
	}
	// jsonb 鍒楀弬鏁颁互 string 浼犻€掞紙[]byte 浼氳 pgx 缂栫爜涓?bytea锛夛紱nil 鈫?NULL
	var manifest any
	if body.Manifest != nil {
		manifest = string(*body.Manifest)
	}
	if db.Pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	updated, err := scanMarketItem(db.Pool.QueryRow(r.Context(),
		`UPDATE ent_catalog_items SET
		     name = COALESCE($2, name),
		     version = COALESCE($3, version),
		     manifest = COALESCE($4, manifest),
		     updated_at = NOW()
		 WHERE id = $1 RETURNING `+catalogItemColumns,
		id, body.Name, body.Version, manifest))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			NotFound(w, ErrNotFound)
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, "update market item failed")
		return
	}
	OK(w, updated)
}

// DeleteItem DELETE /v1/ent/market/items/{id}锛堢骇鑱斿垹闄ゅ畨瑁呰褰曪級銆?
func (h *MarketHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		BadRequest(w, "invalid item id")
		return
	}
	if db.Pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	tag, err := db.Pool.Exec(r.Context(), `DELETE FROM ent_catalog_items WHERE id = $1`, id)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "delete market item failed")
		return
	}
	if tag.RowsAffected() == 0 {
		NotFound(w, ErrNotFound)
		return
	}
	NoContent(w)
}

// transitionItem 璇诲彇褰撳墠鐘舵€佸苟鏍￠獙鐘舵€佹満杩佺Щ锛岄€氳繃鍚庡師瀛?UPDATE銆?
func (h *MarketHandler) transitionItem(w http.ResponseWriter, r *http.Request, to string) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		BadRequest(w, "invalid item id")
		return
	}
	if db.Pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	var from string
	if err := db.Pool.QueryRow(r.Context(),
		`SELECT status FROM ent_catalog_items WHERE id = $1`, id).Scan(&from); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			NotFound(w, ErrNotFound)
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, "transition market item failed")
		return
	}
	if !canCatalogTransition(from, to) {
		JSON(w, http.StatusConflict, APIResponse{
			Success: false,
			Error:   "invalid status transition: " + from + " -> " + to,
		})
		return
	}
	updated, err := scanMarketItem(db.Pool.QueryRow(r.Context(),
		`UPDATE ent_catalog_items SET status = $2, updated_at = NOW()
		 WHERE id = $1 AND status = $3 RETURNING `+catalogItemColumns, id, to, from))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// 骞跺彂涓嬬姸鎬佸凡琚叾浠栬姹傚彉鏇?
			JSON(w, http.StatusConflict, APIResponse{
				Success: false,
				Error:   "invalid status transition (concurrent update)",
			})
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, "transition market item failed")
		return
	}
	OK(w, updated)
}

// PublishItem POST /v1/ent/market/items/{id}/publish锛坉raft 鈫?published锛夈€?
func (h *MarketHandler) PublishItem(w http.ResponseWriter, r *http.Request) {
	h.transitionItem(w, r, "published")
}

// RetireItem POST /v1/ent/market/items/{id}/retire锛坧ublished 鈫?retired锛夈€?
func (h *MarketHandler) RetireItem(w http.ResponseWriter, r *http.Request) {
	h.transitionItem(w, r, "retired")
}

// 鈹€鈹€ 绉熸埛鎺堟潈锛堝畨瑁呰褰曪級 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func scanMarketGrant(row interface{ Scan(...any) error }) (*MarketGrant, error) {
	var g MarketGrant
	if err := row.Scan(&g.ItemID, &g.TenantID, &g.Enabled, &g.InstalledAt); err != nil {
		return nil, err
	}
	return &g, nil
}

// ListGrants GET /v1/ent/market/grants?item_id=&tenant_id=
func (h *MarketHandler) ListGrants(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	query := `SELECT item_id, tenant_id, enabled, installed_at FROM ent_catalog_installs WHERE TRUE`
	args := []any{}
	if itemID := r.URL.Query().Get("item_id"); itemID != "" {
		args = append(args, itemID)
		query += ` AND item_id = $` + string(rune('0'+len(args)))
	}
	if tenantID := r.URL.Query().Get("tenant_id"); tenantID != "" {
		args = append(args, tenantID)
		query += ` AND tenant_id = $` + string(rune('0'+len(args)))
	}
	query += ` ORDER BY installed_at DESC LIMIT 500`

	pool := db.ReadPool()
	if pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	rows, err := pool.Query(r.Context(), query, args...)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list market grants failed")
		return
	}
	defer rows.Close()
	grants := []MarketGrant{}
	for rows.Next() {
		g, err := scanMarketGrant(rows)
		if err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "list market grants failed")
			return
		}
		grants = append(grants, *g)
	}
	if err := rows.Err(); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list market grants failed")
		return
	}
	OK(w, grants)
}

// GrantItem POST /v1/ent/market/grants锛圲PSERT锛歩tem_id + tenant_id 涓婚敭锛夈€?
func (h *MarketHandler) GrantItem(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	var body struct {
		ItemID   string `json:"item_id"`
		TenantID string `json:"tenant_id"`
		Enabled  *bool  `json:"enabled"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if _, err := uuid.Parse(body.ItemID); err != nil {
		BadRequest(w, "valid item_id (uuid) required")
		return
	}
	if _, err := uuid.Parse(body.TenantID); err != nil {
		BadRequest(w, "valid tenant_id (uuid) required")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	if db.Pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	// 鏉＄洰蹇呴』瀛樺湪锛堝閿厹搴曪紝鎻愬墠缁欏嚭鍙嬪ソ 404锛?
	var exists bool
	if err := db.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM ent_catalog_items WHERE id = $1)`, body.ItemID).Scan(&exists); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "grant market item failed")
		return
	}
	if !exists {
		NotFound(w, ErrNotFound)
		return
	}
	g, err := scanMarketGrant(db.Pool.QueryRow(r.Context(),
		`INSERT INTO ent_catalog_installs (item_id, tenant_id, enabled)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (item_id, tenant_id) DO UPDATE SET enabled = EXCLUDED.enabled
		 RETURNING item_id, tenant_id, enabled, installed_at`,
		body.ItemID, body.TenantID, enabled))
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "grant market item failed")
		return
	}
	Created(w, g)
}

// UpdateGrant PUT /v1/ent/market/grants/{itemID}/{tenantID}锛堜粎鍒囨崲 enabled锛夈€?
func (h *MarketHandler) UpdateGrant(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	itemID := r.PathValue("itemID")
	tenantID := r.PathValue("tenantID")
	if _, err := uuid.Parse(itemID); err != nil || uuidValidate(tenantID) != nil {
		BadRequest(w, "invalid item or tenant id")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if db.Pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	g, err := scanMarketGrant(db.Pool.QueryRow(r.Context(),
		`UPDATE ent_catalog_installs SET enabled = $3
		 WHERE item_id = $1 AND tenant_id = $2
		 RETURNING item_id, tenant_id, enabled, installed_at`, itemID, tenantID, body.Enabled))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			NotFound(w, ErrNotFound)
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, "update market grant failed")
		return
	}
	OK(w, g)
}

// DeleteGrant DELETE /v1/ent/market/grants/{itemID}/{tenantID}
func (h *MarketHandler) DeleteGrant(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	itemID := r.PathValue("itemID")
	tenantID := r.PathValue("tenantID")
	if _, err := uuid.Parse(itemID); err != nil || uuidValidate(tenantID) != nil {
		BadRequest(w, "invalid item or tenant id")
		return
	}
	if db.Pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}
	tag, err := db.Pool.Exec(r.Context(),
		`DELETE FROM ent_catalog_installs WHERE item_id = $1 AND tenant_id = $2`, itemID, tenantID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "delete market grant failed")
		return
	}
	if tag.RowsAffected() == 0 {
		NotFound(w, ErrNotFound)
		return
	}
	NoContent(w)
}
