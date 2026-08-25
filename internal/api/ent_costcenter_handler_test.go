package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/athenavi/chiron/internal/auth"
)

// 鈹€鈹€ 鍐呭瓨鐗?EntCostStore锛坒ake锛?鈹€鈹€
// 璁╂垚鏈腑蹇?handler 鑴辩 PostgreSQL 娴嬭瘯锛坧gEntCostStore 渚濊禆鍏ㄥ眬 db.Pool锛夈€?

type fakeEntStore struct {
	mu sync.Mutex

	pools  map[string]*EntQuotaPool
	allocs map[string][]EntQuotaAllocation // poolID 鈫?allocations
	sums   map[string]int64                // poolID 鈫?宸插垎閰嶅悎璁?

	createPoolErr  error
	createAllocErr error
	deleteAllocHit bool

	tokenPools    []EntQuotaPool
	resolveTenant string
	resolveErr    error
	usageSQL      int64
	usageSQLErr   error

	summary   *entCostSummary
	summaryErr error
	groupCost *entGroupCost
}

func newFakeEntStore() *fakeEntStore {
	return &fakeEntStore{
		pools:  map[string]*EntQuotaPool{},
		allocs: map[string][]EntQuotaAllocation{},
		sums:   map[string]int64{},
	}
}

func (s *fakeEntStore) CostSummary(ctx context.Context, from, to time.Time, groupBy string) (*entCostSummary, error) {
	if s.summaryErr != nil {
		return nil, s.summaryErr
	}
	if s.summary != nil {
		return s.summary, nil
	}
	return &entCostSummary{Rows: []entCostSummaryRow{}}, nil
}

func (s *fakeEntStore) GroupCost(ctx context.Context, groupID string, from, to time.Time) (*entGroupCost, error) {
	if s.groupCost != nil {
		return s.groupCost, nil
	}
	return &entGroupCost{GroupID: groupID, Records: []entGroupCostRow{}}, nil
}

func (s *fakeEntStore) ListQuotaPools(ctx context.Context, tenantID string) ([]EntQuotaPool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []EntQuotaPool{}
	for _, p := range s.pools {
		if tenantID == "" || p.TenantID == tenantID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (s *fakeEntStore) GetQuotaPool(ctx context.Context, id string) (*EntQuotaPool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.pools[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, errQuotaNotFound
}

func (s *fakeEntStore) CreateQuotaPool(ctx context.Context, p *EntQuotaPool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createPoolErr != nil {
		return s.createPoolErr
	}
	p.ID = "pool-fake-1"
	p.CreatedAt, p.UpdatedAt = time.Now(), time.Now()
	cp := *p
	s.pools[p.ID] = &cp
	return nil
}

func (s *fakeEntStore) UpdateQuotaPool(ctx context.Context, p *EntQuotaPool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.pools[p.ID]; !ok {
		return errQuotaNotFound
	}
	p.UpdatedAt = time.Now()
	cp := *p
	s.pools[p.ID] = &cp
	return nil
}

func (s *fakeEntStore) DeleteQuotaPool(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.pools[id]; !ok {
		return errQuotaNotFound
	}
	delete(s.pools, id)
	return nil
}

func (s *fakeEntStore) ListAllocations(ctx context.Context, poolID string) ([]EntQuotaAllocation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]EntQuotaAllocation{}, s.allocs[poolID]...), nil
}

func (s *fakeEntStore) SumAllocated(ctx context.Context, poolID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sums[poolID], nil
}

func (s *fakeEntStore) CreateAllocation(ctx context.Context, a *EntQuotaAllocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createAllocErr != nil {
		return s.createAllocErr
	}
	a.ID = "alloc-fake-1"
	a.CreatedAt = time.Now()
	s.allocs[a.PoolID] = append(s.allocs[a.PoolID], *a)
	s.sums[a.PoolID] += a.Amount
	return nil
}

func (s *fakeEntStore) DeleteAllocation(ctx context.Context, poolID, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteAllocHit, nil
}

func (s *fakeEntStore) TenantTokenPools(ctx context.Context, tenantID string) ([]EntQuotaPool, error) {
	return s.tokenPools, nil
}

func (s *fakeEntStore) TokenUsageSQL(ctx context.Context, tenantID string, since time.Time) (int64, error) {
	return s.usageSQL, s.usageSQLErr
}

func (s *fakeEntStore) ResolveTenantID(ctx context.Context, userID string) (string, error) {
	if s.resolveErr != nil {
		return "", s.resolveErr
	}
	return s.resolveTenant, nil
}

// 鈹€鈹€ 娴嬭瘯杈呭姪 鈹€鈹€

func entOwnerClaims() *auth.Claims {
	return &auth.Claims{UserID: "u-owner", Role: "owner", Perms: auth.RolePermissions["owner"]}
}

func entUserClaims() *auth.Claims {
	return &auth.Claims{UserID: "u-normal", Role: "user", Perms: auth.RolePermissions["user"]}
}

// entDo 鍙戣捣甯?claims 鐨?httptest 璇锋眰骞惰繑鍥炵姸鎬佺爜涓庡搷搴斾綋銆?
func entDo(t *testing.T, h *EntCostCenterHandler, claims *auth.Claims, method, target string, body any) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	if claims != nil {
		req = req.WithContext(auth.WithClaims(req.Context(), claims))
	}
	rec := httptest.NewRecorder()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(rec, req)

	var resp map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	}
	return rec.Code, resp
}

// 鈹€鈹€ 娴嬭瘯鐢ㄤ緥 鈹€鈹€

// 鍒嗛厤瓒呴厤锛歟xisting(80) + requested(30) > total(100) 鈫?422
func TestEntCreateAllocationExceedsPoolReturns422(t *testing.T) {
	store := newFakeEntStore()
	poolID := "31111111-1111-1111-1111-111111111111"
	store.pools[poolID] = &EntQuotaPool{
		ID: poolID, TenantID: "21111111-1111-1111-1111-111111111111",
		ResourceType: "token", TotalAmount: 100, Period: "monthly",
	}
	store.sums[poolID] = 80
	h := NewEntCostCenterHandler(store, nil)

	status, resp := entDo(t, h, entOwnerClaims(), "POST",
		"/v1/ent/quotas/"+poolID+"/allocations",
		map[string]any{
			"target_type": "group",
			"target_id":   "41111111-1111-1111-1111-111111111111",
			"amount":      30,
		})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d (resp=%v)", status, resp)
	}
	if len(store.allocs[poolID]) != 0 {
		t.Fatalf("over-quota allocation must not be persisted, got %v", store.allocs[poolID])
	}
}

// 鍒嗛厤鍦ㄩ搴﹀唴 鈫?201 涓旇惤璐?
func TestEntCreateAllocationWithinPoolReturns201(t *testing.T) {
	store := newFakeEntStore()
	poolID := "31111111-1111-1111-1111-111111111111"
	store.pools[poolID] = &EntQuotaPool{
		ID: poolID, TenantID: "21111111-1111-1111-1111-111111111111",
		ResourceType: "token", TotalAmount: 100, Period: "monthly",
	}
	store.sums[poolID] = 80
	h := NewEntCostCenterHandler(store, nil)

	status, resp := entDo(t, h, entOwnerClaims(), "POST",
		"/v1/ent/quotas/"+poolID+"/allocations",
		map[string]any{
			"target_type": "user",
			"target_id":   "41111111-1111-1111-1111-111111111111",
			"amount":      20,
		})
	if status != http.StatusCreated {
		t.Fatalf("want 201, got %d (resp=%v)", status, resp)
	}
	if len(store.allocs[poolID]) != 1 || store.allocs[poolID][0].Amount != 20 {
		t.Fatalf("allocation not persisted: %v", store.allocs[poolID])
	}
}

// 閰嶉姹犲敮涓€绾︽潫鍐茬獊 鈫?409
func TestEntCreateQuotaConflictReturns409(t *testing.T) {
	store := newFakeEntStore()
	store.createPoolErr = errQuotaConflict
	h := NewEntCostCenterHandler(store, nil)

	status, _ := entDo(t, h, entOwnerClaims(), "POST", "/v1/ent/quotas",
		map[string]any{
			"tenant_id":     "21111111-1111-1111-1111-111111111111",
			"resource_type": "token",
			"total_amount":  1000,
			"period":        "monthly",
		})
	if status != http.StatusConflict {
		t.Fatalf("want 409, got %d", status)
	}
}

// 闈炴硶鏋氫妇 鈫?400
func TestEntCreateQuotaValidationReturns400(t *testing.T) {
	store := newFakeEntStore()
	h := NewEntCostCenterHandler(store, nil)

	status, _ := entDo(t, h, entOwnerClaims(), "POST", "/v1/ent/quotas",
		map[string]any{
			"tenant_id":     "21111111-1111-1111-1111-111111111111",
			"resource_type": "bandwidth", // 闈炴硶璧勬簮绫诲瀷
			"total_amount":  1000,
		})
	if status != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", status)
	}
}

// 鍒涘缓閰嶉姹犳垚鍔?鈫?201
func TestEntCreateQuotaReturns201(t *testing.T) {
	store := newFakeEntStore()
	h := NewEntCostCenterHandler(store, nil)

	status, resp := entDo(t, h, entOwnerClaims(), "POST", "/v1/ent/quotas",
		map[string]any{
			"tenant_id":     "21111111-1111-1111-1111-111111111111",
			"resource_type": "credits",
			"total_amount":  0,
			"period":        "daily",
		})
	if status != http.StatusCreated {
		t.Fatalf("want 201, got %d (resp=%v)", status, resp)
	}
	if len(store.pools) != 1 {
		t.Fatalf("pool not persisted: %v", store.pools)
	}
}

// 鍒楄〃锛氭惡甯?allocated 瀛楁
func TestEntListQuotasIncludesAllocated(t *testing.T) {
	store := newFakeEntStore()
	tenant := "21111111-1111-1111-1111-111111111111"
	store.pools["p1"] = &EntQuotaPool{ID: "p1", TenantID: tenant, ResourceType: "token", TotalAmount: 100, Period: "monthly"}
	store.sums["p1"] = 42
	h := NewEntCostCenterHandler(store, nil)

	status, resp := entDo(t, h, entOwnerClaims(), "GET", "/v1/ent/quotas?tenant_id="+tenant, nil)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d", status)
	}
	data, _ := resp["data"].(map[string]any)
	pools, _ := data["pools"].([]any)
	if len(pools) != 1 {
		t.Fatalf("want 1 pool, got %v", data)
	}
	if allocated := pools[0].(map[string]any)["allocated"]; allocated != float64(42) {
		t.Fatalf("want allocated=42, got %v", allocated)
	}
}

// 涓嶅瓨鍦ㄧ殑閰嶉姹?鈫?404
func TestEntGetQuotaNotFoundReturns404(t *testing.T) {
	h := NewEntCostCenterHandler(newFakeEntStore(), nil)
	status, _ := entDo(t, h, entOwnerClaims(), "GET",
		"/v1/ent/quotas/39999999-9999-9999-9999-999999999999", nil)
	if status != http.StatusNotFound {
		t.Fatalf("want 404, got %d", status)
	}
}

// 鍒犻櫎涓嶅瓨鍦ㄧ殑鍒嗛厤 鈫?404
func TestEntDeleteAllocationNotFoundReturns404(t *testing.T) {
	store := newFakeEntStore()
	store.deleteAllocHit = false
	h := NewEntCostCenterHandler(store, nil)
	status, _ := entDo(t, h, entOwnerClaims(), "DELETE",
		"/v1/ent/quotas/31111111-1111-1111-1111-111111111111/allocations/51111111-1111-1111-1111-111111111111", nil)
	if status != http.StatusNotFound {
		t.Fatalf("want 404, got %d", status)
	}
}

// 鎴愭湰姹囨€伙細閫忎紶 store 缁撴灉
func TestEntCostSummary(t *testing.T) {
	store := newFakeEntStore()
	store.summary = &entCostSummary{
		From: "a", To: "b", GroupBy: "tenant",
		Rows: []entCostSummaryRow{{Key: "t1", TenantID: "t1", CostCents: 77}},
		Totals: entCostSummaryRow{CostCents: 77},
	}
	h := NewEntCostCenterHandler(store, nil)

	status, resp := entDo(t, h, entOwnerClaims(), "GET",
		"/v1/ent/cost/summary?group_by=tenant&from=2026-08-01&to=2026-08-31", nil)
	if status != http.StatusOK {
		t.Fatalf("want 200, got %d (resp=%v)", status, resp)
	}
	data, _ := resp["data"].(map[string]any)
	rows, _ := data["rows"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["cost_cents"] != float64(77) {
		t.Fatalf("unexpected summary rows: %v", data)
	}
}

// 闈炴硶 group_by 鈫?400
func TestEntCostSummaryInvalidGroupByReturns400(t *testing.T) {
	h := NewEntCostCenterHandler(newFakeEntStore(), nil)
	status, _ := entDo(t, h, entOwnerClaims(), "GET", "/v1/ent/cost/summary?group_by=week", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", status)
	}
}

// 鏅€?user 瑙掕壊鏃?admin:read 鈫?403
func TestEntForbiddenForNonAdmin(t *testing.T) {
	h := NewEntCostCenterHandler(newFakeEntStore(), nil)
	status, _ := entDo(t, h, entUserClaims(), "GET", "/v1/ent/cost/summary", nil)
	if status != http.StatusForbidden {
		t.Fatalf("want 403, got %d", status)
	}
	status, _ = entDo(t, h, nil, "GET", "/v1/ent/cost/summary", nil)
	if status != http.StatusForbidden {
		t.Fatalf("want 403 without claims, got %d", status)
	}
}

// 闈?owner 鎼哄甫 TenantID 鏃跺己鍒堕敋瀹氳嚜韬鎴?
func TestEntScopedTenantID(t *testing.T) {
	admin := &auth.Claims{UserID: "u-admin", Role: "admin", TenantID: "t-self", Perms: auth.RolePermissions["admin"]}
	if got := scopedTenantID(admin, "t-other"); got != "t-self" {
		t.Fatalf("admin must be scoped to own tenant, got %s", got)
	}
	owner := &auth.Claims{UserID: "u-owner", Role: "owner", TenantID: "t-self"}
	if got := scopedTenantID(owner, "t-other"); got != "t-other" {
		t.Fatalf("owner may query any tenant, got %s", got)
	}
}

// parseTimeRange 榛樿鍖洪棿涓庢牎楠?
func TestEntParseTimeRange(t *testing.T) {
	from, to, err := parseTimeRange("", "")
	if err != nil || !from.Before(to) {
		t.Fatalf("default range invalid: %v %v %v", from, to, err)
	}
	if _, _, err = parseTimeRange("not-a-date", ""); err == nil {
		t.Fatal("expected error for invalid from")
	}
	if _, _, err = parseTimeRange("2026-08-31", "2026-08-01"); err == nil {
		t.Fatal("expected error when from >= to")
	}
}
