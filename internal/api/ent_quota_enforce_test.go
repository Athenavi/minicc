package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/redis/go-redis/v9"
)

// ── fake 依赖 ──

type fakeQuotaStore struct {
	pools       []EntQuotaPool
	poolsErr    error
	resolveID   string
	resolveErr  error
	usageSQL    int64
	usageSQLErr error
}

func (s *fakeQuotaStore) ResolveTenantID(ctx context.Context, userID string) (string, error) {
	if s.resolveErr != nil {
		return "", s.resolveErr
	}
	return s.resolveID, nil
}

func (s *fakeQuotaStore) TenantTokenPools(ctx context.Context, tenantID string) ([]EntQuotaPool, error) {
	return s.pools, s.poolsErr
}

func (s *fakeQuotaStore) TokenUsageSQL(ctx context.Context, tenantID string, since time.Time) (int64, error) {
	return s.usageSQL, s.usageSQLErr
}

type fakeQuotaRedis struct {
	existsVal int64
	existsErr error
	setErr    error
	evalVal   int64
	evalErr   error
	getVal    string
	getErr    error

	setCalls  int
	setKey    string
	evalCalls int
	evalKey   string
	evalDelta int64
}

func (r *fakeQuotaRedis) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	return redis.NewIntResult(r.existsVal, r.existsErr)
}

func (r *fakeQuotaRedis) Get(ctx context.Context, key string) *redis.StringCmd {
	return redis.NewStringResult(r.getVal, r.getErr)
}

func (r *fakeQuotaRedis) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	r.setCalls++
	r.setKey = key
	return redis.NewStatusResult("OK", r.setErr)
}

func (r *fakeQuotaRedis) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	r.evalCalls++
	if len(keys) > 0 {
		r.evalKey = keys[0]
	}
	if len(args) > 0 {
		if d, ok := args[0].(int64); ok {
			r.evalDelta = d
		}
	}
	cmd := redis.NewCmd(ctx)
	if r.evalErr != nil {
		cmd.SetErr(r.evalErr)
		return cmd
	}
	cmd.SetVal(r.evalVal)
	return cmd
}

func newTestEnforcer(store *fakeQuotaStore, rr *fakeQuotaRedis) *tenantQuotaEnforcer {
	return &tenantQuotaEnforcer{
		store: store,
		redis: rr,
		now:   func() time.Time { return time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC) },
	}
}

func quotaTestClaims(tenantID string) *auth.Claims {
	return &auth.Claims{UserID: "u-1", TenantID: tenantID}
}

func testTokenPool(total int64) EntQuotaPool {
	return EntQuotaPool{
		ID: "p-1", TenantID: "t-1", ResourceType: "token",
		TotalAmount: total, Period: "monthly",
	}
}

// ── 用例 ──

// 无配额池配置 → 直接放行（nil），且不触碰 Redis
func TestEnforceNoQuotaConfigPasses(t *testing.T) {
	rr := &fakeQuotaRedis{}
	e := newTestEnforcer(&fakeQuotaStore{pools: nil}, rr)
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 100); err != nil {
		t.Fatalf("no quota config must pass, got %v", err)
	}
	if rr.existsErr == nil && (rr.setCalls > 0 || rr.evalCalls > 0) {
		t.Fatal("redis must not be touched without quota pools")
	}
}

// 无 claims → 放行
func TestEnforceNilClaimsPasses(t *testing.T) {
	e := newTestEnforcer(&fakeQuotaStore{}, &fakeQuotaRedis{})
	if err := e.enforce(context.Background(), nil, 100); err != nil {
		t.Fatalf("nil claims must pass, got %v", err)
	}
}

// 策略查询失败一律 fail-open
func TestEnforceStoreFailureFailOpen(t *testing.T) {
	e := newTestEnforcer(&fakeQuotaStore{poolsErr: errors.New("db down")}, &fakeQuotaRedis{})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 100); err != nil {
		t.Fatalf("pool query failure must fail-open, got %v", err)
	}

	// claims 无 TenantID 且租户解析失败 → fail-open
	e2 := newTestEnforcer(&fakeQuotaStore{resolveErr: errors.New("db down")}, &fakeQuotaRedis{})
	if err := e2.enforce(context.Background(), &auth.Claims{UserID: "u-1"}, 100); err != nil {
		t.Fatalf("tenant resolve failure must fail-open, got %v", err)
	}
}

// Redis 失败（含不可用）一律 fail-open
func TestEnforceRedisFailureFailOpen(t *testing.T) {
	pool := testTokenPool(100)

	// Exists 失败
	e := newTestEnforcer(&fakeQuotaStore{pools: []EntQuotaPool{pool}},
		&fakeQuotaRedis{existsErr: errors.New("redis down")})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 100); err != nil {
		t.Fatalf("redis exists failure must fail-open, got %v", err)
	}

	// Eval（INCRBY）失败
	e = newTestEnforcer(&fakeQuotaStore{pools: []EntQuotaPool{pool}},
		&fakeQuotaRedis{existsVal: 1, evalErr: errors.New("redis down")})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 100); err != nil {
		t.Fatalf("redis incr failure must fail-open, got %v", err)
	}

	// Redis 完全不可用（nil 接口）
	e = &tenantQuotaEnforcer{
		store: &fakeQuotaStore{pools: []EntQuotaPool{pool}},
		redis: nil,
		now:   func() time.Time { return time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC) },
	}
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 100); err != nil {
		t.Fatalf("nil redis must fail-open, got %v", err)
	}
}

// 用量超出配额 → ErrTenantQuotaExceeded
func TestEnforceQuotaExceeded(t *testing.T) {
	pool := testTokenPool(100)
	rr := &fakeQuotaRedis{existsVal: 1, evalVal: 150}
	e := newTestEnforcer(&fakeQuotaStore{pools: []EntQuotaPool{pool}}, rr)

	err := e.enforce(context.Background(), quotaTestClaims("t-1"), 50)
	if !errors.Is(err, ErrTenantQuotaExceeded) {
		t.Fatalf("want ErrTenantQuotaExceeded, got %v", err)
	}
	if rr.evalKey != "ent:quota:tokens:t-1:202608" {
		t.Fatalf("unexpected counter key: %s", rr.evalKey)
	}
	if rr.evalDelta != 50 {
		t.Fatalf("want delta=50, got %d", rr.evalDelta)
	}
}

// 用量在配额内 → 放行
func TestEnforceWithinQuota(t *testing.T) {
	pool := testTokenPool(100)
	e := newTestEnforcer(&fakeQuotaStore{pools: []EntQuotaPool{pool}},
		&fakeQuotaRedis{existsVal: 1, evalVal: 50})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 10); err != nil {
		t.Fatalf("within quota must pass, got %v", err)
	}
}

// total_amount=0（无限制）→ 不拦截
func TestEnforceUnlimitedPoolPasses(t *testing.T) {
	pool := testTokenPool(0)
	e := newTestEnforcer(&fakeQuotaStore{pools: []EntQuotaPool{pool}},
		&fakeQuotaRedis{existsVal: 1, evalVal: 1e9})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 10); err != nil {
		t.Fatalf("unlimited pool must pass, got %v", err)
	}
}

// 缓存键缺失 → 从 billing_records SQL 聚合回填后再自增
func TestEnforceBackfillsMissingCounter(t *testing.T) {
	pool := testTokenPool(100)
	store := &fakeQuotaStore{pools: []EntQuotaPool{pool}, usageSQL: 40}
	rr := &fakeQuotaRedis{existsVal: 0, evalVal: 45}
	e := newTestEnforcer(store, rr)

	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 5); err != nil {
		t.Fatalf("backfilled usage within quota must pass, got %v", err)
	}
	if rr.setCalls != 1 || rr.setKey != "ent:quota:tokens:t-1:202608" {
		t.Fatalf("expected one backfill Set on counter key, got calls=%d key=%s",
			rr.setCalls, rr.setKey)
	}
	if rr.evalCalls != 1 {
		t.Fatalf("expected incr after backfill, got evalCalls=%d", rr.evalCalls)
	}
}

// 回填查询失败 → fail-open
func TestEnforceBackfillFailureFailOpen(t *testing.T) {
	pool := testTokenPool(100)
	store := &fakeQuotaStore{pools: []EntQuotaPool{pool}, usageSQLErr: errors.New("db down")}
	e := newTestEnforcer(store, &fakeQuotaRedis{existsVal: 0})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 5); err != nil {
		t.Fatalf("backfill failure must fail-open, got %v", err)
	}
}

// 计数器键与周期边界
func TestQuotaPeriodKey(t *testing.T) {
	now := time.Date(2026, 8, 17, 3, 0, 0, 0, time.UTC)

	key, start, exp := quotaPeriodKey("t-1", "monthly", now)
	if key != "ent:quota:tokens:t-1:202608" {
		t.Fatalf("monthly key: %s", key)
	}
	if !start.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) ||
		!exp.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("monthly bounds: %v → %v", start, exp)
	}

	key, start, exp = quotaPeriodKey("t-1", "daily", now)
	if key != "ent:quota:tokens:t-1:20260817" {
		t.Fatalf("daily key: %s", key)
	}
	if exp.Sub(start) != 24*time.Hour {
		t.Fatalf("daily bounds: %v → %v", start, exp)
	}
}

// tenantTokenUsage：Redis 优先，缺失回退 SQL
func TestTenantTokenUsageSources(t *testing.T) {
	store := &fakeQuotaStore{usageSQL: 321}
	rr := &fakeQuotaRedis{getVal: "123"}
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)

	used, source := tenantTokenUsage(context.Background(), store, rr, "t-1", "monthly", now)
	if used != 123 || source != "redis" {
		t.Fatalf("want 123/redis, got %d/%s", used, source)
	}

	rr = &fakeQuotaRedis{getErr: redis.Nil}
	used, source = tenantTokenUsage(context.Background(), store, rr, "t-1", "monthly", now)
	if used != 321 || source != "sql" {
		t.Fatalf("want 321/sql, got %d/%s", used, source)
	}

	// Redis 与 SQL 都失败 → 0/none（不 panic）
	store.usageSQLErr = errors.New("db down")
	used, source = tenantTokenUsage(context.Background(), store, rr, "t-1", "monthly", now)
	if used != 0 || source != "none" {
		t.Fatalf("want 0/none, got %d/%s", used, source)
	}
}

// 导出函数签名烟雾测试：无 claims 时直接放行（不触达 DB/Redis）
func TestEnforceTenantQuotaExportedNoClaims(t *testing.T) {
	if err := EnforceTenantQuota(context.Background(), nil, nil, 10); err != nil {
		t.Fatalf("nil claims must pass, got %v", err)
	}
}
