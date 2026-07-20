package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
)

// DistributedRateLimiter 基于 Redis 的分布式限流器
// 支持多级限流：全局、租户、用户
type DistributedRateLimiter struct {
	rdb db.RedisClient

	// 限流配置
	globalLimit  int // 全局每分钟请求数
	tenantLimit  int // 每租户每分钟请求数
	userLimit    int // 每用户每分钟请求数

	// 本地缓存（减少 Redis 查询）
	localCache sync.Map
}

// NewDistributedRateLimiter 创建分布式限流器
func NewDistributedRateLimiter(rdb db.RedisClient, globalLimit, tenantLimit, userLimit int) *DistributedRateLimiter {
	if globalLimit <= 0 {
		globalLimit = 1000
	}
	if tenantLimit <= 0 {
		tenantLimit = 100
	}
	if userLimit <= 0 {
		userLimit = 30
	}

	return &DistributedRateLimiter{
		rdb:         rdb,
		globalLimit: globalLimit,
		tenantLimit: tenantLimit,
		userLimit:   userLimit,
	}
}

// rateLimitLua Redis Lua 脚本实现原子限流
const rateLimitLua = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local current = tonumber(redis.call('GET', key) or "0")
if current >= limit then
    return 0
else
    redis.call('INCR', key)
    redis.call('EXPIRE', key, window)
    return 1
end
`

// Allow 检查是否允许请求
func (l *DistributedRateLimiter) Allow(ctx context.Context, tenantID, userID string) (bool, error) {
	if l.rdb == nil {
		return true, nil // Redis 不可用时放行
	}

	// 1. 全局限流
	globalKey := "ratelimit:global:minute"
	result, err := l.rdb.Eval(ctx, rateLimitLua, []string{globalKey}, l.globalLimit, 60).Int()
	if err != nil {
		slog.Warn("全局限流检查失败", "error", err)
		return true, nil // Redis 错误时放行
	}
	if result == 0 {
		return false, fmt.Errorf("全局请求频率超限")
	}

	// 2. 租户限流
	if tenantID != "" {
		tenantKey := fmt.Sprintf("ratelimit:tenant:%s:minute", tenantID)
		result, err = l.rdb.Eval(ctx, rateLimitLua, []string{tenantKey}, l.tenantLimit, 60).Int()
		if err != nil {
			slog.Warn("租户限流检查失败", "tenant", tenantID, "error", err)
			return true, nil
		}
		if result == 0 {
			return false, fmt.Errorf("租户 %s 请求频率超限", tenantID)
		}
	}

	// 3. 用户限流
	if userID != "" {
		userKey := fmt.Sprintf("ratelimit:user:%s:minute", userID)
		result, err = l.rdb.Eval(ctx, rateLimitLua, []string{userKey}, l.userLimit, 60).Int()
		if err != nil {
			slog.Warn("用户限流检查失败", "user", userID, "error", err)
			return true, nil
		}
		if result == 0 {
			return false, fmt.Errorf("用户 %s 请求频率超限", userID)
		}
	}

	return true, nil
}

// DistributedRateLimitMiddleware 分布式限流中间件
func DistributedRateLimitMiddleware(limiter *DistributedRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 提取用户 ID
			var userID string
			claims := auth.GetClaims(r.Context())
			if claims != nil {
				userID = claims.UserID
			}

			allowed, err := limiter.Allow(r.Context(), "", userID)
			if err != nil {
				slog.Warn("限流触发",
					"error", err,
					"user", userID,
					"path", r.URL.Path,
				)
				w.Header().Set("Retry-After", "60")
				TooManyRequests(w)
				return
			}

			if !allowed {
				w.Header().Set("Retry-After", "60")
				TooManyRequests(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Cleanup 清理过期的本地缓存
func (l *DistributedRateLimiter) Cleanup(interval time.Duration) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("rate limiter cleanup panic", "panic", r)
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			l.localCache.Range(func(key, value any) bool {
				if entry, ok := value.(*cacheEntry); ok {
					if time.Since(entry.ts) > time.Minute {
						l.localCache.Delete(key)
					}
				}
				return true
			})
		}
	}()
}

type cacheEntry struct {
	count int
	ts    time.Time
}
