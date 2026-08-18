package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/redis/go-redis/v9"
)

// ErrTenantQuotaExceeded 表示租户当期 token 配额已耗尽（EnforceTenantQuota 唯一非 nil 返回）。
var ErrTenantQuotaExceeded = errors.New("tenant quota exceeded")

// quotaRedis 是配额计数所需的最小 Redis 接口（db.RedisClient 满足；测试可注入 fake）。
type quotaRedis interface {
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd
}

// quotaStore 是配额强制所需的最小数据访问接口（pgEntCostStore 满足）。
type quotaStore interface {
	ResolveTenantID(ctx context.Context, userID string) (string, error)
	TenantTokenPools(ctx context.Context, tenantID string) ([]EntQuotaPool, error)
	TokenUsageSQL(ctx context.Context, tenantID string, since time.Time) (int64, error)
}

// quotaIncrScript 原子自增并刷新月末/日末过期时间，返回自增后的值。
const quotaIncrScript = `
local v = redis.call('INCRBY', KEYS[1], ARGV[1])
redis.call('EXPIREAT', KEYS[1], ARGV[2])
return v`

// tenantQuotaEnforcer 组合 store/redis/时钟，便于单测注入。
type tenantQuotaEnforcer struct {
	store quotaStore
	redis quotaRedis
	now   func() time.Time
}

// EnforceTenantQuota 是聊天请求的租户 token 配额预检（fail-open）。
//
// 语义：
//   - 租户无 token 配额池配置 → 直接放行（nil）。
//   - Redis 计数器 ent:quota:tokens:{tenantID}:{yyyymm|yyyymmdd} INCR（Lua 原子自增 +
//     周期末过期）；缓存键缺失时先从 billing_records SQL 聚合回填。
//   - 任何策略/存储查询失败一律 fail-open（返回 nil + slog.Warn），
//     保证现有聊天链路永不因配额子系统中断。
//
// 接线点（集成任务）：gateway_router.go registerAgentRoutes 中 POST /submit
// 现有 billing 预检（billingMgr.DailyFreeCount / GetBalance）旁，一行调用：
//
//	if err := api.EnforceTenantQuota(r.Context(), r, claims, 0); errors.Is(err, api.ErrTenantQuotaExceeded) {
//	    TooManyRequests(w); return
//	}
func EnforceTenantQuota(ctx context.Context, r *http.Request, claims *auth.Claims, estimatedTokens int64) error {
	if ctx == nil && r != nil {
		ctx = r.Context()
	}
	var rr quotaRedis
	if db.Redis != nil {
		rr = db.Redis
	}
	e := &tenantQuotaEnforcer{store: newPGEntCostStore(), redis: rr, now: time.Now}
	return e.enforce(ctx, claims, estimatedTokens)
}

// enforce 是核心逻辑（可注入依赖，独立可测）。
func (e *tenantQuotaEnforcer) enforce(ctx context.Context, claims *auth.Claims, estimatedTokens int64) error {
	if claims == nil || claims.UserID == "" {
		return nil // 无身份信息无从强制，放行
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if estimatedTokens <= 0 {
		estimatedTokens = 1 // 无预估时最小增量，保持计数器热度
	}

	tenantID := claims.TenantID
	if tenantID == "" {
		var err error
		tenantID, err = e.store.ResolveTenantID(ctx, claims.UserID)
		if err != nil {
			slog.Warn("quota enforce: tenant resolve failed, fail-open",
				"user_id", claims.UserID, "error", err)
			return nil
		}
	}

	pools, err := e.store.TenantTokenPools(ctx, tenantID)
	if err != nil {
		slog.Warn("quota enforce: pool query failed, fail-open",
			"tenant_id", tenantID, "error", err)
		return nil
	}
	if len(pools) == 0 {
		return nil // 该租户无配额池配置 → 直接放行
	}

	for _, pool := range pools {
		if pool.TotalAmount <= 0 {
			continue // 0 = 无限制，仅计数不拦截
		}
		used, ok := e.consume(ctx, tenantID, pool, estimatedTokens)
		if !ok {
			continue // 计数链路任何失败 → fail-open
		}
		if used > pool.TotalAmount {
			return fmt.Errorf("%w: tenant %s token %s used %d/%d",
				ErrTenantQuotaExceeded, tenantID, pool.Period, used, pool.TotalAmount)
		}
	}
	return nil
}

// consume 对单个配额池执行"回填（如需）+ 原子自增"，返回自增后的用量。
// ok=false 表示计数失败，调用方应 fail-open。
func (e *tenantQuotaEnforcer) consume(ctx context.Context, tenantID string, pool EntQuotaPool, delta int64) (int64, bool) {
	if e.redis == nil {
		slog.Warn("quota enforce: redis unavailable, fail-open", "tenant_id", tenantID)
		return 0, false
	}
	now := time.Now()
	if e.now != nil {
		now = e.now()
	}
	key, periodStart, expireAt := quotaPeriodKey(tenantID, pool.Period, now)

	// 缓存键缺失 → 从 billing_records SQL 聚合回填
	if n, err := e.redis.Exists(ctx, key).Result(); err != nil {
		slog.Warn("quota enforce: redis exists failed, fail-open", "key", key, "error", err)
		return 0, false
	} else if n == 0 {
		used, err := e.store.TokenUsageSQL(ctx, tenantID, periodStart)
		if err != nil {
			slog.Warn("quota enforce: usage backfill query failed, fail-open",
				"tenant_id", tenantID, "error", err)
			return 0, false
		}
		if err := e.redis.Set(ctx, key, used, time.Until(expireAt)).Err(); err != nil {
			slog.Warn("quota enforce: usage backfill set failed, fail-open",
				"key", key, "error", err)
			return 0, false
		}
	}

	res, err := e.redis.Eval(ctx, quotaIncrScript, []string{key}, delta, expireAt.Unix()).Result()
	if err != nil {
		slog.Warn("quota enforce: redis incr failed, fail-open", "key", key, "error", err)
		return 0, false
	}
	used, ok := res.(int64)
	if !ok {
		slog.Warn("quota enforce: unexpected incr result, fail-open", "key", key, "result", res)
		return 0, false
	}
	return used, true
}

// quotaPeriodKey 返回计数器键、周期起点与过期时间（UTC）。
// monthly → ent:quota:tokens:{tenant}:{yyyymm}；daily → ent:quota:tokens:{tenant}:{yyyymmdd}。
func quotaPeriodKey(tenantID, period string, now time.Time) (key string, start, expireAt time.Time) {
	now = now.UTC()
	if period == "daily" {
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return fmt.Sprintf("ent:quota:tokens:%s:%s", tenantID, now.Format("20060102")),
			start, start.AddDate(0, 0, 1)
	}
	start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return fmt.Sprintf("ent:quota:tokens:%s:%s", tenantID, now.Format("200601")),
		start, start.AddDate(0, 1, 0)
}

// tenantTokenUsage 读取租户当期 token 用量：优先 Redis 计数器，缺失时
// billing_records SQL 聚合（只读，不回填）。供 QuotaUsage 端点复用。
func tenantTokenUsage(ctx context.Context, store quotaStore, r quotaRedis, tenantID, period string, now time.Time) (int64, string) {
	if r != nil {
		key, _, _ := quotaPeriodKey(tenantID, period, now)
		if v, err := r.Get(ctx, key).Result(); err == nil && v != "" {
			if used, pErr := strconv.ParseInt(v, 10, 64); pErr == nil {
				return used, "redis"
			}
		}
	}
	_, periodStart, _ := quotaPeriodKey(tenantID, period, now)
	used, err := store.TokenUsageSQL(ctx, tenantID, periodStart)
	if err != nil {
		slog.Warn("quota usage: sql aggregation failed", "tenant_id", tenantID, "error", err)
		return 0, "none"
	}
	return used, "sql"
}
