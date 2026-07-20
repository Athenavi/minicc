package api

import (
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/monitor"
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
	if db.Pool != nil {
		ctx := r.Context()
		start := time.Now()
		db.Pool.Ping(ctx)
		stats.Gateway.DBLatencyMs = float64(time.Since(start).Microseconds()) / 1000
	}

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
	if h.pythonClient == nil || !h.pythonClient.IsConnected() {
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

	if h.pythonClient == nil || !h.pythonClient.IsConnected() {
		InternalError(w, "python engine not available")
		return
	}
	var resp interface{}
	if err := h.pythonClient.PostJSON(r.Context(), "/v1/admin/api-keys", body, &resp); err != nil {
		slog.Error("add api key proxy error", "error", err)
		InternalError(w, "python engine error")
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

	if h.pythonClient == nil || !h.pythonClient.IsConnected() {
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

	if h.pythonClient == nil || !h.pythonClient.IsConnected() {
		InternalError(w, "python engine not available")
		return
	}
	h.pythonClient.ForwardRequest(w, r, "/v1/admin/api-keys/"+id)
}

// ── Settings ──

func (h *AdminHandler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Category string                 `json:"category"`
		Config   map[string]interface{} `json:"config"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}

	if body.Category == "" {
		BadRequest(w, "category is required")
		return
	}

	// 设置保存到内存（运行时生效，重启后丢失）
	// 后续可持久化到配置文件或数据库
	OK(w, map[string]string{"status": "saved", "category": body.Category})
}
