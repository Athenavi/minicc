package db

import (
	"context"
	"log/slog"
)

var Redis RedisClient

func ConnectRedis(ctx context.Context, addr, password string, db int) error {
	client, err := NewSingleRedis(addr, password, db)
	if err != nil {
		return err
	}
	Redis = client
	return nil
}

func CloseRedis() {
	if Redis != nil {
		if err := Redis.Close(); err != nil {
			slog.Error("redis close error", "error", err)
		}
		Redis = nil
		slog.Info("redis disconnected")
	}
}
