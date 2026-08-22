package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
)

// tenantBucketLua 原子地完成 token bucket 取令牌：
// KEYS[1] = bucket key
// ARGV[1] = capacity (burst)
// ARGV[2] = refill per second (tokens/sec, ×1000 → ms 精度由服务端计算简化为秒)
// ARGV[3] = now (unix seconds, float)
// ARGV[4] = requested tokens (1.0)
// 返回: 1 允许；0 拒绝
const tenantBucketLua = `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local req = tonumber(ARGV[4])

local data = redis.call("HMGET", key, "tokens", "ts")
local tokens = tonumber(data[1])
local ts = tonumber(data[2])
if tokens == nil then
  tokens = capacity
  ts = now
end

local elapsed = math.max(0, now - ts)
tokens = math.min(capacity, tokens + elapsed * refill)

local allowed = 0
if tokens >= req then
  tokens = tokens - req
  allowed = 1
end

redis.call("HMSET", key, "tokens", tostring(tokens), "ts", tostring(now))
redis.call("EXPIRE", key, 300)
return tostring(allowed)
`

// TenantRateLimiter 提供 per-tenant 资源级 QPS 限制。
// Redis 可用时走 Lua 原子脚本（多实例状态一致）；
// Redis 不可用时 fail-close（与 DistributedRateLimiter 策略一致）。
type TenantRateLimiter struct {
	rdb       db.RedisClient
	maxBurst  int
	refillPerSec float64

	// 本地降级缓存：仅在 Redis fail-close 时用于规避空指针；不作为限流真实状态
	mu     sync.Mutex
	tokens map[string]*tokenBucketLocal
}

type tokenBucketLocal struct {
	tokens     float64
	lastRefill time.Time
}

// NewTenantRateLimiter 创建基于 Redis 的 token bucket 限流器。
func NewTenantRateLimiter(rdb db.RedisClient, maxQPS, burst int) *TenantRateLimiter {
	return &TenantRateLimiter{
		rdb:          rdb,
		maxBurst:     burst,
		refillPerSec: float64(maxQPS),
		tokens:       make(map[string]*tokenBucketLocal),
	}
}

// Allow 检查请求是否被允许。返回 (allowed, retryAfterSeconds)。
// fail-close：Redis 不可用或出错时拒绝请求。
func (rl *TenantRateLimiter) Allow(ctx context.Context, resource, tenantID string) (bool, float64) {
	if rl.rdb == nil {
		return false, 1 // fail-close
	}
	if tenantID == "" {
		return false, 1
	}

	key := fmt.Sprintf("tenantbucket:%s:%s", resource, tenantID)
	now := float64(time.Now().UnixNano()) / 1e9

	result, err := rl.rdb.Eval(ctx, tenantBucketLua,
		[]string{key},
		rl.maxBurst, rl.refillPerSec, now, 1.0).Text()
	if err != nil {
		slog.Error("tenant rate limit Redis eval failed (fail-close)",
			"error", err, "tenant", tenantID, "resource", resource)
		return false, 1
	}
	if result == "1" {
		return true, 0
	}
	return false, 1
}

// Middleware 返回 HTTP 中间件，按 tenant_id + 资源类型限流。
func (rl *TenantRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.GetClaims(r.Context())
		if claims == nil || claims.TenantID == "" {
			Unauthorized(w, ErrAuthRequired)
			return
		}
		tenantID := claims.TenantID
		resource := extractResource(r.URL.Path)

		allowed, retryAfter := rl.Allow(r.Context(), resource, tenantID)
		if !allowed {
			slog.Warn("tenant rate limit exceeded",
				"resource", resource, "tenant", tenantID, "retry_after", retryAfter)
			w.Header().Set("Retry-After", formatFloat(retryAfter))
			TooManyRequests(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractResource 从 URL 路径提取资源名。
func extractResource(path string) string {
	parts := splitPath(path)
	if len(parts) >= 2 {
		resource := parts[1]
		if len(parts) >= 3 {
			action := parts[2]
			if action == "query" || action == "build" || action == "test" {
				return resource + "_" + action
			}
		}
		return resource
	}
	return "unknown"
}

func splitPath(path string) []string {
	if path == "/" {
		return []string{""}
	}
	result := []string{}
	start := 0
	for i, c := range path {
		if c == '/' && i > 0 {
			result = append(result, path[start:i])
			start = i + 1
		}
	}
	if start < len(path) {
		result = append(result, path[start:])
	}
	return result
}

func formatFloat(f float64) string {
	if f < 1 {
		return "1"
	}
	return strconv.Itoa(int(f))
}
