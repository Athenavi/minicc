package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/redis/go-redis/v9"
)

// 鈹€鈹€ fake 渚濊禆 鈹€鈹€

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

// 鈹€鈹€ 鐢ㄤ緥 鈹€鈹€

// 鏃犻厤棰濇睜閰嶇疆 鈫?鐩存帴鏀捐锛坣il锛夛紝涓斾笉瑙︾ Redis
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

// 鏃?claims 鈫?鏀捐
func TestEnforceNilClaimsPasses(t *testing.T) {
	e := newTestEnforcer(&fakeQuotaStore{}, &fakeQuotaRedis{})
	if err := e.enforce(context.Background(), nil, 100); err != nil {
		t.Fatalf("nil claims must pass, got %v", err)
	}
}

// 绛栫暐鏌ヨ澶辫触涓€寰?fail-open
func TestEnforceStoreFailureFailOpen(t *testing.T) {
	e := newTestEnforcer(&fakeQuotaStore{poolsErr: errors.New("db down")}, &fakeQuotaRedis{})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 100); err != nil {
		t.Fatalf("pool query failure must fail-open, got %v", err)
	}

	// claims 鏃?TenantID 涓旂鎴疯В鏋愬け璐?鈫?fail-open
	e2 := newTestEnforcer(&fakeQuotaStore{resolveErr: errors.New("db down")}, &fakeQuotaRedis{})
	if err := e2.enforce(context.Background(), &auth.Claims{UserID: "u-1"}, 100); err != nil {
		t.Fatalf("tenant resolve failure must fail-open, got %v", err)
	}
}

// Redis 澶辫触锛堝惈涓嶅彲鐢級涓€寰?fail-open
func TestEnforceRedisFailureFailOpen(t *testing.T) {
	pool := testTokenPool(100)

	// Exists 澶辫触
	e := newTestEnforcer(&fakeQuotaStore{pools: []EntQuotaPool{pool}},
		&fakeQuotaRedis{existsErr: errors.New("redis down")})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 100); err != nil {
		t.Fatalf("redis exists failure must fail-open, got %v", err)
	}

	// Eval锛圛NCRBY锛夊け璐?
	e = newTestEnforcer(&fakeQuotaStore{pools: []EntQuotaPool{pool}},
		&fakeQuotaRedis{existsVal: 1, evalErr: errors.New("redis down")})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 100); err != nil {
		t.Fatalf("redis incr failure must fail-open, got %v", err)
	}

	// Redis 瀹屽叏涓嶅彲鐢紙nil 鎺ュ彛锛?
	e = &tenantQuotaEnforcer{
		store: &fakeQuotaStore{pools: []EntQuotaPool{pool}},
		redis: nil,
		now:   func() time.Time { return time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC) },
	}
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 100); err != nil {
		t.Fatalf("nil redis must fail-open, got %v", err)
	}
}

// 鐢ㄩ噺瓒呭嚭閰嶉 鈫?ErrTenantQuotaExceeded
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

// 鐢ㄩ噺鍦ㄩ厤棰濆唴 鈫?鏀捐
func TestEnforceWithinQuota(t *testing.T) {
	pool := testTokenPool(100)
	e := newTestEnforcer(&fakeQuotaStore{pools: []EntQuotaPool{pool}},
		&fakeQuotaRedis{existsVal: 1, evalVal: 50})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 10); err != nil {
		t.Fatalf("within quota must pass, got %v", err)
	}
}

// total_amount=0锛堟棤闄愬埗锛夆啋 涓嶆嫤鎴?
func TestEnforceUnlimitedPoolPasses(t *testing.T) {
	pool := testTokenPool(0)
	e := newTestEnforcer(&fakeQuotaStore{pools: []EntQuotaPool{pool}},
		&fakeQuotaRedis{existsVal: 1, evalVal: 1e9})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 10); err != nil {
		t.Fatalf("unlimited pool must pass, got %v", err)
	}
}

// 缂撳瓨閿己澶?鈫?浠?billing_records SQL 鑱氬悎鍥炲～鍚庡啀鑷
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

// 鍥炲～鏌ヨ澶辫触 鈫?fail-open
func TestEnforceBackfillFailureFailOpen(t *testing.T) {
	pool := testTokenPool(100)
	store := &fakeQuotaStore{pools: []EntQuotaPool{pool}, usageSQLErr: errors.New("db down")}
	e := newTestEnforcer(store, &fakeQuotaRedis{existsVal: 0})
	if err := e.enforce(context.Background(), quotaTestClaims("t-1"), 5); err != nil {
		t.Fatalf("backfill failure must fail-open, got %v", err)
	}
}

// 璁℃暟鍣ㄩ敭涓庡懆鏈熻竟鐣?
func TestQuotaPeriodKey(t *testing.T) {
	now := time.Date(2026, 8, 17, 3, 0, 0, 0, time.UTC)

	key, start, exp := quotaPeriodKey("t-1", "monthly", now)
	if key != "ent:quota:tokens:t-1:202608" {
		t.Fatalf("monthly key: %s", key)
	}
	if !start.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) ||
		!exp.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("monthly bounds: %v 鈫?%v", start, exp)
	}

	key, start, exp = quotaPeriodKey("t-1", "daily", now)
	if key != "ent:quota:tokens:t-1:20260817" {
		t.Fatalf("daily key: %s", key)
	}
	if exp.Sub(start) != 24*time.Hour {
		t.Fatalf("daily bounds: %v 鈫?%v", start, exp)
	}
}

// tenantTokenUsage锛歊edis 浼樺厛锛岀己澶卞洖閫€ SQL
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

	// Redis 涓?SQL 閮藉け璐?鈫?0/none锛堜笉 panic锛?
	store.usageSQLErr = errors.New("db down")
	used, source = tenantTokenUsage(context.Background(), store, rr, "t-1", "monthly", now)
	if used != 0 || source != "none" {
		t.Fatalf("want 0/none, got %d/%s", used, source)
	}
}

// 瀵煎嚭鍑芥暟绛惧悕鐑熼浘娴嬭瘯锛氭棤 claims 鏃剁洿鎺ユ斁琛岋紙涓嶈Е杈?DB/Redis锛?
func TestEnforceTenantQuotaExportedNoClaims(t *testing.T) {
	if err := EnforceTenantQuota(context.Background(), nil, nil, 10); err != nil {
		t.Fatalf("nil claims must pass, got %v", err)
	}
}
