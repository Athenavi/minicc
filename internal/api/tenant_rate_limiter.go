package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
)

// tenantBucketLua 鍘熷瓙鍦板畬鎴?token bucket 鍙栦护鐗岋細
// KEYS[1] = bucket key
// ARGV[1] = capacity (burst)
// ARGV[2] = refill per second (tokens/sec, 脳1000 鈫?ms 绮惧害鐢辨湇鍔＄璁＄畻绠€鍖栦负绉?
// ARGV[3] = now (unix seconds, float)
// ARGV[4] = requested tokens (1.0)
// 杩斿洖: 1 鍏佽锛? 鎷掔粷
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

// TenantRateLimiter 鎻愪緵 per-tenant 璧勬簮绾?QPS 闄愬埗銆?// Redis 鍙敤鏃惰蛋 Lua 鍘熷瓙鑴氭湰锛堝瀹炰緥鐘舵€佷竴鑷达級锛?// Redis 涓嶅彲鐢ㄦ椂 fail-close锛堜笌 DistributedRateLimiter 绛栫暐涓€鑷达級銆?type TenantRateLimiter struct {
	rdb       db.RedisClient
	maxBurst  int
	refillPerSec float64

	// 鏈湴闄嶇骇缂撳瓨锛氫粎鍦?Redis fail-close 鏃剁敤浜庤閬跨┖鎸囬拡锛涗笉浣滀负闄愭祦鐪熷疄鐘舵€?	mu     sync.Mutex
	tokens map[string]*tokenBucketLocal
}

type tokenBucketLocal struct {
	tokens     float64
	lastRefill time.Time
}

// NewTenantRateLimiter 鍒涘缓鍩轰簬 Redis 鐨?token bucket 闄愭祦鍣ㄣ€?func NewTenantRateLimiter(rdb db.RedisClient, maxQPS, burst int) *TenantRateLimiter {
	return &TenantRateLimiter{
		rdb:          rdb,
		maxBurst:     burst,
		refillPerSec: float64(maxQPS),
		tokens:       make(map[string]*tokenBucketLocal),
	}
}

// Allow 妫€鏌ヨ姹傛槸鍚﹁鍏佽銆傝繑鍥?(allowed, retryAfterSeconds)銆?// fail-close锛歊edis 涓嶅彲鐢ㄦ垨鍑洪敊鏃舵嫆缁濊姹傘€?func (rl *TenantRateLimiter) Allow(ctx context.Context, resource, tenantID string) (bool, float64) {
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

// Middleware 杩斿洖 HTTP 涓棿浠讹紝鎸?tenant_id + 璧勬簮绫诲瀷闄愭祦銆?func (rl *TenantRateLimiter) Middleware(next http.Handler) http.Handler {
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

// extractResource 浠?URL 璺緞鎻愬彇璧勬簮鍚嶃€?func extractResource(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "unknown"
	}
	parts := strings.SplitN(trimmed, "/", 4)
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

func formatFloat(f float64) string {
	if f < 1 {
		return "1"
	}
	return strconv.Itoa(int(f))
}
