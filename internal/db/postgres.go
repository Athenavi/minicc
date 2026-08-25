package db

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athenavi/chiron/internal/monitor"
)

// PoolMu guards Pool for concurrent access (e.g. hot-reload reconnection).
var PoolMu sync.RWMutex

// Pool is the global PostgreSQL connection pool.
// Protected by PoolMu for concurrent read/write access.
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
	// P 鎬ц兘/绋冲畾锛歴tatement_timeout 闃叉參鏌ヨ闀挎湡鍗犵敤杩炴帴鑰楀敖姹?	// 闀挎煡璇紙杩佺Щ/鎵归噺锛夊簲璧扮嫭绔嬭繛鎺ワ紝涓嶅鐢ㄤ笟鍔℃睜
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

	PoolMu.Lock()
	Pool = pool
	PoolMu.Unlock()
	slog.Info("postgres connected", "max_conns", maxConn, "min_conns", minConn)

	// 娉ㄥ唽杩炴帴姹犵洃鎺у埌鍏ㄥ眬 metrics
	monitor.RegisterExtraStats(PoolStats)
	return nil
}

func ClosePostgres() {
	PoolMu.Lock()
	p := Pool
	Pool = nil
	PoolMu.Unlock()
	if p != nil {
		p.Close()
		slog.Info("postgres disconnected")
	}
}

// ReadPool returns the best available pool for read operations.
// If a DatabaseRouter with read replicas is configured, returns a healthy replica.
// Otherwise falls back to the primary Pool.
func ReadPool() *pgxpool.Pool {
	PoolMu.RLock()
	p := Pool
	PoolMu.RUnlock()
	if Router != nil {
		return Router.ReadPreferred()
	}
	return p
}

// PoolStats returns current PostgreSQL connection pool statistics for monitoring.
func PoolStats() map[string]interface{} {
	PoolMu.RLock()
	p := Pool
	PoolMu.RUnlock()
	if p == nil {
		return nil
	}
	s := p.Stat()
	return map[string]interface{}{
		"total_conns":     s.TotalConns(),
		"idle_conns":      s.IdleConns(),
		"acquired_conns":  s.AcquiredConns(),
		"empty_acquire":   s.EmptyAcquireCount(),
		"acquire_duration_ms": s.AcquireDuration().Milliseconds(),
	}
}
