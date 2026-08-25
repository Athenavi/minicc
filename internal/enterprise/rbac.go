package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/athenavi/chiron/internal/db"
	"github.com/redis/go-redis/v9"
)

const (
	// permsCacheKeyPrefix 鏄湁鏁堟潈闄愮紦瀛橀敭鍓嶇紑锛屽畬鏁撮敭涓?ent:rbac:perms:{userID}
	permsCacheKeyPrefix = "ent:rbac:perms:"
	// permsCacheTTL 鏉冮檺缂撳瓨杩囨湡鏃堕棿
	permsCacheTTL = 5 * time.Minute
)

// LoadEffectivePerms 鑱氬悎鐢ㄦ埛鐨勪紒涓氱骇鏈夋晥鏉冮檺锛?// 鐢ㄦ埛鐩存帴瑙掕壊鏉冮檺 鈭?鐢ㄦ埛鎵€鍦ㄧ兢缁勨啋缇ょ粍瑙掕壊鏉冮檺锛坋nt_user_roles鈫抏nt_roles 鈭?// ent_group_members鈫抏nt_group_roles鈫抏nt_roles锛夛紝permissions text[] 骞堕泦鍘婚噸銆?//
// 杩斿洖鍊艰涔夛紙闃茶秺鏉冪殑鍏抽敭鍖哄垎锛夛細
//   - nil, nil锛氱敤鎴锋病鏈変换浣?ent 瑙掕壊璁板綍锛堢洿鎺?+ 缇ょ粍鍧囨湭鍏宠仈锛夛紝
//     璋冪敤鏂瑰簲鍥為€€鏃ф潈闄愪綋绯伙紙auth.HasPermission锛夈€傛鐘舵€佷笉鍐欑紦瀛樸€?//   - 闈?nil 绌哄垏鐗? nil锛氱敤鎴锋湁瑙掕壊璁板綍浣嗚仛鍚堟潈闄愪负绌猴紝琛ㄧず"鏄庣‘鏃犳潈闄?锛?//     璋冪敤鏂圭姝㈠洖閫€鏃т綋绯汇€傛鐘舵€佷互 "[]" 搴忓垪鍖栫紦瀛樸€?//   - 闈炵┖鍒囩墖, nil锛氳仛鍚堝悗鐨勬潈闄愮偣鍒楄〃锛堝凡鍘婚噸锛夈€?//
// Redis 涓嶅彲鐢ㄦ垨璇诲彇澶辫触鏃堕檷绾х洿鏌?DB锛坰log.Warn锛屼笉杩斿洖閿欒锛夛紱
// DB 鏌ヨ澶辫触杩斿洖 error锛堢敱璋冪敤鏂瑰喅瀹?fail-open/fail-close 绛栫暐锛夈€?func LoadEffectivePerms(ctx context.Context, userID string) ([]string, error) {
	// 鈹€鈹€ 1. 缂撳瓨璇诲彇锛堝懡涓洿鎺ヨ繑鍥烇級鈹€鈹€
	cacheKey := permsCacheKeyPrefix + userID
	if rdb := db.Redis; rdb != nil {
		cached, err := rdb.Get(ctx, cacheKey).Result()
		if err == nil {
			if perms, ok := decodePerms(cached); ok {
				return perms, nil
			}
			// 缂撳瓨鍐呭鎹熷潖锛氬垹闄よ剰閿悗鍥炴簮 DB
			if err := rdb.Del(ctx, cacheKey).Err(); err != nil {
				slog.Warn("ent rbac: failed to delete corrupted cache key",
					"user_id", userID, "error", err)
			}
		} else if !errors.Is(err, redis.Nil) {
			slog.Warn("ent rbac: redis get failed, falling back to DB",
				"user_id", userID, "error", err)
		}
	} else {
		slog.Warn("ent rbac: redis unavailable, falling back to DB", "user_id", userID)
	}

	// 鈹€鈹€ 2. PG 鑱氬悎鏌ヨ 鈹€鈹€
	perms, hasRoles, err := queryEffectivePerms(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !hasRoles {
		// 鏃?ent 瑙掕壊璁板綍锛氫笉缂撳瓨锛坣il 璇箟锛夛紝浜ょ敱璋冪敤鏂瑰洖閫€鏃ф潈闄愪綋绯?		return nil, nil
	}

	// 鈹€鈹€ 3. 鍥炲啓缂撳瓨锛堝惈绌哄垏鐗囷細"[]" 淇濊瘉"鏄庣‘鏃犳潈闄?璇箟鍙紦瀛橈級鈹€鈹€
	if rdb := db.Redis; rdb != nil {
		if encoded, err := encodePerms(perms); err == nil {
			if err := rdb.Set(ctx, cacheKey, encoded, permsCacheTTL).Err(); err != nil {
				slog.Warn("ent rbac: redis set failed", "user_id", userID, "error", err)
			}
		}
	}

	return perms, nil
}

// queryEffectivePerms 浠?PG 鑱氬悎鐢ㄦ埛鐨勬湁鏁堟潈闄愩€?// 杩斿洖 (骞堕泦鍘婚噸鍚庣殑鏉冮檺鍒楄〃, 鏄惁瀛樺湪浠讳綍瑙掕壊鍏宠仈, error)銆?func queryEffectivePerms(ctx context.Context, userID string) ([]string, bool, error) {
	pool := db.ReadPool()
	if pool == nil {
		return nil, false, errors.New("ent rbac: postgres pool unavailable")
	}

	const query = `
SELECT COUNT(DISTINCT r.id)::int AS role_count,
       COALESCE(array_agg(DISTINCT p ORDER BY p) FILTER (WHERE p IS NOT NULL), '{}')::text[] AS perms
FROM (
    SELECT r.id, r.permissions
    FROM ent_user_roles ur
    JOIN ent_roles r ON r.id = ur.role_id
    WHERE ur.user_id = $1
    UNION ALL
    SELECT r.id, r.permissions
    FROM ent_group_members gm
    JOIN ent_group_roles gr ON gr.group_id = gm.group_id
    JOIN ent_roles r ON r.id = gr.role_id
    WHERE gm.user_id = $1
) AS role_grants
JOIN ent_roles r ON r.id = role_grants.id
CROSS JOIN LATERAL unnest(COALESCE(r.permissions, '{}')) AS p`

	var roleCount int
	var perms []string
	if err := pool.QueryRow(ctx, query, userID).Scan(&roleCount, &perms); err != nil {
		return nil, false, err
	}
	if roleCount == 0 {
		return nil, false, nil
	}
	return unionPerms(perms), true, nil
}

// InvalidateUserPerms 鍒犻櫎鎸囧畾鐢ㄦ埛鐨勬湁鏁堟潈闄愮紦瀛橀敭銆?// Redis 涓嶅彲鐢ㄦ垨鍒犻櫎澶辫触鏃朵粎璁板綍鏃ュ織锛堢紦瀛樻渶澶?5 鍒嗛挓鑷劧杩囨湡锛夈€?func InvalidateUserPerms(ctx context.Context, userID string) {
	rdb := db.Redis
	if rdb == nil {
		return
	}
	if err := rdb.Del(ctx, permsCacheKeyPrefix+userID).Err(); err != nil {
		slog.Warn("ent rbac: invalidate user perms cache failed",
			"user_id", userID, "error", err)
	}
}

// InvalidateGroupMembersPerms 鎵归噺澶辨晥缇ょ粍鎵€鏈夋垚鍛樼殑鏉冮檺缂撳瓨銆?// 渚涚兢缁?瑙掕壊鍐欐搷浣滐紙淇敼缇ょ粍鎴愬憳銆佺兢缁勮鑹茬粦瀹氥€佽鑹叉潈闄愬彉鏇达級鍚庤皟鐢ㄣ€?// 鎴愬憳鏌ヨ澶辫触鏃朵粎璁板綍鏃ュ織锛堜緷璧?TTL 鑷劧杩囨湡鍏滃簳锛屼笉闃绘柇鍐欒矾寰勶級銆?func InvalidateGroupMembersPerms(ctx context.Context, groupID string) {
	rdb := db.Redis
	if rdb == nil {
		return
	}

	pool := db.ReadPool()
	if pool == nil {
		slog.Warn("ent rbac: postgres pool unavailable, skip group member cache invalidation",
			"group_id", groupID)
		return
	}

	rows, err := pool.Query(ctx,
		`SELECT user_id FROM ent_group_members WHERE group_id = $1`, groupID)
	if err != nil {
		slog.Warn("ent rbac: query group members failed", "group_id", groupID, "error", err)
		return
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var memberID string
		if err := rows.Scan(&memberID); err != nil {
			slog.Warn("ent rbac: scan group member failed", "group_id", groupID, "error", err)
			continue
		}
		keys = append(keys, permsCacheKeyPrefix+memberID)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("ent rbac: iterate group members failed", "group_id", groupID, "error", err)
	}
	if len(keys) == 0 {
		return
	}

	if err := rdb.Del(ctx, keys...).Err(); err != nil {
		slog.Warn("ent rbac: batch invalidate member perms cache failed",
			"group_id", groupID, "members", len(keys), "error", err)
	}
}

// unionPerms 瀵规潈闄愬垪琛ㄥ幓閲嶏紙淇濇寔棣栨鍑虹幇椤哄簭锛夈€傜函鍑芥暟锛屼究浜庡崟鍏冩祴璇曘€?func unionPerms(perms []string) []string {
	if len(perms) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(perms))
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// encodePerms 灏嗛潪 nil 鏉冮檺鍒囩墖搴忓垪鍖栦负缂撳瓨鍊硷紙绌哄垏鐗?鈫?"[]"锛夈€?// nil 涓嶅簲杩涘叆缂撳瓨锛屾晠鍏ュ弬绾﹀畾涓洪潪 nil銆?func encodePerms(perms []string) (string, error) {
	if perms == nil {
		perms = []string{}
	}
	data, err := json.Marshal(perms)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// decodePerms 鍙嶅簭鍒楀寲缂撳瓨鍊笺€俹k=false 琛ㄧず缂撳瓨鎹熷潖锛屽簲鎸夋湭鍛戒腑澶勭悊銆?// "[]" 瑙ｇ爜涓洪潪 nil 绌哄垏鐗囷紙鏄庣‘鏃犳潈闄愶級锛屼笌 nil锛堟湭閰嶇疆銆佷笉鍏ョ紦瀛橈級鍖哄垎銆?func decodePerms(raw string) ([]string, bool) {
	var perms []string
	if err := json.Unmarshal([]byte(raw), &perms); err != nil || perms == nil {
		return nil, false
	}
	return perms, true
}
