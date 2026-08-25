package enterprise

import (
	"context"
	"testing"
	"time"

	"github.com/athenavi/chiron/internal/db"
	"github.com/redis/go-redis/v9"
)

// 鈹€鈹€ unionPerms锛氬苟闆嗗幓閲?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestUnionPerms(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"nil input returns non-nil empty", nil, []string{}},
		{"empty input returns non-nil empty", []string{}, []string{}},
		{"no duplicates preserved in order", []string{"chat:read", "chat:write"}, []string{"chat:read", "chat:write"}},
		{"duplicates removed keeping first occurrence",
			[]string{"chat:read", "admin:read", "chat:read", "chat:write", "admin:read"},
			[]string{"chat:read", "admin:read", "chat:write"}},
		{"single element", []string{"ent:manage"}, []string{"ent:manage"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unionPerms(tt.input)
			if got == nil {
				t.Fatalf("unionPerms() returned nil, want non-nil slice")
			}
			if len(got) != len(tt.want) {
				t.Fatalf("unionPerms() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("unionPerms() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// 鈹€鈹€ encodePerms / decodePerms锛歯il vs 绌哄垏鐗囧簭鍒楀寲璇箟 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestEncodeDecodePerms(t *testing.T) {
	tests := []struct {
		name      string
		perms     []string
		wantRaw   string
		wantEmpty bool // 瑙ｇ爜鍚庡簲涓洪潪 nil 绌哄垏鐗?	}{
		{"empty perms encodes to JSON empty array", []string{}, "[]", true},
		{"nil normalized to JSON empty array", nil, "[]", true},
		{"non-empty perms roundtrip", []string{"chat:read", "ent:manage"},
			`["chat:read","ent:manage"]`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := encodePerms(tt.perms)
			if err != nil {
				t.Fatalf("encodePerms() error = %v", err)
			}
			if raw != tt.wantRaw {
				t.Fatalf("encodePerms() raw = %q, want %q", raw, tt.wantRaw)
			}

			decoded, ok := decodePerms(raw)
			if !ok {
				t.Fatalf("decodePerms(%q) ok = false, want true", raw)
			}
			// 鍏抽敭鏂█锛氱┖鍒囩墖蹇呴』瑙ｇ爜涓洪潪 nil锛?鏄庣‘鏃犳潈闄?璇箟涓嶅彲涓㈠け锛?			if decoded == nil {
				t.Fatalf("decodePerms(%q) returned nil, want non-nil empty slice", raw)
			}
			if len(decoded) != len(tt.perms) {
				t.Fatalf("decodePerms() = %v, want %v", decoded, tt.perms)
			}
		})
	}
}

func TestDecodePerms_CorruptedOrNilPayload(t *testing.T) {
	for _, raw := range []string{"", "{bad json", "null", `"not-an-array"`, `[1,2]`} {
		if perms, ok := decodePerms(raw); ok {
			t.Errorf("decodePerms(%q) = (%v, true), want (_, false)", raw, perms)
		}
	}
}

// 鈹€鈹€ 缂撳瓨閿笌 TTL 绾﹀畾 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestCacheKeyAndTTL(t *testing.T) {
	if got := permsCacheKeyPrefix + "user-1"; got != "ent:rbac:perms:user-1" {
		t.Fatalf("cache key = %q, want %q", got, "ent:rbac:perms:user-1")
	}
	if permsCacheTTL != 5*time.Minute {
		t.Fatalf("permsCacheTTL = %v, want 5m", permsCacheTTL)
	}
}

// 鈹€鈹€ fake RedisClient锛氱紦瀛樺懡涓?/ 鏈懡涓矾寰?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// fakeRedis 浠呭疄鐜版祴璇曟墍闇€鐨?Get/Set/Del锛屽叾浣欐柟娉曢€氳繃宓屽叆鎺ュ彛
// 淇濇寔 nil锛堟祴璇曡矾寰勪笉浼氳Е杈撅級銆?type fakeRedis struct {
	db.RedisClient
	store   map[string]string
	gets    int
	sets    map[string]string
	deletes []string
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{
		store: make(map[string]string),
		sets:  make(map[string]string),
	}
}

func (f *fakeRedis) Get(ctx context.Context, key string) *redis.StringCmd {
	f.gets++
	cmd := redis.NewStringCmd(ctx, "get", key)
	if v, ok := f.store[key]; ok {
		cmd.SetVal(v)
	} else {
		cmd.SetErr(redis.Nil)
	}
	return cmd
}

func (f *fakeRedis) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	s, _ := value.(string)
	f.sets[key] = s
	f.store[key] = s
	cmd := redis.NewStatusCmd(ctx, "set", key, value, expiration)
	cmd.SetVal("OK")
	return cmd
}

func (f *fakeRedis) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	f.deletes = append(f.deletes, keys...)
	cmd := redis.NewIntCmd(ctx, append([]interface{}{"del"}, toIface(keys)...)...)
	for _, k := range keys {
		delete(f.store, k)
	}
	cmd.SetVal(int64(len(keys)))
	return cmd
}

func toIface(keys []string) []interface{} {
	out := make([]interface{}, len(keys))
	for i, k := range keys {
		out[i] = k
	}
	return out
}

// swapRedis 涓存椂鏇挎崲鍏ㄥ眬 db.Redis 骞跺湪娴嬭瘯缁撴潫鏃惰繕鍘熴€?func swapRedis(t *testing.T, client db.RedisClient) {
	t.Helper()
	old := db.Redis
	db.Redis = client
	t.Cleanup(func() { db.Redis = old })
}

// TestLoadEffectivePerms_CacheHit 缂撳瓨鍛戒腑鏃剁洿鎺ヨ繑鍥炵紦瀛樺€硷紝
// 涓斾笉瑙﹀彂浠讳綍 DB 鍥炲啓锛堟棤 PG 杩炴帴鏃惰嫢鍥炴簮蹇呯劧鎶ラ敊锛夈€?func TestLoadEffectivePerms_CacheHit(t *testing.T) {
	tests := []struct {
		name   string
		cached string
		want   []string
	}{
		{"cached non-empty perms", `["chat:read","ent:manage"]`, []string{"chat:read", "ent:manage"}},
		{"cached empty array keeps non-nil empty semantics", `[]`, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := newFakeRedis()
			fr.store[permsCacheKeyPrefix+"u1"] = tt.cached
			swapRedis(t, fr)

			got, err := LoadEffectivePerms(context.Background(), "u1")
			if err != nil {
				t.Fatalf("LoadEffectivePerms() error = %v", err)
			}
			if got == nil {
				t.Fatalf("LoadEffectivePerms() = nil, want non-nil %v", tt.want)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("LoadEffectivePerms() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("LoadEffectivePerms() = %v, want %v", got, tt.want)
				}
			}
			if len(fr.sets) != 0 {
				t.Fatalf("cache hit must not write back, got sets = %v", fr.sets)
			}
		})
	}
}

// TestLoadEffectivePerms_CorruptedCacheFallback 缂撳瓨鍐呭鎹熷潖鏃舵寜鏈懡涓鐞嗭細
// 鍒犻櫎鑴忛敭骞跺洖婧?DB锛堟棤 PG 姹犳椂杩斿洖閿欒锛岃瘉鏄庢湭浣跨敤鑴忕紦瀛橈級銆?func TestLoadEffectivePerms_CorruptedCacheFallback(t *testing.T) {
	fr := newFakeRedis()
	key := permsCacheKeyPrefix + "u2"
	fr.store[key] = "{corrupted"
	swapRedis(t, fr)

	_, err := LoadEffectivePerms(context.Background(), "u2")
	if err == nil {
		t.Fatalf("LoadEffectivePerms() expected error (DB unavailable), got nil")
	}
	if len(fr.deletes) == 0 || fr.deletes[0] != key {
		t.Fatalf("corrupted cache key should be deleted, deletes = %v", fr.deletes)
	}
}

// TestLoadEffectivePerms_RedisUnavailable 鏃?Redis 鏃堕檷绾х洿鏌?DB锛堜笉鎶?Redis 閿欒锛夛紱
// 鏃?PG 姹犳椂閿欒淇℃伅搴旀寚鍚?DB 鑰岄潪 Redis銆?func TestLoadEffectivePerms_RedisUnavailable(t *testing.T) {
	swapRedis(t, nil)

	_, err := LoadEffectivePerms(context.Background(), "u3")
	if err == nil {
		t.Fatalf("LoadEffectivePerms() expected DB error, got nil")
	}
	if got := err.Error(); got != "ent rbac: postgres pool unavailable" {
		t.Fatalf("error = %q, want postgres pool unavailable", got)
	}
}

// 鈹€鈹€ InvalidateUserPerms / InvalidateGroupMembersPerms 闄嶇骇琛屼负 鈹€鈹€鈹€

func TestInvalidate_NoRedis_NoPanic(t *testing.T) {
	swapRedis(t, nil)
	InvalidateUserPerms(context.Background(), "u1")
	InvalidateGroupMembersPerms(context.Background(), "g1")
}
