package db

import (
	"context"
	"os"
	"testing"
	"time"
)

// Redis 绗簩闃舵瀹炴祴锛氱湡瀹?Redis 闆嗘垚娴嬭瘯銆?// 鏈湴/CI 鏃?Redis 鏃惰嚜鍔ㄨ烦杩囷紙璁剧疆 REDIS_TEST_ADDR 鍚敤锛屽 redis://localhost:6379锛夈€?
func testRedisAddr(t *testing.T) string {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("REDIS_TEST_ADDR not set; skipping real-redis integration tests")
	}
	return addr
}

func TestRealRedis_Client(t *testing.T) {
	addr := testRedisAddr(t)
	cfg := RedisConfig{Mode: "single", Addr: addr, PoolSize: 5}
	client, err := NewRedisClient(cfg)
	if err != nil {
		t.Fatalf("connect redis: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping: %v", err)
	}

	// Set/Get round-trip
	key := "chiron:test:" + time.Now().Format("150405.000")
	if err := client.Set(ctx, key, "v1", 30*time.Second).Err(); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := client.Get(ctx, key).Result()
	if err != nil || got != "v1" {
		t.Fatalf("get: got=%q err=%v", got, err)
	}
	if n, err := client.Del(ctx, key).Result(); err != nil || n != 1 {
		t.Fatalf("del: n=%d err=%v", n, err)
	}

	// Do() 浠绘剰鍛戒护锛堟參鏃ュ織/绠＄悊鎿嶄綔锛?	if cmd := client.Do(ctx, "ECHO", "mini"); cmd.Err() != nil || cmd.Val() != "mini" {
		t.Fatalf("Do ECHO failed: %v", cmd.Err())
	}
}

func TestRealRedis_AtomicLua(t *testing.T) {
	addr := testRedisAddr(t)
	cfg := RedisConfig{Mode: "single", Addr: addr, PoolSize: 5}
	client, err := NewRedisClient(cfg)
	if err != nil {
		t.Fatalf("connect redis: %v", err)
	}
	defer client.Close()
	atomic := NewAtomicRedis(client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 鍘熷瓙閫掑锛堥檺娴?璁℃暟鍏辩敤鑴氭湰璺緞锛?	key := "chiron:test:counter:" + time.Now().Format("150405.000")
	for i := 0; i < 5; i++ {
		if err := atomic.Incr(ctx, key).Err(); err != nil {
			t.Fatalf("incr %d: %v", i, err)
		}
	}
	if v, err := atomic.Get(ctx, key).Int(); err != nil || v != 5 {
		t.Fatalf("counter: v=%d err=%v", v, err)
	}
	_ = atomic.Del(ctx, key)
}
