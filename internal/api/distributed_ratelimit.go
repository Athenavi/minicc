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

// rateLimitLua 三级限流原子脚本 — 预检查全部三级后统一递增，防止配额泄漏
//
// KEYS[1]  全局 key     KEYS[2] 租户 key（空串则跳过）  KEYS[3] 用户 key（空串则跳过）
// ARGV[1]  全局上限     ARGV[2] 租户上限               ARGV[3] 用户上限
// ARGV[4]  窗口秒数
// 返回 "ok" / "global" / "tenant" / "user"
const rateLimitLua = `
local function check(key, limit_str)
    if key == "" or limit_str == "" then return true end
    local cur = tonumber(redis.call("GET", key) or "0")
    local lim = tonumber(limit_str)
    return cur < lim
end
if not check(KEYS[1], ARGV[1]) then return "global" end
if not check(KEYS[2], ARGV[2]) then return "tenant" end
if not check(KEYS[3], ARGV[3]) then return "user" end
local w = tonumber(ARGV[4])
if KEYS[1] ~= "" then redis.call("INCR", KEYS[1]); redis.call("EXPIRE", KEYS[1], w) end
if KEYS[2] ~= "" then redis.call("INCR", KEYS[2]); redis.call("EXPIRE", KEYS[2], w) end
if KEYS[3] ~= "" then redis.call("INCR", KEYS[3]); redis.call("EXPIRE", KEYS[3], w) end
return "ok"
`

// Allow 检查是否允许请求 — 单次原子 eval 完成三级检查
func (l *DistributedRateLimiter) Allow(ctx context.Context, tenantID, userID string) (bool, error) {
	if l.rdb == nil {
		return true, nil // Redis 不可用时放行
	}

	globalKey := "ratelimit:global:minute"
	tenantKey := ""
	if tenantID != "" {
		tenantKey = fmt.Sprintf("ratelimit:tenant:%s:minute", tenantID)
	}
	userKey := ""
	if userID != "" {
		userKey = fmt.Sprintf("ratelimit:user:%s:minute", userID)
	}

	// 限流失效的参数（limit≤0）直接跳过
	tenantLim := l.tenantLimit
	if tenantKey == "" {
		tenantLim = 0
	}
	userLim := l.userLimit
	if userKey == "" {
		userLim = 0
	}

	result, err := l.rdb.Eval(ctx, rateLimitLua,
		[]string{globalKey, tenantKey, userKey},
		l.globalLimit, tenantLim, userLim, 60).Text()
	if err != nil {
		slog.Warn("限流检查失败", "error", err)
		return true, nil // Redis 错误时放行
	}

	switch result {
	case "global":
		return false, fmt.Errorf("全局请求频率超限")
	case "tenant":
		return false, fmt.Errorf("租户 %s 请求频率超限", tenantID)
	case "user":
		return false, fmt.Errorf("用户 %s 请求频率超限", userID)
	default:
		return true, nil
	}
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
