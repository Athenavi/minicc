package api

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
)

// 鈹€鈹€ 浼佷笟瀹¤鏃ュ織鏌ヨ 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// EntAuditHandler 鎻愪緵浼佷笟瀹¤鏃ュ織鏌ヨ API銆?
// 璺敱娉ㄥ唽鐢遍泦鎴愪换鍔＄粺涓€鎺ュ叆锛堟湰浠诲姟涓嶆敞鍐岋級锛?
//
//	auditHandler := api.NewEntAuditHandler()
//	auditHandler.RegisterRoutes(mux, authMW)
type EntAuditHandler struct{}

// NewEntAuditHandler 鍒涘缓瀹¤鏌ヨ handler銆?
func NewEntAuditHandler() *EntAuditHandler { return &EntAuditHandler{} }

// RegisterRoutes 鎸傝浇瀹¤璺敱锛坅uthMW + RequireEntPerm("audit:read")锛夈€?
func (h *EntAuditHandler) RegisterRoutes(mux *http.ServeMux, authMW func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/ent/audit", authMW(RequireEntPerm("audit:read")(http.HandlerFunc(h.Query))))
}

// auditQueryFilter 瀹¤鏌ヨ鐨勫綊涓€鍖栬繃婊ゆ潯浠讹紙鏃堕棿鑼冨洿涓庡垎椤靛凡寮哄埗鏀舵暃锛夈€?
type auditQueryFilter struct {
	TenantID     string
	UserID       string
	Action       string
	ResourceType string
	From         time.Time
	To           time.Time
	Page         int
	PageSize     int
}

const auditMaxRange = 7 * 24 * time.Hour

// parseAuditQuery 瑙ｆ瀽骞跺己鍒舵敹鏁涙煡璇㈠弬鏁帮紙绾嚱鏁帮紝鐙珛鍙祴锛夛細
//   - 鏃堕棿鑼冨洿蹇呭～璇箟锛歠rom/to 缂虹渷鏃跺己鍒舵渶杩?7 澶╋紱
//     鏄惧紡鑼冨洿瓒呰繃 7 澶╂椂鏀舵暃涓?to-7d ~ to锛沠rom >= to 鎶ラ敊锛?
//   - 寮哄埗鍒嗛〉锛歱age>=1锛宲age_size 缂虹渷 50銆佷笂闄?100銆?
func parseAuditQuery(q url.Values, now time.Time) (auditQueryFilter, error) {
	f := auditQueryFilter{
		UserID:       q.Get("user_id"),
		Action:       q.Get("action"),
		ResourceType: q.Get("resource_type"),
		Page:         1,
		PageSize:     50,
	}

	parse := func(s string) (time.Time, error) {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.UTC(), nil
		}
		return time.Parse(time.RFC3339, s)
	}

	f.To = now.UTC()
	f.From = f.To.Add(-auditMaxRange)
	if toStr := q.Get("to"); toStr != "" {
		t, err := parse(toStr)
		if err != nil {
			return f, fmt.Errorf("invalid to: %w", err)
		}
		f.To = t.UTC()
	}
	if fromStr := q.Get("from"); fromStr != "" {
		t, err := parse(fromStr)
		if err != nil {
			return f, fmt.Errorf("invalid from: %w", err)
		}
		f.From = t.UTC()
	}
	if !f.From.Before(f.To) {
		return f, fmt.Errorf("from must be before to")
	}
	// 鏃堕棿鑼冨洿涓婇檺 7 澶╋紙闃叉鍏ㄨ〃鎵弿锛屼繚璇佸懡涓?idx_audit_logs_tenant_time锛?
	if f.To.Sub(f.From) > auditMaxRange {
		f.From = f.To.Add(-auditMaxRange)
	}

	if v := q.Get("page"); v != "" {
		n := 0
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 1 {
			return f, fmt.Errorf("invalid page")
		}
		f.Page = n
	}
	if v := q.Get("page_size"); v != "" {
		n := 0
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 1 {
			return f, fmt.Errorf("invalid page_size")
		}
		f.PageSize = n
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	return f, nil
}

// auditLogRow 瀹¤鏌ヨ鍝嶅簲琛屻€?
type auditLogRow struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	UserID       *string   `json:"user_id"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   *string   `json:"resource_id"`
	Details      any       `json:"details"`
	IPAddress    *string   `json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
}

// Query GET /v1/ent/audit?user_id=&action=&resource_type=&from=&to=&page=&page_size=
// 绉熸埛闅旂锛歝laims.TenantID 浼樺厛锛岀己鐪佸洖閫€榛樿绉熸埛锛涚函 SQL 璧?
// idx_audit_logs_tenant_time(tenant_id, created_at DESC) 绱㈠紩銆?
func (h *EntAuditHandler) Query(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	f, err := parseAuditQuery(r.URL.Query(), time.Now())
	if err != nil {
		BadRequest(w, err.Error())
		return
	}
	f.TenantID = entPolicyTenantID(claims)

	pool := db.ReadPool()
	if pool == nil {
		ServiceUnavailable(w, ErrDBUnavailable)
		return
	}

	where := ` WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3`
	args := []any{f.TenantID, f.From, f.To}
	if f.UserID != "" {
		args = append(args, f.UserID)
		where += fmt.Sprintf(` AND user_id = $%d`, len(args))
	}
	if f.Action != "" {
		args = append(args, f.Action)
		where += fmt.Sprintf(` AND action = $%d`, len(args))
	}
	if f.ResourceType != "" {
		args = append(args, f.ResourceType)
		where += fmt.Sprintf(` AND resource_type = $%d`, len(args))
	}

	var total int
	if err := pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM audit_logs`+where, args...).Scan(&total); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "audit query failed")
		return
	}

	offset := (f.Page - 1) * f.PageSize
	rows, err := pool.Query(r.Context(),
		`SELECT id, tenant_id, user_id, action, resource_type, resource_id,
		        details, ip_address, created_at
		 FROM audit_logs`+where+`
		 ORDER BY created_at DESC
		 LIMIT $`+fmt.Sprint(len(args)+1)+` OFFSET $`+fmt.Sprint(len(args)+2),
		append(args, f.PageSize, offset)...)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "audit query failed")
		return
	}
	defer rows.Close()

	out := []auditLogRow{}
	for rows.Next() {
		var row auditLogRow
		if err := rows.Scan(&row.ID, &row.TenantID, &row.UserID, &row.Action,
			&row.ResourceType, &row.ResourceID, &row.Details, &row.IPAddress,
			&row.CreatedAt); err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "audit query failed")
			return
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "audit query failed")
		return
	}

	JSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    out,
		Meta:    &Meta{Total: total, Page: f.Page, PerPage: f.PageSize},
	})
}
