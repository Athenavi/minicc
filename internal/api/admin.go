package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/engine"
	"github.com/athenavi/chiron/internal/monitor"
	"github.com/athenavi/chiron/internal/settings"
	"github.com/athenavi/chiron/internal/storage"
)

var validDBName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// AdminHandler provides admin-only management endpoints.
type AdminHandler struct {
	authenticator *auth.Authenticator
	store         *storage.AtomicStore
	redis         *db.AtomicRedis
	pythonClient  *engine.PythonClient
	rateLimiter   *DistributedRateLimiter
	appSecret     string
	settingsStore *settings.Store
}

func NewAdminHandler(a *auth.Authenticator, store *storage.AtomicStore, redis *db.AtomicRedis, pythonClient *engine.PythonClient) *AdminHandler {
	return &AdminHandler{authenticator: a, store: store, redis: redis, pythonClient: pythonClient}
}

// 閳光偓閳光偓 Routes 閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓

// RegisterRoutes adds admin endpoints to the given router under /v1/admin.
// Caller is responsible for auth middleware.
func (h *AdminHandler) RegisterRoutes(r *http.ServeMux) {
	// 鍘熸湁绔偣
	r.HandleFunc("GET /metrics", h.Metrics)
	r.HandleFunc("GET /users", h.ListUsers)
	r.HandleFunc("GET /users/{id}", h.GetUser)
	r.HandleFunc("PUT /users/{id}", h.UpdateUser)
	r.HandleFunc("DELETE /users/{id}", h.DeleteUser)
	r.HandleFunc("GET /system", h.SystemInfo)
	r.HandleFunc("POST /maintenance", h.TriggerMaintenance)
	r.HandleFunc("POST /backup", h.CreateBackup)
	r.HandleFunc("POST /restore", h.RestoreBackup)
	r.HandleFunc("GET /storage", h.GetStorage)
	r.HandleFunc("PUT /storage", h.UpdateStorage)
	r.HandleFunc("POST /storage/test", h.TestStorage)
	r.HandleFunc("GET /redis", h.GetRedis)
	r.HandleFunc("PUT /redis", h.UpdateRedis)
	r.HandleFunc("POST /redis/test", h.TestRedis)

	// 鏂板绔偣锛氶槦鍒楃鐞?	r.HandleFunc("GET /queue", h.GetQueueStats)
	r.HandleFunc("POST /queue/flush", h.FlushQueue)
	r.HandleFunc("POST /queue/pause", h.PauseQueue)

	// 鏂板绔偣锛氱紦瀛樼洃鎺?	r.HandleFunc("GET /cache/stats", h.GetCacheStats)

	// 鏂板绔偣锛氭€ц兘鐩戞帶
	r.HandleFunc("GET /performance", h.GetPerformance)

	// 鏂板绔偣锛欰PI Key 绠＄悊
	r.HandleFunc("GET /api-keys", h.ListApiKeys)
	// 杩愮淮绫荤鐐癸紙绉熸埛/鍩熷悕/鏁版嵁搴?Redis/妯″瀷/瀹氭椂浠诲姟锛夆€斺€?/admin 鍏ㄦ爤瀹炶
	h.registerOpsRoutes(r)
	r.HandleFunc("POST /api-keys", h.AddApiKey)
	r.HandleFunc("PUT /api-keys/{id}", h.UpdateApiKey)
	r.HandleFunc("DELETE /api-keys/{id}", h.DeleteApiKey)

	// 鏂板绔偣锛氱郴缁熻缃?	r.HandleFunc("PUT /settings", h.SaveSettings)
	r.HandleFunc("GET /settings", h.GetSettings)
}

// 閳光偓閳光偓 Metrics 閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓

func (h *AdminHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	snap := monitor.Snapshot()
	// Map internal metric names to dashboard-expected field names
	snap["concurrent_connections"] = snap["requests_active"]
	snap["queue_backlog"] = snap["requests_total"]
	snap["cache_hit_rate"] = 0
	snap["api_latency_p99"] = 0
	OK(w, snap)
}

// 閳光偓閳光偓 User Management 閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓

type AdminUser struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	tenantID := GetTenantID(r)
	if tenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
	}
	rows, err := db.ReadPool().Query(r.Context(),
		`SELECT id, email, name, role, created_at, updated_at
		 FROM users WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 100`,
		tenantID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "query users failed")
		return
	}
	defer rows.Close()

	users := make([]AdminUser, 0)
	for rows.Next() {
		var u AdminUser
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &createdAt, &updatedAt); err != nil {
			continue
		}
		u.CreatedAt = createdAt.Format(time.RFC3339)
		u.UpdatedAt = updatedAt.Format(time.RFC3339)
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		InternalError(w, "failed to iterate users")
		return
	}

	OK(w, users)
}

func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}
	tenantID := GetTenantID(r)
	if tenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
	}

	var u AdminUser
	var createdAt, updatedAt time.Time
	err := db.ReadPool().QueryRow(r.Context(),
		`SELECT id, email, name, role, created_at, updated_at
		 FROM users WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &createdAt, &updatedAt)
	if err != nil {
		NotFound(w, "user not found")
		return
	}
	u.CreatedAt = createdAt.Format(time.RFC3339)
	u.UpdatedAt = updatedAt.Format(time.RFC3339)

	OK(w, u)
}

func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}
	tenantID := GetTenantID(r)
	if tenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
	}

	var body struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}

	// Validate role
	if body.Role != "" && body.Role != "owner" && body.Role != "admin" && body.Role != "user" {
		BadRequest(w, "invalid role: must be owner, admin, or user")
		return
	}
	// S 瀹夊叏淇锛氶潪 owner 涓嶅彲灏嗚鑹叉彁鍗囦负 owner锛堥槻姝?admin 鎻愭潈锛?	claims := auth.GetClaims(r.Context())
	if body.Role == "owner" && (claims == nil || claims.Role != "owner") {
		BadRequest(w, "only owner can assign owner role")
		return
	}

	// Build dynamic UPDATE with column name whitelist 鈥?tenant_id 浣滀负棰濆 WHERE 鏉′欢闃茶秺鏉?	// S 瀹夊叏淇锛氬垪鍚嶅繀椤绘潵鑷櫧鍚嶅崟锛岄槻姝?SQL 娉ㄥ叆
	userColumnMap := map[string]string{
		"email": "email",
		"name":  "name",
		"role":  "role",
	}
	setClauses := ""
	args := []interface{}{}
	argIdx := 1

	fieldValues := []struct {
		field string
		value string
	}{
		{"email", body.Email},
		{"name", body.Name},
		{"role", body.Role},
	}
	for _, fv := range fieldValues {
		if fv.value != "" {
			col, ok := userColumnMap[fv.field]
			if !ok {
				continue
			}
			setClauses += fmt.Sprintf("%s = $%d, ", col, argIdx)
			args = append(args, fv.value)
			argIdx++
		}
	}

	if setClauses == "" {
		BadRequest(w, "no fields to update")
		return
	}

	setClauses += fmt.Sprintf("updated_at = NOW()")
	args = append(args, id, tenantID)

	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d AND tenant_id = $%d", setClauses, argIdx, argIdx+1)
	result, err := db.Pool.Exec(r.Context(), query, args...)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update user failed")
		return
	}
	if result.RowsAffected() == 0 {
		NotFound(w, "user not found")
		return
	}

	OK(w, map[string]string{"status": "updated"})
}

func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}
	tenantID := GetTenantID(r)
	if tenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
	}

	// Prevent deleting yourself
	claims := auth.GetClaims(r.Context())
	if claims != nil && claims.UserID == id {
		BadRequest(w, "cannot delete your own account")
		return
	}

	_, err := db.Pool.Exec(r.Context(),
		`DELETE FROM users WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "delete user failed")
		return
	}

	OK(w, map[string]string{"status": "deleted"})
}

// 閳光偓閳光偓 System Management 閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓閳光偓

func (h *AdminHandler) SystemInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"version": "2.0.0",
		"uptime":  time.Since(monitor.Global.StartTime).String(),
		"db": map[string]interface{}{
			"postgres": true,
			"redis":    db.Redis != nil,
		},
	}
	OK(w, info)
}

func (h *AdminHandler) TriggerMaintenance(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"` // vacuum | reindex | analyze | flush_cache
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "action is required (vacuum, reindex, analyze, flush_cache)")
		return
	}
	if body.Action == "" {
		BadRequest(w, "action is required (vacuum, reindex, analyze, flush_cache)")
		return
	}

	switch body.Action {
	case "vacuum":
		if _, err := db.Pool.Exec(r.Context(), "VACUUM ANALYZE"); err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "vacuum failed")
			return
		}
	case "reindex":
		dbName := dbNameFromDSN()
		if !validDBName.MatchString(dbName) {
			InternalError(w, "invalid database name")
			return
		}
		if _, err := db.Pool.Exec(r.Context(), fmt.Sprintf("REINDEX DATABASE %s", dbName)); err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "reindex failed")
			return
		}
	case "analyze":
		if _, err := db.Pool.Exec(r.Context(), "ANALYZE"); err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "analyze failed")
			return
		}
	case "flush_cache":
		if db.Redis != nil {
			const prefix = "chiron_cache:*"
			iter := db.Redis.Scan(r.Context(), 0, prefix, 0).Iterator()
			var deleted int
			for iter.Next(r.Context()) {
				db.Redis.Del(r.Context(), iter.Val())
				deleted++
			}
			if err := iter.Err(); err != nil {
				logAndRespond(w, err, http.StatusInternalServerError, "flush_cache failed")
				return
			}
			slog.Info("cache flushed", "prefix", prefix, "deleted", deleted)
		}
	default:
		BadRequest(w, fmt.Sprintf("unknown action: %s", body.Action))
		return
	}

	OK(w, map[string]string{
		"status": "completed",
		"action": body.Action,
	})
}

// dbNameFromDSN extracts the database name from POSTGRES_DSN environment variable.
func dbNameFromDSN() string {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		return "chiron" // fallback
	}
	// Parse URL format: postgres://user:pass@host:port/dbname?params
	u, err := url.Parse(dsn)
	if err != nil {
		return "chiron"
	}
	if u.Path != "" && u.Path != "/" {
		// Path is /dbname 閳?trim leading slash
		return u.Path[1:]
	}
	return "chiron"
}

// 閳光偓閳光偓 Backup & Restore 閳光偓閳光偓

func (h *AdminHandler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	// P0-P4 淇锛歱g_dump 杈撳嚭娴佸紡杞彂锛岄伩鍏嶆暣搴撶紦鍐插叆鍐呭瓨瀵艰嚧 OOM
	cmd := exec.CommandContext(r.Context(), "pg_dump", "--dbname="+extractDSN())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "backup failed")
		return
	}
	if err := cmd.Start(); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "backup failed")
		return
	}
	w.Header().Set("Content-Type", "application/sql")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=chiron_backup_%s.sql", time.Now().Format("20060102_150405")))
	if _, err := io.Copy(w, stdout); err != nil {
		slog.Warn("backup stream failed", "error", err)
	}
	if err := cmd.Wait(); err != nil {
		slog.Warn("pg_dump failed", "error", err)
	}
}

func (h *AdminHandler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		BadRequest(w, "file is required")
		return
	}
	defer file.Close()
	// P0-P4 闃叉姢锛氶檺鍒舵仮澶嶆枃浠跺ぇ灏忥紝閬垮厤鏁存枃浠惰鍏ュ唴瀛?	sqlData, err := io.ReadAll(io.LimitReader(file, 512<<20))
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "read file failed")
		return
	}
	if len(sqlData) >= 512<<20 {
		BadRequest(w, "backup file too large (max 512MB)")
		return
	}
	tx, err := db.Pool.Begin(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "begin tx failed")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), string(sqlData)); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "restore failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "commit failed")
		return
	}
	OK(w, map[string]string{"message": "Database restored successfully"})
}

func extractDSN() string {
	return os.Getenv("POSTGRES_DSN")
}

// 鈹€鈹€鈹€ Storage Management 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

type StorageConfig struct {
	Backend     string `json:"backend"`
	StorageRoot string `json:"storage_root,omitempty"`
	S3Endpoint  string `json:"s3_endpoint,omitempty"`
	S3Bucket    string `json:"s3_bucket,omitempty"`
	S3UseSSL    bool   `json:"s3_use_ssl"`
}

type StorageUpdateRequest struct {
	Backend     string `json:"backend"`
	S3Endpoint  string `json:"s3_endpoint,omitempty"`
	S3Bucket    string `json:"s3_bucket,omitempty"`
	S3AccessKey string `json:"s3_access_key,omitempty"`
	S3SecretKey string `json:"s3_secret_key,omitempty"`
	S3UseSSL    bool   `json:"s3_use_ssl,omitempty"`
}

func (h *AdminHandler) GetStorage(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		OK(w, map[string]interface{}{
			"backend": "none",
			"config":  StorageConfig{},
		})
		return
	}
	OK(w, map[string]interface{}{
		"backend": h.store.Backend(),
		"config":  StorageConfig{},
	})
}

func (h *AdminHandler) UpdateStorage(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		InternalError(w, "storage not initialized")
		return
	}

	var body StorageUpdateRequest
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}

	if body.Backend != "local" && body.Backend != "s3" {
		BadRequest(w, "backend must be 'local' or 's3'")
		return
	}

	previous := h.store.Backend()

	var newStore storage.FileStore
	var err error
	switch body.Backend {
	case "local":
		root := "./workspace"
		if ls, ok := h.store.LoadRaw().(*storage.LocalStore); ok {
			root = ls.Root
		}
		newStore, err = storage.NewStore("local", root, "", "", "", "", false)
	case "s3":
		if body.S3Endpoint == "" || body.S3Bucket == "" || body.S3AccessKey == "" || body.S3SecretKey == "" {
			BadRequest(w, "s3_endpoint, s3_bucket, s3_access_key, s3_secret_key are required for S3 backend")
			return
		}
		newStore, err = storage.NewStore("s3", "", body.S3Endpoint, body.S3Bucket, body.S3AccessKey, body.S3SecretKey, body.S3UseSSL)
	}
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "failed to create storage backend")
		return
	}

	h.store.Swap(newStore)

	warning := ""
	if previous != body.Backend {
		if previous == "local" {
			warning = "瀛樺偍鍚庣宸蹭粠 local 鍒囨崲涓?s3銆傛棫鍚庣涓殑鏂囦欢涓嶄細鑷姩杩佺Щ銆?
		} else {
			warning = "瀛樺偍鍚庣宸蹭粠 s3 鍒囨崲涓?local銆傛棫鍚庣涓殑鏂囦欢涓嶄細鑷姩杩佺Щ銆?
		}
	}

	OK(w, map[string]interface{}{
		"status":   "switched",
		"warning":  warning,
		"previous": previous,
		"current":  body.Backend,
	})

	slog.Info("storage backend switched", "from", previous, "to", body.Backend)
}

func (h *AdminHandler) TestStorage(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		InternalError(w, "storage not initialized")
		return
	}

	var body StorageUpdateRequest
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}

	switch body.Backend {
	case "local":
		OK(w, map[string]interface{}{
			"status":  "ok",
			"message": "鏈湴瀛樺偍鍙敤",
		})
	case "s3":
		if body.S3Endpoint == "" || body.S3Bucket == "" || body.S3AccessKey == "" || body.S3SecretKey == "" {
			BadRequest(w, "s3_endpoint, s3_bucket, s3_access_key, s3_secret_key are required")
			return
		}
		testStore, err := storage.NewS3Store(body.S3Endpoint, body.S3Bucket, "", body.S3AccessKey, body.S3SecretKey, "", body.S3UseSSL)
		if err != nil {
			OK(w, map[string]interface{}{
				"status":  "error",
				"message": fmt.Errorf("S3 杩炴帴澶辫触: %w", err).Error(),
			})
			return
		}
		ctx := r.Context()
		_, err = testStore.List(ctx, "")
		if err != nil {
			OK(w, map[string]interface{}{
				"status":  "error",
				"message": fmt.Errorf("S3 bucket 璁块棶澶辫触: %w", err).Error(),
			})
			return
		}
		OK(w, map[string]interface{}{
			"status":  "ok",
			"message": fmt.Sprintf("S3 杩炴帴鎴愬姛锛宐ucket '%s' 鍙闂?, body.S3Bucket),
		})
	default:
		BadRequest(w, "backend must be 'local' or 's3'")
	}
}

// 鈹€鈹€鈹€ Redis Management 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func (h *AdminHandler) GetRedis(w http.ResponseWriter, r *http.Request) {
	if h.redis == nil {
		OK(w, map[string]interface{}{
			"status": "disconnected",
			"mode":   "none",
		})
		return
	}
	stats := h.redis.Stats()
	OK(w, map[string]interface{}{
		"status": "connected",
		"mode":   h.redis.Mode(),
		"pool": map[string]interface{}{
			"hits":        stats.Hits,
			"misses":      stats.Misses,
			"timeouts":    stats.Timeouts,
			"total_conns": stats.TotalConns,
			"idle_conns":  stats.IdleConns,
			"stale_conns": stats.StaleConns,
		},
	})
}

func (h *AdminHandler) UpdateRedis(w http.ResponseWriter, r *http.Request) {
	if h.redis == nil {
		InternalError(w, "redis not initialized")
		return
	}

	var body db.RedisConfig
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}

	if body.Mode == "" {
		body.Mode = "single"
	}

	switch body.Mode {
	case "single":
		if body.Addr == "" {
			BadRequest(w, "addr is required for single mode")
			return
		}
	case "cluster":
		if len(body.Addrs) == 0 {
			BadRequest(w, "addrs is required for cluster mode")
			return
		}
	case "sentinel":
		if body.MasterName == "" {
			BadRequest(w, "master_name is required for sentinel mode")
			return
		}
		if len(body.SentinelAddrs) == 0 {
			BadRequest(w, "sentinel_addrs is required for sentinel mode")
			return
		}
	default:
		BadRequest(w, "mode must be 'single', 'cluster', or 'sentinel'")
		return
	}

	newClient, err := db.NewRedisClient(body)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "failed to connect to redis")
		return
	}

	oldClient := h.redis.LoadRaw()
	h.redis.Swap(newClient)
	if oldClient != nil {
		oldClient.Close()
	}

	OK(w, map[string]interface{}{
		"status":  "switched",
		"mode":    body.Mode,
		"warning": "Redis connection switched. Cached data from the previous instance is not migrated.",
	})

	slog.Info("redis backend switched", "mode", body.Mode)
}

func (h *AdminHandler) TestRedis(w http.ResponseWriter, r *http.Request) {
	var body db.RedisConfig
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}

	if body.Mode == "" {
		body.Mode = "single"
	}

	newClient, err := db.NewRedisClient(body)
	if err != nil {
		OK(w, map[string]interface{}{
			"status":  "error",
			"message": fmt.Errorf("Redis connection failed: %w", err).Error(),
		})
		return
	}
	newClient.Close()

	OK(w, map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Redis %s connection successful", body.Mode),
	})
}
