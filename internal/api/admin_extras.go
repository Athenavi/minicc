package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/monitor"
	"github.com/athenavi/minicc/internal/settings"
)

// ── Queue Stats ──

type QueueStats struct {
	TaskQueueLength int         `json:"task_queue_length"`
	VIPQueueLength  int         `json:"vip_queue_length"`
	Consumers       int         `json:"consumers"`
	ThroughputQPS   float64     `json:"throughput_qps"`
	AvgWaitMs       int         `json:"avg_wait_ms"`
	MaxWaitMs       int         `json:"max_wait_ms"`
	WaitingTasks    []QueueTask `json:"waiting_tasks"`
}

type QueueTask struct {
	TaskID   string `json:"task_id"`
	UserID   string `json:"user_id"`
	Content  string `json:"content"`
	QueuedAt string `json:"queued_at"`
	Position int    `json:"position"`
	IsVIP    bool   `json:"is_vip"`
}

func (h *AdminHandler) GetQueueStats(w http.ResponseWriter, r *http.Request) {
	stats := QueueStats{
		WaitingTasks: []QueueTask{},
	}

	// 从 Redis 获取队列长度
	if db.Redis != nil {
		ctx := r.Context()
		taskLen, _ := db.Redis.Get(ctx, "queue:tasks:length").Int64()
		vipLen, _ := db.Redis.Get(ctx, "queue:vip:length").Int64()
		stats.TaskQueueLength = int(taskLen)
		stats.VIPQueueLength = int(vipLen)
	}

	OK(w, stats)
}

var queuePaused atomic.Bool

func (h *AdminHandler) FlushQueue(w http.ResponseWriter, r *http.Request) {
	if db.Redis == nil {
		InternalError(w, "redis not available")
		return
	}

	ctx := r.Context()
	// 通过设置标志通知 worker 清空队列
	db.Redis.Set(ctx, "queue:flush", "1", 10*time.Second)

	OK(w, map[string]string{"status": "flush_requested"})
}

func (h *AdminHandler) PauseQueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Pause bool `json:"pause"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}

	queuePaused.Store(body.Pause)

	// 通过 Redis 通知所有 worker
	if db.Redis != nil {
		ctx := r.Context()
		if body.Pause {
			db.Redis.Set(ctx, "queue:paused", "1", 0)
		} else {
			db.Redis.Del(ctx, "queue:paused")
		}
	}

	OK(w, map[string]interface{}{"paused": body.Pause})
}

// ── Cache Stats ──

type CacheStats struct {
	L1HitRate     float64    `json:"l1_hit_rate"`
	L2HitRate     float64    `json:"l2_hit_rate"`
	L3HitRate     float64    `json:"l3_hit_rate"`
	TotalHitRate  float64    `json:"total_hit_rate"`
	TotalRequests int64      `json:"total_requests"`
	TotalHits     int64      `json:"total_hits"`
	TotalMisses   int64      `json:"total_misses"`
	AvgLatencyMs  float64    `json:"avg_latency_ms"`
	L1Size        int        `json:"l1_size"`
	L1Capacity    int        `json:"l1_capacity"`
	HotQueries    []HotQuery `json:"hot_queries"`
}

type HotQuery struct {
	Query        string  `json:"query"`
	Hits         int     `json:"hits"`
	HitRate      float64 `json:"hit_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

func (h *AdminHandler) GetCacheStats(w http.ResponseWriter, r *http.Request) {
	stats := CacheStats{
		HotQueries: []HotQuery{},
	}

	// 从 Redis 获取缓存统计
	if db.Redis != nil {
		ctx := r.Context()
		hits, _ := db.Redis.Get(ctx, "cache:stats:hits").Int64()
		misses, _ := db.Redis.Get(ctx, "cache:stats:misses").Int64()
		stats.TotalHits = hits
		stats.TotalMisses = misses
		stats.TotalRequests = hits + misses

		if stats.TotalRequests > 0 {
			stats.TotalHitRate = float64(hits) / float64(stats.TotalRequests) * 100
		}
	}

	OK(w, stats)
}

// ── Performance Stats ──

type PerformanceStats struct {
	Gateway GatewayStats `json:"gateway"`
	Python  PythonStats  `json:"python_engine"`
}

type GatewayStats struct {
	Instances      int     `json:"instances"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryMB       float64 `json:"memory_mb"`
	Goroutines     int     `json:"goroutines"`
	Connections    int     `json:"connections"`
	RedisLatencyMs float64 `json:"redis_latency_ms"`
	DBLatencyMs    float64 `json:"db_latency_ms"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
	Version        string  `json:"version"`
}

type PythonStats struct {
	Pods           int     `json:"pods"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryMB       float64 `json:"memory_mb"`
	ActiveTasks    int     `json:"active_tasks"`
	AvgInferenceMs float64 `json:"avg_inference_ms"`
	RedisLatencyMs float64 `json:"redis_latency_ms"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
	Version        string  `json:"version"`
}

func (h *AdminHandler) GetPerformance(w http.ResponseWriter, r *http.Request) {
	snapshot := monitor.Snapshot()

	stats := PerformanceStats{
		Gateway: GatewayStats{
			Version: "3.0.0",
		},
		Python: PythonStats{
			Version: "3.0.0",
		},
	}

	// 从 monitor snapshot 提取数据
	if v, ok := snapshot["goroutines"]; ok {
		if n, ok := v.(int); ok {
			stats.Gateway.Goroutines = n
		}
	}

	// 测量 Redis 延迟
	if db.Redis != nil {
		ctx := r.Context()
		start := time.Now()
		db.Redis.Ping(ctx)
		stats.Gateway.RedisLatencyMs = float64(time.Since(start).Microseconds()) / 1000
	}

	// 测量 DB 延迟
	ctx := r.Context()
	start := time.Now()
	db.Pool.Ping(ctx)
	stats.Gateway.DBLatencyMs = float64(time.Since(start).Microseconds()) / 1000

	OK(w, stats)
}

// ── API Keys ──

type ApiKey struct {
	ID         string `json:"id"`
	Provider   string `json:"provider"`
	KeyPreview string `json:"key_preview"`
	Status     string `json:"status"`
	Weight     int    `json:"weight"`
	Failures   int    `json:"failures"`
	LastUsed   string `json:"last_used"`
	Remark     string `json:"remark"`
}

func (h *AdminHandler) ListApiKeys(w http.ResponseWriter, r *http.Request) {
	if h.pythonClient == nil {
		OK(w, map[string]interface{}{"keys": []ApiKey{}, "stats": map[string]interface{}{"total": 0, "active": 0, "rate_limited": 0, "circuit_open": 0}})
		return
	}
	h.pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys")
}

func (h *AdminHandler) AddApiKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		Key      string `json:"key"`
		Remark   string `json:"remark"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}

	if body.Provider == "" || body.Key == "" {
		BadRequest(w, "provider and key are required")
		return
	}

	if h.pythonClient == nil {
		InternalError(w, "python engine not available")
		return
	}
	var resp interface{}
	if err := h.pythonClient.PostJSON(r.Context(), "/v1/admin/api-keys", body, &resp); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "python engine error")
		return
	}
	OK(w, resp)
}

func (h *AdminHandler) UpdateApiKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	if h.pythonClient == nil {
		InternalError(w, "python engine not available")
		return
	}
	h.pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys/"+id)
}

func (h *AdminHandler) DeleteApiKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	if h.pythonClient == nil {
		InternalError(w, "python engine not available")
		return
	}
	h.pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys/"+id)
}

// ── Settings ──

// settingsCategories 后台「系统设置」允许的配置分组。
// 敏感键（password/secret/api_key/dsn/token 等）由 settings.Store 用 APP_SECRET
// 派生密钥加密落库；非敏感配置明文存储。上级配置经 DB 持久化，env 作为默认值。
var settingsCategories = []string{
	"rate_limit", "degradation", "cache", "api_key",
	"agent", "llm", "storage", "payment", "redis", "postgres", "cors", "s3",
	"python",
}

func validSettingsCategory(c string) bool {
	for _, v := range settingsCategories {
		if v == c {
			return true
		}
	}
	return false
}

var settingsCategoryList = strings.Join(settingsCategories, ", ")

// intFromValue 从 JSON 解码出的值安全取整数，非数值/越界时返回 fallback。
func intFromValue(v interface{}, fallback int) int {
	switch n := v.(type) {
	case float64:
		i := int(n)
		if i > 0 {
			return i
		}
	case int:
		if n > 0 {
			return n
		}
	case string:
		var i int
		if _, err := fmt.Sscanf(n, "%d", &i); err == nil && i > 0 {
			return i
		}
	}
	return fallback
}

// SaveSettings PUT /v1/admin/settings
// 将某分组配置按 key 逐条 upsert 到 system_settings 表。
// config 中 value 为 null 的 key 会被删除，使该配置回落到 env 默认值。
func (h *AdminHandler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Category string                 `json:"category"`
		Config   map[string]interface{} `json:"config"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.Category == "" {
		BadRequest(w, "category is required: "+settingsCategoryList)
		return
	}
	if body.Config == nil {
		BadRequest(w, "config is required")
		return
	}
	if !validSettingsCategory(body.Category) {
		BadRequest(w, "invalid category: must be one of "+settingsCategoryList)
		return
	}

	ctx := r.Context()
	s := h.ensureSettingsStore()
	if s == nil {
		InternalError(w, "settings store unavailable")
		return
	}

	// 当前操作用户（可空，用于审计 updated_by）
	userID := ""
	if claims := auth.GetClaims(r.Context()); claims != nil {
		userID = claims.ID
	}

	if err := s.SaveConfig(ctx, body.Category, body.Config, userID); err != nil {
		slog.Error("save settings failed", "category", body.Category, "error", err)
		if err == settings.ErrEncryptedKeyNotFound {
			InternalError(w, "APP_SECRET not configured; cannot encrypt sensitive settings")
			return
		}
		InternalError(w, "failed to save settings")
		return
	}

	// rate_limit 分组：热更新分布式限流阈值（保存成功后生效）
	if body.Category == "rate_limit" && h.rateLimiter != nil {
		global := intFromValue(body.Config["global"], 0)
		tenant := intFromValue(body.Config["tenant"], 0)
		user := intFromValue(body.Config["user"], 0)
		if global > 0 || tenant > 0 || user > 0 {
			h.rateLimiter.Configure(global, tenant, user)
			slog.Info("rate limiter hot-reloaded", "global", global, "tenant", tenant, "user", user)
		}
	}

	// redirect：保存 redis 配置后热换 Redis 连接
	if body.Category == "redis" {
		h.hotReloadRedis(body.Config)
	}

	slog.Info("settings saved", "category", body.Category, "keys", len(body.Config))
	OK(w, map[string]interface{}{"status": "saved", "category": body.Category})
}

// ensureSettingsStore 惰性初始化 DB 加密设置存储。
func (h *AdminHandler) ensureSettingsStore() *settings.Store {
	if h.settingsStore == nil && db.Pool != nil {
		h.settingsStore = settings.New(db.Pool, h.appSecret)
	}
	return h.settingsStore
}

// hotReloadRedis 在保存 redis 分组设置后热换 Redis 连接（AtomicRedis.Swap）。
// 仅当配置了新地址才执行；失败仅记日志不影响保存结果。
func (h *AdminHandler) hotReloadRedis(cfg map[string]interface{}) {
	if h.redis == nil {
		return
	}
	addr := strFromValue(cfg["addr"])
	if addr == "" {
		return
	}
	rc := db.RedisConfig{
		Mode:     "single",
		Addr:     addr,
		Password: strFromValue(cfg["password"]),
		DB:       intFromValue(cfg["db"], 0),
		PoolSize: intFromValue(cfg["pool_size"], 50),
	}
	client, err := db.NewRedisClient(rc)
	if err != nil {
		slog.Error("redis hot-swap failed", "addr", addr, "error", err)
		return
	}
	h.redis.Swap(client)
	slog.Info("redis hot-swapped", "addr", addr)
}

// strFromValue 从 JSON 解码出的值安全取字符串；非字符串返回 ""。
func strFromValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// GetSettings GET /v1/admin/settings?category=... 读取某分组已持久化配置。
// 无记录时返回空 config（前端保留默认值）。
func (h *AdminHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	if category == "" {
		BadRequest(w, "category is required: "+settingsCategoryList)
		return
	}
	if !validSettingsCategory(category) {
		BadRequest(w, "invalid category: must be one of "+settingsCategoryList)
		return
	}
	s := h.ensureSettingsStore()
	if s == nil {
		OK(w, map[string]interface{}{"category": category, "config": map[string]interface{}{}})
		return
	}
	config, err := s.LoadConfig(r.Context(), category)
	if err != nil {
		OK(w, map[string]interface{}{"category": category, "config": map[string]interface{}{}})
		return
	}
	OK(w, map[string]interface{}{"category": category, "config": config})
}
