package db

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig holds connection pool tuning parameters.
type PoolConfig struct {
	MaxConns          int
	MinConns          int
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

// DefaultPoolConfig returns sensible defaults matching ConnectPostgres behavior.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConns:          20,
		MinConns:          2,
		MaxConnLifetime:   30 * time.Minute,
		MaxConnIdleTime:   5 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
	}
}

// DatabaseRouter routes database operations to primary (write) or replica (read) pools.
type DatabaseRouter struct {
	writePool *pgxpool.Pool   // Primary database (read-write)
	readPools []*pgxpool.Pool // Replica databases (read-only)
	next      atomic.Uint64   // Round-robin counter for load balancing
}

// Router is the global database router, set when read replicas are configured.
var Router *DatabaseRouter

// newPool creates a pgxpool with the given DSN and pool config.
func newPool(ctx context.Context, dsn string, cfg PoolConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = int32(cfg.MinConns)
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.HealthCheckPeriod = cfg.HealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	return pool, nil
}

// NewDatabaseRouter creates a new database router with pool configuration.
// writeDSN is the primary database connection string.
// readDSNs is a list of replica database connection strings (can be empty).
func NewDatabaseRouter(ctx context.Context, writeDSN string, readDSNs []string, cfg PoolConfig) (*DatabaseRouter, error) {
	// Create write pool
	writePool, err := newPool(ctx, writeDSN, cfg)
	if err != nil {
		return nil, fmt.Errorf("write pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := writePool.Ping(pingCtx); err != nil {
		writePool.Close()
		return nil, fmt.Errorf("ping write pool: %w", err)
	}

	// Create read pools
	readPools := make([]*pgxpool.Pool, 0, len(readDSNs))
	for _, dsn := range readDSNs {
		if dsn == "" {
			continue
		}

		pool, err := newPool(ctx, dsn, cfg)
		if err != nil {
			slog.Warn("failed to create read pool", "dsn", dsn, "error", err)
			continue
		}

		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := pool.Ping(pingCtx); err != nil {
			slog.Warn("failed to ping read pool", "dsn", dsn, "error", err)
			pool.Close()
			cancel()
			continue
		}
		cancel()

		readPools = append(readPools, pool)
	}

	slog.Info("database router initialized",
		"max_conns", cfg.MaxConns,
		"min_conns", cfg.MinConns,
		"read_pools", len(readPools),
	)

	return &DatabaseRouter{
		writePool: writePool,
		readPools: readPools,
	}, nil
}

// Write returns the primary (write) pool.
func (r *DatabaseRouter) Write() *pgxpool.Pool {
	return r.writePool
}

// Read returns a replica (read) pool using round-robin load balancing.
// Falls back to primary if no replicas are available.
func (r *DatabaseRouter) Read() *pgxpool.Pool {
	if len(r.readPools) == 0 {
		return r.writePool
	}

	idx := r.next.Add(1) % uint64(len(r.readPools))
	return r.readPools[idx]
}

// ReadPreferred returns a healthy replica pool.
// Falls back to primary if no healthy replicas are available.
func (r *DatabaseRouter) ReadPreferred() *pgxpool.Pool {
	if len(r.readPools) == 0 {
		return r.writePool
	}

	// Try each pool in round-robin order
	startIdx := r.next.Add(1)
	for i := 0; i < len(r.readPools); i++ {
		idx := (startIdx + uint64(i)) % uint64(len(r.readPools))
		pool := r.readPools[idx]

		// Quick health check
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		if err := pool.Ping(ctx); err == nil {
			cancel()
			return pool
		}
		cancel()
	}

	// All replicas unhealthy, fall back to primary
	slog.Warn("all read pools unhealthy, falling back to write pool")
	return r.writePool
}

// Close closes all database pools.
func (r *DatabaseRouter) Close() {
	if r.writePool != nil {
		r.writePool.Close()
	}
	for _, pool := range r.readPools {
		if pool != nil {
			pool.Close()
		}
	}
}

// Stats returns statistics for all pools.
func (r *DatabaseRouter) Stats() map[string]interface{} {
	stats := map[string]interface{}{
		"write_pool": r.writePool.Stat(),
		"read_pools": len(r.readPools),
	}

	for i, pool := range r.readPools {
		stats[fmt.Sprintf("read_pool_%d", i)] = pool.Stat()
	}

	return stats
}

// IsHealthy checks if the database router is healthy.
func (r *DatabaseRouter) IsHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := r.writePool.Ping(ctx); err != nil {
		return false
	}

	if len(r.readPools) > 0 {
		for _, pool := range r.readPools {
			if err := pool.Ping(ctx); err == nil {
				return true
			}
		}
		return false
	}

	return true
}
