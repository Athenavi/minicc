package models

import (
	"time"
)

// DBConfig represents PostgreSQL connection and pool configuration.
type DBConfig struct {
	ID   string `json:"id"`

	// 连接信息
	Host         string `json:"host"`
	Port         int    `json:"port"`
	DBName       string `json:"dbname"`
	MaxOpenConns int    `json:"max_open_connections"`
	MaxIdleConns int    `json:"max_idle_connections"`
	ConnMaxLifeTime string `json:"conn_max_lifetime"`

	// 运行时状态
	Status         string    `json:"status"` // active/inactive
	LastHealthCheck *time.Time `json:"last_health_check"`
	AvgQueryTimeMS float64   `json:"avg_query_time_ms"`

	// 统计信息
	DatabaseSizeMB  float64 `json:"database_size_mb"`
	TotalTables     int     `json:"total_tables"`
	SequentialScans int64   `json:"sequential_scans"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DBStatus represents real-time database status.
type DBStatus struct {
	Host              string    `json:"host"`
	Port              int       `json:"port"`
	DBName            string    `json:"dbname"`
	Status            string    `json:"status"`
	Version           string    `json:"version"`
	ActiveConnections int       `json:"active_connections"`
	IdleConnections   int       `json:"idle_connections"`
	TotalConnections  int       `json:"total_connections"`
	DatabaseSizeMB    float64   `json:"database_size_mb"`
	TotalTables       int       `json:"total_tables"`
	TableCount        int       `json:"table_count"`
	IndexCount        int       `json:"index_count"`
	SeqScans          int64     `json:"sequential_scans"`
	IdxScans          int64     `json:"index_scans"`
	RowsProcessed    int64     `json:"rows_processed"`
	AvgQueryTimeMS    float64   `json:"avg_query_time_ms"`
	LongestQueries    []QueryStat `json:"longest_queries"`
	PoolStats        DBPoolStats `json:"pool_stats"`
}

// QueryStat represents a long-running query statistic.
type QueryStat struct {
	Query      string `json:"query"`
	AvgTimeMS  float64 `json:"avg_time_ms"`
	Calls      int64   `json:"calls"`
	TotalTimeMS float64 `json:"total_time_ms"`
}

// DBPoolStats represents database connection pool statistics.
type DBPoolStats struct {
	MaxOpenConnections int `json:"max_open_connections"`
	OpenConnections    int `json:"open_connections"`
	InUse              int `json:"in_use"`
	Idle               int `json:"idle"`
	Waits              int `json:"waits"`
}

// DatabaseBackup represents a database backup record.
type DatabaseBackup struct {
	ID          string    `json:"id"`
	BackupType  string    `json:"backup_type"` // manual/scheduled
	Description string    `json:"description"`
	FilePath    string    `json:"file_path"`
	FileSizeMB  float64   `json:"file_size_mb"`
	Status      string    `json:"status"` // running/completed/failed/deleted
	ErrorMsg    string    `json:"error_msg,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	DurationSec int       `json:"duration_sec"`
	CreatedBy   string    `json:"created_by"`
}
