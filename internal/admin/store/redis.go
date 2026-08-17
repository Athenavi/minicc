package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"minicc/internal/admin/models"

	"github.com/google/uuid"
)

// RedisStore defines the interface for Redis configuration and monitoring operations.
type RedisStore interface {
	Create(ctx context.Context, config *models.RedisConfig) error
	GetByID(ctx context.Context, id string) (*models.RedisConfig, error)
	List(ctx context.Context) ([]models.RedisConfig, error)
	Update(ctx context.Context, id string, updates models.UpdateRedisRequest) error
	Delete(ctx context.Context, id string) error
	HealthCheck(ctx context.Context, id string) (*models.RedisStatus, error)
	GetStats(ctx context.Context, id string) (*models.RedisStatus, error)
	UpdateStats(ctx context.Context, id string, stats *models.RedisStatsUpdate) error
}

// PostgreSQLRedisStore implements RedisStore using PostgreSQL.
type PostgreSQLRedisStore struct {
	db *sql.DB
}

// NewPostgreSQLRedisStore creates a new PostgreSQL Redis store.
func NewPostgreSQLRedisStore(db *sql.DB) *PostgreSQLRedisStore {
	return &PostgreSQLRedisStore{db: db}
}

// Create inserts a new Redis configuration into the database.
func (s *PostgreSQLRedisStore) Create(ctx context.Context, config *models.RedisConfig) error {
	now := time.Now()
	config.ID = uuid.New().String()
	config.CreatedAt = now
	config.UpdatedAt = now

	if config.Status == "" {
		config.Status = "active"
	}
	if config.Port == 0 {
		config.Port = 6379
	}
	if config.PoolSize == 0 {
		config.PoolSize = 10
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_redis_configs (
			id, name, host, port, db_index, password_hash,
			pool_size, min_idle_conns, max_conn_age,
			status, last_health_check, avg_latency_ms,
			memory_used_mb, connected_clients, hits, misses,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`,
		config.ID,
		config.Name,
		config.Host,
		config.Port,
		config.DBIndex,
		config.PasswordHash,
		config.PoolSize,
		config.MinIdleConns,
		config.MaxConnAge,
		config.Status,
		config.LastHealthCheck,
		config.AvgLatencyMS,
		config.MemoryUsedMB,
		config.ConnectedClients,
		config.Hits,
		config.Misses,
		now,
		now,
	)

	if err != nil {
		return fmt.Errorf("create redis config: %w", err)
	}

	_, err = result.RowsAffected()
	return err
}

// GetByID retrieves a Redis configuration by its ID.
func (s *PostgreSQLRedisStore) GetByID(ctx context.Context, id string) (*models.RedisConfig, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, host, port, db_index, password_hash,
		       pool_size, min_idle_conns, max_conn_age,
		       status, last_health_check, avg_latency_ms,
		       memory_used_mb, connected_clients, hits, misses,
		       created_at, updated_at
		FROM admin_redis_configs
		WHERE id = $1
	`, id)

	return scanRedisConfig(row)
}

// List retrieves all Redis configurations.
func (s *PostgreSQLRedisStore) List(ctx context.Context) ([]models.RedisConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, host, port, db_index, password_hash,
		       pool_size, min_idle_conns, max_conn_age,
		       status, last_health_check, avg_latency_ms,
		       memory_used_mb, connected_clients, hits, misses,
		       created_at, updated_at
		FROM admin_redis_configs
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list redis configs: %w", err)
	}
	defer rows.Close()

	var configs []models.RedisConfig
	for rows.Next() {
		config, err := scanRedisConfig(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, *config)
	}

	return configs, nil
}

// Update modifies an existing Redis configuration.
func (s *PostgreSQLRedisStore) Update(ctx context.Context, id string, updates models.UpdateRedisRequest) error {
	var setClauses []string
	var args []interface{}
	argIdx := 1

	if updates.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *updates.Name)
		argIdx++
	}

	if updates.Host != nil {
		setClauses = append(setClauses, fmt.Sprintf("host = $%d", argIdx))
		args = append(args, *updates.Host)
		argIdx++
	}

	if updates.Port != nil {
		setClauses = append(setClauses, fmt.Sprintf("port = $%d", argIdx))
		args = append(args, *updates.Port)
		argIdx++
	}

	if updates.PoolSize != nil {
		setClauses = append(setClauses, fmt.Sprintf("pool_size = $%d", argIdx))
		args = append(args, *updates.PoolSize)
		argIdx++
	}

	if updates.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *updates.Status)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now())
	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE admin_redis_configs
		SET %s
		WHERE id = $%d
	`, join(",", setClauses), argIdx+1)

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update redis config: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// Delete removes a Redis configuration.
func (s *PostgreSQLRedisStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM admin_redis_configs WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete redis config: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// HealthCheck performs a health check and updates the status.
func (s *PostgreSQLRedisStore) HealthCheck(ctx context.Context, id string) (*models.RedisStatus, error) {
	config, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// TODO: Actually connect to Redis and perform health check
	// For now, just update the timestamp
	now := time.Now()
	_, err = s.db.ExecContext(ctx, `
		UPDATE admin_redis_configs 
		SET last_health_check = $1, updated_at = $1
		WHERE id = $2
	`, now, id)
	if err != nil {
		return nil, fmt.Errorf("health check update: %w", err)
	}

	return &models.RedisStatus{
		ConfigID:    id,
		Status:      "healthy",
		Uptime:      "unknown",
		Version:     "unknown",
		MemoryUsage: config.MemoryUsedMB,
		Clients:     config.ConnectedClients,
		SlowLogCount: 0,
	}, nil
}

// GetStats retrieves real-time statistics from Redis.
func (s *PostgreSQLRedisStore) GetStats(ctx context.Context, id string) (*models.RedisStatus, error) {
	config, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// TODO: Actually query Redis INFO command
	// For now, return stored values
	hitsMisses := config.Hits + config.Misses
	hitRate := 0.0
	if hitsMisses > 0 {
		hitRate = float64(config.Hits) / float64(hitsMisses) * 100
	}

	return &models.RedisStatus{
		ConfigID:     id,
		Name:         config.Name,
		Host:         config.Host,
		Port:         config.Port,
		Status:       config.Status,
		AvgLatencyMS: config.AvgLatencyMS,
		MemoryUsage:  config.MemoryUsedMB,
		Clients:      config.ConnectedClients,
		HitRate:      hitRate,
		Hits:         config.Hits,
		Misses:       config.Misses,
		PoolStats: models.RedisPoolStats{
			PoolSize:    config.PoolSize,
			IdleConns:   config.MinIdleConns,
			WaitCount:   0,
			WaitDuration: "",
		},
		SlowLogCount: 0,
	}, nil
}

// UpdateStats updates the statistics in the database.
func (s *PostgreSQLRedisStore) UpdateStats(ctx context.Context, id string, stats *models.RedisStatsUpdate) error {
	setClauses := []string{"updated_at = $1"}
	var args []interface{}
	argIdx := 2

	args = append(args, time.Now())

	if stats.MemoryUsedMB != nil {
		setClauses = append(setClauses, fmt.Sprintf("memory_used_mb = $%d", argIdx))
		args = append(args, *stats.MemoryUsedMB)
		argIdx++
	}

	if stats.ConnectedClients != nil {
		setClauses = append(setClauses, fmt.Sprintf("connected_clients = $%d", argIdx))
		args = append(args, *stats.ConnectedClients)
		argIdx++
	}

	if stats.Hits != nil {
		setClauses = append(setClauses, fmt.Sprintf("hits = hits + $%d", argIdx))
		args = append(args, *stats.Hits)
		argIdx++
	}

	if stats.Misses != nil {
		setClauses = append(setClauses, fmt.Sprintf("misses = misses + $%d", argIdx))
		args = append(args, *stats.Misses)
		argIdx++
	}

	if stats.AvgLatencyMS != nil {
		setClauses = append(setClauses, fmt.Sprintf("avg_latency_ms = $%d", argIdx))
		args = append(args, *stats.AvgLatencyMS)
		argIdx++
	}

	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE admin_redis_configs
		SET %s
		WHERE id = $%d
	`, join(",", setClauses), argIdx)

	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update redis stats: %w", err)
	}

	return nil
}

// scanRedisConfig scans a single row into a RedisConfig.
func scanRedisConfig(row scanner) (*models.RedisConfig, error) {
	var config models.RedisConfig
	var port int
	var dbIndex int
	var poolSize int
	var minIdleConns int
	var memoryUsedMB sql.NullFloat64
	var connectedClients sql.NullInt64
	var hits sql.NullInt64
	var misses sql.NullInt64
	var avgLatencyMS sql.NullFloat64
	var lastHealthCheck sql.NullTime

	err := row.Scan(
		&config.ID, &config.Name, &config.Host, &port, &dbIndex,
		&config.PasswordHash, &poolSize, &minIdleConns, &config.MaxConnAge,
		&config.Status, &lastHealthCheck, &avgLatencyMS,
		&memoryUsedMB, &connectedClients, &hits, &misses,
		&config.CreatedAt, &config.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("scan redis config: %w", err)
	}

	config.Port = port
	config.DBIndex = dbIndex
	config.PoolSize = poolSize
	config.MinIdleConns = minIdleConns
	config.LastHealthCheck = lastHealthCheck.Time
	if avgLatencyMS.Valid {
		config.AvgLatencyMS = avgLatencyMS.Float64
	}
	if memoryUsedMB.Valid {
		config.MemoryUsedMB = memoryUsedMB.Float64
	}
	if connectedClients.Valid {
		config.ConnectedClients = int(connectedClients.Int64)
	}
	if hits.Valid {
		config.Hits = hits.Int64
	}
	if misses.Valid {
		config.Misses = misses.Int64
	}

	return &config, nil
}

// Scanner is the interface used by Scan functions.
type scanner interface {
	Scan(dest ...interface{}) error
