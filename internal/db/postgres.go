package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
