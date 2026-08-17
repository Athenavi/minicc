package models

import (
	"time"
)

// RedisConfig represents Redis connection and pool configuration.
type RedisConfig struct {
	ID   string `json:"id"`

	// 连接信息
	Host         string `json:"host"`
	Port         int    `json:"port"`
	DBIndex      int    `json:"db_index"`
	PoolSize     int    `json:"pool_size"`
	MinIdleConns int    `json:"min_idle_connections"`
	MaxConnAge   string `json:"max_conn_age"` // interval string like "300s"

	// 运行时状态
	Status          string  `json:"status"` // active/inactive
	LastHealthCheck *time.Time `json:"last_health_check"`
	AvgLatencyMS   float64 `json:"avg_latency_ms"`

	// 统计信息
	MemoryUsedMB    float64 `json:"memory_used_mb"`
	ConnectedClients int    `json:"connected_clients"`
	Hits            int64   `json:"hits"`
	Misses          int64   `json:"misses"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RedisStatus represents real-time Redis status.
type RedisStatus struct {
	Host             string  `json:"host"`
	Port             int     `json:"port"`
	Status           string  `json:"status"`
	Version          string  `json:"version"`
	UptimeDays       int     `json:"uptime_days"`
	MemoryUsedMB     float64 `json:"memory_used_mb"`
	MemoryPeakMB     float64 `json:"memory_peak_mb"`
	ConnectedClients int     `json:"connected_clients"`
	KeyspaceHits     int64   `json:"keyspace_hits"`
	KeyspaceMisses   int64   `json:"keyspace_misses"`
	HitRate          float64 `json:"hit_rate"` // percentage
	TotalCommands    int64   `json:"total_commands_processed"`
	SlowLogCount     int     `json:"slow_log_count"`
	PoolStats        PoolStats `json:"pool_stats"`
}

// PoolStats represents connection pool statistics.
type PoolStats struct {
	TotalGets       int `json:"total_gets"`
	PoolSize        int `json:"pool_size"`
	IdleConnections int `json:"idle_connections"`
	ActiveConnections int `json:"active_connections"`
}

// RedisSlowLog represents a slow query log entry.
type RedisSlowLog struct {
	ID        int       `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	DurationMs int64    `json:"duration_ms"`
	Command   string    `json:"command"`
	ClientAddr string   `json:"client_addr"`
}
