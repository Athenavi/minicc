package db

import (
	"context"
	"log/slog"
	"sync"
)

var (
	redisMu sync.RWMutex
	Redis   RedisClient
)

func ConnectRedis(ctx context.Context, addr, password string, db int, poolSize int) error {
	client, err := NewSingleRedis(addr, password, db, poolSize)
	if err != nil {
		return err
	}
	redisMu.Lock()
	Redis = client
	redisMu.Unlock()
	return nil
}

func GetRedis() RedisClient {
	redisMu.RLock()
	defer redisMu.RUnlock()
	return Redis
}

func CloseRedis() {
	redisMu.Lock()
	defer redisMu.Unlock()
	if Redis != nil {
		if err := Redis.Close(); err != nil {
			slog.Error("redis close error", "error", err)
		}
		Redis = nil
		slog.Info("redis disconnected")
	}
}
