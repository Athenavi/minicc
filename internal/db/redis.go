package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

var Redis *redis.Client

func ConnectRedis(ctx context.Context, addr, password string, db int) error {
	opts := &redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     50,
		MinIdleConns: 5,
		MaxRetries:   1,
		PoolTimeout:  4 * time.Second,
	}

	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}

	Redis = client
	slog.Info("redis connected", "addr", addr, "db", db)
	return nil
}

func CloseRedis() {
	if Redis != nil {
		Redis.Close()
		slog.Info("redis disconnected")
	}
}
