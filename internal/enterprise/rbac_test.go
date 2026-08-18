package enterprise

import (
	"context"
	"testing"
	"time"

	"github.com/athenavi/minicc/internal/db"
	"github.com/redis/go-redis/v9"
)

// ── unionPerms：并集去重 ─────────────────────────────────────────

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

// ── encodePerms / decodePerms：nil vs 空切片序列化语义 ───────────

func TestEncodeDecodePerms(t *testing.T) {
	tests := []struct {
		name      string
		perms     []string
		wantRaw   string
		wantEmpty bool // 解码后应为非 nil 空切片
	}{
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
			// 关键断言：空切片必须解码为非 nil（"明确无权限"语义不可丢失）
			if decoded == nil {
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

// ── 缓存键与 TTL 约定 ──────────────────────────────────────────

func TestCacheKeyAndTTL(t *testing.T) {
	if got := permsCacheKeyPrefix + "user-1"; got != "ent:rbac:perms:user-1" {
		t.Fatalf("cache key = %q, want %q", got, "ent:rbac:perms:user-1")
	}
	if permsCacheTTL != 5*time.Minute {
		t.Fatalf("permsCacheTTL = %v, want 5m", permsCacheTTL)
	}
}

// ── fake RedisClient：缓存命中 / 未命中路径 ──────────────────────

// fakeRedis 仅实现测试所需的 Get/Set/Del，其余方法通过嵌入接口
// 保持 nil（测试路径不会触达）。
type fakeRedis struct {
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

// swapRedis 临时替换全局 db.Redis 并在测试结束时还原。
func swapRedis(t *testing.T, client db.RedisClient) {
	t.Helper()
	old := db.Redis
	db.Redis = client
	t.Cleanup(func() { db.Redis = old })
}

// TestLoadEffectivePerms_CacheHit 缓存命中时直接返回缓存值，
// 且不触发任何 DB 回写（无 PG 连接时若回源必然报错）。
func TestLoadEffectivePerms_CacheHit(t *testing.T) {
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

// TestLoadEffectivePerms_CorruptedCacheFallback 缓存内容损坏时按未命中处理：
// 删除脏键并回源 DB（无 PG 池时返回错误，证明未使用脏缓存）。
func TestLoadEffectivePerms_CorruptedCacheFallback(t *testing.T) {
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

// TestLoadEffectivePerms_RedisUnavailable 无 Redis 时降级直查 DB（不报 Redis 错误）；
// 无 PG 池时错误信息应指向 DB 而非 Redis。
func TestLoadEffectivePerms_RedisUnavailable(t *testing.T) {
	swapRedis(t, nil)

	_, err := LoadEffectivePerms(context.Background(), "u3")
	if err == nil {
		t.Fatalf("LoadEffectivePerms() expected DB error, got nil")
	}
	if got := err.Error(); got != "ent rbac: postgres pool unavailable" {
		t.Fatalf("error = %q, want postgres pool unavailable", got)
	}
}

// ── InvalidateUserPerms / InvalidateGroupMembersPerms 降级行为 ───

func TestInvalidate_NoRedis_NoPanic(t *testing.T) {
	swapRedis(t, nil)
	InvalidateUserPerms(context.Background(), "u1")
	InvalidateGroupMembersPerms(context.Background(), "g1")
}
