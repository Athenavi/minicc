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

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/engine"
	"github.com/athenavi/minicc/internal/monitor"
	"github.com/athenavi/minicc/internal/storage"
)

var validDBName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// AdminHandler provides admin-only management endpoints.
type AdminHandler struct {
	authenticator *auth.Authenticator
	store         *storage.AtomicStore
	redis         *db.AtomicRedis
	pythonClient  *engine.PythonClient
}

func NewAdminHandler(a *auth.Authenticator, store *storage.AtomicStore, redis *db.AtomicRedis, pythonClient *engine.PythonClient) *AdminHandler {
	return &AdminHandler{authenticator: a, store: store, redis: redis, pythonClient: pythonClient}
}

// 鈹€鈹€ Routes 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// RegisterRoutes adds admin endpoints to the given router under /v1/admin.
// Caller is responsible for auth middleware.
func (h *AdminHandler) RegisterRoutes(r *http.ServeMux) {
	// 原有端点
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

	// 新增端点：队列管理
	r.HandleFunc("GET /queue", h.GetQueueStats)
	r.HandleFunc("POST /queue/flush", h.FlushQueue)
	r.HandleFunc("POST /queue/pause", h.PauseQueue)

	// 新增端点：缓存监控
	r.HandleFunc("GET /cache/stats", h.GetCacheStats)

	// 新增端点：性能监控
	r.HandleFunc("GET /performance", h.GetPerformance)

	// 新增端点：API Key 管理
	r.HandleFunc("GET /api-keys", h.ListApiKeys)
	r.HandleFunc("POST /api-keys", h.AddApiKey)
	r.HandleFunc("PUT /api-keys/{id}", h.UpdateApiKey)
	r.HandleFunc("DELETE /api-keys/{id}", h.DeleteApiKey)

	// 新增端点：系统设置
	r.HandleFunc("PUT /settings", h.SaveSettings)
}

// 鈹€鈹€ Metrics 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func (h *AdminHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	snap := monitor.Snapshot()
	// Map internal metric names to dashboard-expected field names
	snap["concurrent_connections"] = snap["requests_active"]
	snap["queue_backlog"] = snap["requests_total"]
	snap["cache_hit_rate"] = 0
	snap["api_latency_p99"] = 0
	OK(w, snap)
}

// 鈹€鈹€ User Management 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

type AdminUser struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		OK(w, []AdminUser{})
		return
	}

	rows, err := db.ReadPool().Query(r.Context(),
		`SELECT id, email, name, role, created_at, updated_at
		 FROM users ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		InternalError(w, "query users: "+err.Error())
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
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	var u AdminUser
	var createdAt, updatedAt time.Time
	err := db.ReadPool().QueryRow(r.Context(),
		`SELECT id, email, name, role, created_at, updated_at
		 FROM users WHERE id = $1`, id).
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
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	var body struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	// Validate role
	if body.Role != "" && body.Role != "owner" && body.Role != "admin" && body.Role != "user" {
		BadRequest(w, "invalid role: must be owner, admin, or user")
		return
	}

	// Build dynamic UPDATE
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
	if body.Role != "" {
		setClauses += fmt.Sprintf("role = $%d, ", argIdx)
		args = append(args, body.Role)
		argIdx++
	}

	if setClauses == "" {
		BadRequest(w, "no fields to update")
		return
	}

	setClauses += fmt.Sprintf("updated_at = NOW()")
	args = append(args, id)

	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d", setClauses, argIdx)
	result, err := db.Pool.Exec(r.Context(), query, args...)
	if err != nil {
		InternalError(w, "update user: "+err.Error())
		return
	}
	if result.RowsAffected() == 0 {
		NotFound(w, "user not found")
		return
	}

	OK(w, map[string]string{"status": "updated"})
}

func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	// Prevent deleting yourself
	claims := auth.GetClaims(r.Context())
	if claims != nil && claims.UserID == id {
		BadRequest(w, "cannot delete your own account")
		return
	}

	_, err := db.Pool.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		InternalError(w, "delete user: "+err.Error())
		return
	}

	OK(w, map[string]string{"status": "deleted"})
}

// 鈹€鈹€ System Management 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func (h *AdminHandler) SystemInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"version":   "2.0.0",
		"uptime":    time.Since(monitor.Global.StartTime).String(),
		"db":        map[string]interface{}{
			"postgres": db.Pool != nil,
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
		if db.Pool != nil {
			if _, err := db.Pool.Exec(r.Context(), "VACUUM ANALYZE"); err != nil {
				InternalError(w, fmt.Sprintf("vacuum failed: %v", err))
				return
			}
		}
	case "reindex":
		if db.Pool != nil {
			dbName := dbNameFromDSN()
			if !validDBName.MatchString(dbName) {
				InternalError(w, fmt.Sprintf("invalid database name: %q", dbName))
				return
			}
			if _, err := db.Pool.Exec(r.Context(), fmt.Sprintf("REINDEX DATABASE %s", dbName)); err != nil {
				InternalError(w, fmt.Sprintf("reindex failed: %v", err))
				return
			}
		}
	case "analyze":
		if db.Pool != nil {
			if _, err := db.Pool.Exec(r.Context(), "ANALYZE"); err != nil {
				InternalError(w, fmt.Sprintf("analyze failed: %v", err))
				return
			}
		}
	case "flush_cache":
		if db.Redis != nil {
			const prefix = "minicc_cache:*"
			iter := db.Redis.Scan(r.Context(), 0, prefix, 0).Iterator()
			var deleted int
			for iter.Next(r.Context()) {
				db.Redis.Del(r.Context(), iter.Val())
				deleted++
			}
			if err := iter.Err(); err != nil {
				InternalError(w, fmt.Sprintf("flush_cache scan failed: %v", err))
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
		return "minicc" // fallback
	}
	// Parse URL format: postgres://user:pass@host:port/dbname?params
	u, err := url.Parse(dsn)
	if err != nil {
		return "minicc"
	}
	if u.Path != "" && u.Path != "/" {
		// Path is /dbname 鈥?trim leading slash
		return u.Path[1:]
	}
	return "minicc"
}

// 鈹€鈹€ Backup & Restore 鈹€鈹€

func (h *AdminHandler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		InternalError(w, "database not available")
		return
	}
	output, err := exec.Command("pg_dump", "--dbname="+extractDSN()).Output()
	if err != nil {
		InternalError(w, fmt.Sprintf("backup failed: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/sql")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=minicc_backup_%s.sql", time.Now().Format("20060102_150405")))
	w.Write(output)
}

func (h *AdminHandler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		InternalError(w, "database not available")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		BadRequest(w, "file is required")
		return
	}
	defer file.Close()
	sqlData, err := io.ReadAll(file)
	if err != nil {
		InternalError(w, fmt.Sprintf("read file failed: %v", err))
		return
	}
	tx, err := db.Pool.Begin(r.Context())
	if err != nil {
		InternalError(w, fmt.Sprintf("begin tx failed: %v", err))
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), string(sqlData)); err != nil {
		InternalError(w, fmt.Sprintf("restore failed: %v", err))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		InternalError(w, fmt.Sprintf("commit failed: %v", err))
		return
	}
	OK(w, map[string]string{"message": "Database restored successfully"})
}

func extractDSN() string {
	return os.Getenv("POSTGRES_DSN")
}

// ─── Storage Management ────────────────────────────────────────────

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
		BadRequest(w, "invalid request body")
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
		InternalError(w, "failed to create storage backend: "+err.Error())
		return
	}

	h.store.Swap(newStore)

	warning := ""
	if previous != body.Backend {
		if previous == "local" {
			warning = "存储后端已从 local 切换为 s3。旧后端中的文件不会自动迁移。"
		} else {
			warning = "存储后端已从 s3 切换为 local。旧后端中的文件不会自动迁移。"
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
		BadRequest(w, "invalid request body")
		return
	}

	switch body.Backend {
	case "local":
		OK(w, map[string]interface{}{
			"status":  "ok",
			"message": "本地存储可用",
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
				"message": fmt.Sprintf("S3 连接失败: %v", err),
			})
			return
		}
		ctx := r.Context()
		_, err = testStore.List(ctx, "")
		if err != nil {
			OK(w, map[string]interface{}{
				"status":  "error",
				"message": fmt.Sprintf("S3 bucket 访问失败: %v", err),
			})
			return
		}
		OK(w, map[string]interface{}{
			"status":  "ok",
			"message": fmt.Sprintf("S3 连接成功，bucket '%s' 可访问", body.S3Bucket),
		})
	default:
		BadRequest(w, "backend must be 'local' or 's3'")
	}
}

// ─── Redis Management ────────────────────────────────────────────

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
		BadRequest(w, "invalid request body")
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
		InternalError(w, "failed to connect to new redis: "+err.Error())
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
		BadRequest(w, "invalid request body")
		return
	}

	if body.Mode == "" {
		body.Mode = "single"
	}

	newClient, err := db.NewRedisClient(body)
	if err != nil {
		OK(w, map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Redis connection failed: %v", err),
		})
		return
	}
	newClient.Close()

	OK(w, map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Redis %s connection successful", body.Mode),
	})
}
