package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athenavi/minicc/internal/monitor"
)

var Pool *pgxpool.Pool

func ConnectPostgres(ctx context.Context, dsn string, maxConn, minConn int) error {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("pgx parse config: %w", err)
	}

	cfg.MaxConns = int32(maxConn)
	cfg.MinConns = int32(minConn)
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	// P 性能/稳定：statement_timeout 防慢查询长期占用连接耗尽池
	// 长查询（迁移/批量）应走独立连接，不复用业务池
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET statement_timeout = 30000")
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("pgx new pool: %w", err)
	}

	// Verify connection
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return fmt.Errorf("pgx ping: %w", err)
	}

	Pool = pool
	slog.Info("postgres connected", "max_conns", maxConn, "min_conns", minConn)

	// 注册连接池监控到全局 metrics
	monitor.RegisterExtraStats(PoolStats)
	return nil
}

func ClosePostgres() {
	if Pool != nil {
		Pool.Close()
		Pool = nil
		slog.Info("postgres disconnected")
	}
}

// ReadPool returns the best available pool for read operations.
// If a DatabaseRouter with read replicas is configured, returns a healthy replica.
// Otherwise falls back to the primary Pool.
func ReadPool() *pgxpool.Pool {
	if Router != nil {
		return Router.ReadPreferred()
	}
	return Pool
}

// PoolStats returns current PostgreSQL connection pool statistics for monitoring.
func PoolStats() map[string]interface{} {
	if Pool == nil {
		return nil
	}
	s := Pool.Stat()
	return map[string]interface{}{
		"total_conns":     s.TotalConns(),
		"idle_conns":      s.IdleConns(),
		"acquired_conns":  s.AcquiredConns(),
		"empty_acquire":   s.EmptyAcquireCount(),
		"acquire_duration_ms": s.AcquireDuration().Milliseconds(),
	}
}
