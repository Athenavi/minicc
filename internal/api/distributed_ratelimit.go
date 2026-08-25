package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
)

// DistributedRateLimiter 鍩轰簬 Redis 鐨勫垎甯冨紡闄愭祦鍣?// 鏀寔澶氱骇闄愭祦锛氬叏灞€銆佺鎴枫€佺敤鎴?type DistributedRateLimiter struct {
	rdb db.RedisClient

	// 闄愭祦閰嶇疆
	globalLimit  int // 鍏ㄥ眬姣忓垎閽熻姹傛暟
	tenantLimit  int // 姣忕鎴锋瘡鍒嗛挓璇锋眰鏁?	userLimit    int // 姣忕敤鎴锋瘡鍒嗛挓璇锋眰鏁?}

// NewDistributedRateLimiter 鍒涘缓鍒嗗竷寮忛檺娴佸櫒
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

// Configure 杩愯鏃剁儹鏇存柊涓夌骇闄愭祦闃堝€硷紙闃堝€?鈮? 琛ㄧず涓嶇敓鏁?璺宠繃锛夈€?// Allow 姣忔璇诲彇瀛楁锛屽洜姝ゆ敼瀹屽嵆鍒荤敓鏁堬紝渚涘悗鍙般€岀郴缁熻缃€嶈皟鐢ㄣ€?func (l *DistributedRateLimiter) Configure(globalLimit, tenantLimit, userLimit int) {
	if globalLimit > 0 {
		l.globalLimit = globalLimit
	}
	if tenantLimit > 0 {
		l.tenantLimit = tenantLimit
	}
	if userLimit > 0 {
		l.userLimit = userLimit
	}
}

// rateLimitLua 涓夌骇闄愭祦鍘熷瓙鑴氭湰 鈥?棰勬鏌ュ叏閮ㄤ笁绾у悗缁熶竴閫掑锛岄槻姝㈤厤棰濇硠婕?//
// KEYS[1]  鍏ㄥ眬 key     KEYS[2] 绉熸埛 key锛堢┖涓插垯璺宠繃锛? KEYS[3] 鐢ㄦ埛 key锛堢┖涓插垯璺宠繃锛?// ARGV[1]  鍏ㄥ眬涓婇檺     ARGV[2] 绉熸埛涓婇檺               ARGV[3] 鐢ㄦ埛涓婇檺
// ARGV[4]  绐楀彛绉掓暟
// 杩斿洖 "ok" / "global" / "tenant" / "user"
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

// Allow 妫€鏌ユ槸鍚﹀厑璁歌姹?鈥?鍗曟鍘熷瓙 eval 瀹屾垚涓夌骇妫€鏌?// fail-close 绛栫暐锛歊edis 涓嶅彲鐢ㄦ垨 Eval 閿欒鏃舵嫆缁濊姹傦紙鐢熶骇瀹夊叏浼樺厛锛?func (l *DistributedRateLimiter) Allow(ctx context.Context, tenantID, userID string) (bool, error) {
	if l.rdb == nil {
		return false, fmt.Errorf("闄愭祦 Redis 涓嶅彲鐢紝鎸?fail-close 鎷掔粷璇锋眰")
	}
	if tenantID == "" {
		// 鏈璇佸叕寮€绔偣锛坕nstall/login/register/health 绛夛級鍏辩敤 public 妗堕檺娴侊紝
		// 闃叉鍗曚竴鏉ユ簮婊ョ敤锛屼絾涓嶆嫆缁濓紙鍚﹀垯 install 棣栨閮ㄧ讲鏃犳硶瀹屾垚锛?		tenantID = "public"
	}

	globalKey := "ratelimit:global:minute"
	tenantKey := fmt.Sprintf("ratelimit:tenant:%s:minute", tenantID)
	userKey := ""
	if userID != "" {
		userKey = fmt.Sprintf("ratelimit:user:%s:minute", userID)
	}

	// 闄愭祦澶辨晥鐨勫弬鏁帮紙limit鈮?锛夌洿鎺ヨ烦杩?	tenantLim := l.tenantLimit
	userLim := l.userLimit
	if userKey == "" {
		userLim = 0
	}

	result, err := l.rdb.Eval(ctx, rateLimitLua,
		[]string{globalKey, tenantKey, userKey},
		l.globalLimit, tenantLim, userLim, 60).Text()
	if err != nil {
		slog.Error("闄愭祦妫€鏌ュけ璐ワ紙fail-close锛?, "error", err, "tenant", tenantID)
		return false, fmt.Errorf("闄愭祦鏈嶅姟鏆傛椂涓嶅彲鐢? %w", err)
	}

	switch result {
	case "global":
		return false, fmt.Errorf("鍏ㄥ眬璇锋眰棰戠巼瓒呴檺")
	case "tenant":
		return false, fmt.Errorf("绉熸埛 %s 璇锋眰棰戠巼瓒呴檺", tenantID)
	case "user":
		return false, fmt.Errorf("鐢ㄦ埛 %s 璇锋眰棰戠巼瓒呴檺", userID)
	default:
		return true, nil
	}
}

// DistributedRateLimitMiddleware 鍒嗗竷寮忛檺娴佷腑闂翠欢
func DistributedRateLimitMiddleware(limiter *DistributedRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 鎻愬彇 tenant_id 涓?user_id锛堝绉熸埛闅旂閿級
		var tenantID, userID string
		claims := auth.GetClaims(r.Context())
		if claims != nil {
			tenantID = claims.TenantID
			userID = claims.UserID
		}

		allowed, err := limiter.Allow(r.Context(), tenantID, userID)
			if err != nil {
				slog.Warn("闄愭祦瑙﹀彂",
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
