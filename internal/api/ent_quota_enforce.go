package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/redis/go-redis/v9"
)

// ErrTenantQuotaExceeded 琛ㄧず绉熸埛褰撴湡 token 閰嶉宸茶€楀敖锛圗nforceTenantQuota 鍞竴闈?nil 杩斿洖锛夈€?
var ErrTenantQuotaExceeded = errors.New("tenant quota exceeded")

// quotaRedis 鏄厤棰濊鏁版墍闇€鐨勬渶灏?Redis 鎺ュ彛锛坉b.RedisClient 婊¤冻锛涙祴璇曞彲娉ㄥ叆 fake锛夈€?
type quotaRedis interface {
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd
}

// quotaStore 鏄厤棰濆己鍒舵墍闇€鐨勬渶灏忔暟鎹闂帴鍙ｏ紙pgEntCostStore 婊¤冻锛夈€?
type quotaStore interface {
	ResolveTenantID(ctx context.Context, userID string) (string, error)
	TenantTokenPools(ctx context.Context, tenantID string) ([]EntQuotaPool, error)
	TokenUsageSQL(ctx context.Context, tenantID string, since time.Time) (int64, error)
}

// quotaIncrScript 鍘熷瓙鑷骞跺埛鏂版湀鏈?鏃ユ湯杩囨湡鏃堕棿锛岃繑鍥炶嚜澧炲悗鐨勫€笺€?
const quotaIncrScript = `
local v = redis.call('INCRBY', KEYS[1], ARGV[1])
redis.call('EXPIREAT', KEYS[1], ARGV[2])
return v`

// tenantQuotaEnforcer 缁勫悎 store/redis/鏃堕挓锛屼究浜庡崟娴嬫敞鍏ャ€?
type tenantQuotaEnforcer struct {
	store quotaStore
	redis quotaRedis
	now   func() time.Time
}

// EnforceTenantQuota 鏄亰澶╄姹傜殑绉熸埛 token 閰嶉棰勬锛坒ail-open锛夈€?
//
// 璇箟锛?
//   - 绉熸埛鏃?token 閰嶉姹犻厤缃?鈫?鐩存帴鏀捐锛坣il锛夈€?
//   - Redis 璁℃暟鍣?ent:quota:tokens:{tenantID}:{yyyymm|yyyymmdd} INCR锛圠ua 鍘熷瓙鑷 +
//     鍛ㄦ湡鏈繃鏈燂級锛涚紦瀛橀敭缂哄け鏃跺厛浠?billing_records SQL 鑱氬悎鍥炲～銆?
//   - 浠讳綍绛栫暐/瀛樺偍鏌ヨ澶辫触涓€寰?fail-open锛堣繑鍥?nil + slog.Warn锛夛紝
//     淇濊瘉鐜版湁鑱婂ぉ閾捐矾姘镐笉鍥犻厤棰濆瓙绯荤粺涓柇銆?
//
// 鎺ョ嚎鐐癸紙闆嗘垚浠诲姟锛夛細gateway_router.go registerAgentRoutes 涓?POST /submit
// 鐜版湁 billing 棰勬锛坆illingMgr.DailyFreeCount / GetBalance锛夋梺锛屼竴琛岃皟鐢細
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

// enforce 鏄牳蹇冮€昏緫锛堝彲娉ㄥ叆渚濊禆锛岀嫭绔嬪彲娴嬶級銆?
func (e *tenantQuotaEnforcer) enforce(ctx context.Context, claims *auth.Claims, estimatedTokens int64) error {
	if claims == nil || claims.UserID == "" {
		return nil // 鏃犺韩浠戒俊鎭棤浠庡己鍒讹紝鏀捐
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
	}
	if estimatedTokens <= 0 {
		estimatedTokens = 1 // 鏃犻浼版椂鏈€灏忓閲忥紝淇濇寔璁℃暟鍣ㄧ儹搴?
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
		return nil // 璇ョ鎴锋棤閰嶉姹犻厤缃?鈫?鐩存帴鏀捐
	}

	for _, pool := range pools {
		if pool.TotalAmount <= 0 {
			continue // 0 = 鏃犻檺鍒讹紝浠呰鏁颁笉鎷︽埅
		}
		used, ok := e.consume(ctx, tenantID, pool, estimatedTokens)
		if !ok {
			continue // 璁℃暟閾捐矾浠讳綍澶辫触 鈫?fail-open
		}
		if used > pool.TotalAmount {
			return fmt.Errorf("%w: tenant %s token %s used %d/%d",
				ErrTenantQuotaExceeded, tenantID, pool.Period, used, pool.TotalAmount)
		}
	}
	return nil
}

// consume 瀵瑰崟涓厤棰濇睜鎵ц"鍥炲～锛堝闇€锛? 鍘熷瓙鑷"锛岃繑鍥炶嚜澧炲悗鐨勭敤閲忋€?
// ok=false 琛ㄧず璁℃暟澶辫触锛岃皟鐢ㄦ柟搴?fail-open銆?
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

	// 缂撳瓨閿己澶?鈫?浠?billing_records SQL 鑱氬悎鍥炲～
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

// quotaPeriodKey 杩斿洖璁℃暟鍣ㄩ敭銆佸懆鏈熻捣鐐逛笌杩囨湡鏃堕棿锛圲TC锛夈€?
// monthly 鈫?ent:quota:tokens:{tenant}:{yyyymm}锛沝aily 鈫?ent:quota:tokens:{tenant}:{yyyymmdd}銆?
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

// tenantTokenUsage 璇诲彇绉熸埛褰撴湡 token 鐢ㄩ噺锛氫紭鍏?Redis 璁℃暟鍣紝缂哄け鏃?
// billing_records SQL 鑱氬悎锛堝彧璇伙紝涓嶅洖濉級銆備緵 QuotaUsage 绔偣澶嶇敤銆?
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
