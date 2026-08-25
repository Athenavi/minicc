package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/athenavi/chiron/internal/auth"
)

// 鈹€鈹€ 瀹¤鏌ヨ鍙傛暟寮哄埗锛堟椂闂磋寖鍥?/ 鍒嗛〉锛?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestParseAuditQuery_DefaultLast7Days(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	f, err := parseAuditQuery(url.Values{}, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.To.Equal(now) {
		t.Fatalf("to = %v, want %v", f.To, now)
	}
	if want := now.Add(-7 * 24 * time.Hour); !f.From.Equal(want) {
		t.Fatalf("from = %v, want default last-7-days %v", f.From, want)
	}
	if f.Page != 1 || f.PageSize != 50 {
		t.Fatalf("expected page=1 page_size=50, got %d/%d", f.Page, f.PageSize)
	}
}

func TestParseAuditQuery_RangeClampedTo7Days(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	q := url.Values{}
	q.Set("from", "2026-01-01")
	q.Set("to", "2026-08-18")
	f, err := parseAuditQuery(q, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := f.To.Sub(f.From); got > 7*24*time.Hour {
		t.Fatalf("range should be clamped to 7d, got %v", got)
	}
}

func TestParseAuditQuery_PageSizeClampedTo100(t *testing.T) {
	q := url.Values{}
	q.Set("page_size", "5000")
	f, err := parseAuditQuery(q, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.PageSize != 100 {
		t.Fatalf("page_size should be clamped to 100, got %d", f.PageSize)
	}
}

func TestParseAuditQuery_InvalidInput(t *testing.T) {
	now := time.Now()
	cases := []url.Values{
		func() url.Values { q := url.Values{}; q.Set("from", "not-a-date"); return q }(),
		func() url.Values { q := url.Values{}; q.Set("to", "not-a-date"); return q }(),
		func() url.Values { // from >= to
			q := url.Values{}
			q.Set("from", now.Format(time.RFC3339))
			q.Set("to", now.Add(-time.Hour).Format(time.RFC3339))
			return q
		}(),
		func() url.Values { q := url.Values{}; q.Set("page", "0"); return q }(),
		func() url.Values { q := url.Values{}; q.Set("page_size", "-1"); return q }(),
	}
	for i, q := range cases {
		if _, err := parseAuditQuery(q, now); err == nil {
			t.Errorf("case %d: expected error, got nil", i)
		}
	}
}

// 鈹€鈹€ AuditMiddleware锛氬啓鏂规硶 + 绠℃帶璺緞 鈫?甯?userID 鐨勫璁?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestAuditMiddleware_RecordsWriteOnScopedPath(t *testing.T) {
	var mu sync.Mutex
	var records []string
	orig := auditMWRecord
	auditMWRecord = func(userID, action, resource, detail, ip string) {
		mu.Lock()
		records = append(records, userID+"|"+action)
		mu.Unlock()
	}
	t.Cleanup(func() { auditMWRecord = orig })

	// 妯℃嫙 authMW 涔嬪悗鐨勮姹傦細ctx 鎼哄甫 claims
	claims := &auth.Claims{UserID: "user-123"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := AuditMiddleware(next)

	r := httptest.NewRequest(http.MethodPost, "/v1/ent/privacy", nil)
	r = r.WithContext(auth.WithClaims(r.Context(), claims))
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	if records[0] != "user-123|POST /v1/ent/privacy" {
		t.Fatalf("unexpected audit record: %q", records[0])
	}
}

func TestAuditMiddleware_SkipsGetAndUnscopedPaths(t *testing.T) {
	var mu sync.Mutex
	count := 0
	orig := auditMWRecord
	auditMWRecord = func(userID, action, resource, detail, ip string) {
		mu.Lock()
		count++
		mu.Unlock()
	}
	t.Cleanup(func() { auditMWRecord = orig })

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mw := AuditMiddleware(next)
	claims := &auth.Claims{UserID: "user-123"}

	paths := []struct{ method, path string }{
		{http.MethodGet, "/v1/ent/privacy"},      // GET 涓嶅璁?
		{http.MethodPost, "/v1/conversations"},   // 闈炵鎺ц矾寰勪笉瀹¤
		{http.MethodDelete, "/v1/plugins/x"},     // 闈炵鎺ц矾寰勪笉瀹¤
	}
	for _, p := range paths {
		r := httptest.NewRequest(p.method, p.path, nil)
		r = r.WithContext(auth.WithClaims(r.Context(), claims))
		w := httptest.NewRecorder()
		mw.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s: expected 200, got %d", p.method, p.path, w.Code)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 audit records, got %d", count)
	}
}

func TestAuditMiddleware_RecordsAdminWrites(t *testing.T) {
	var mu sync.Mutex
	var got []string
	orig := auditMWRecord
	auditMWRecord = func(userID, action, resource, detail, ip string) {
		mu.Lock()
		got = append(got, action)
		mu.Unlock()
	}
	t.Cleanup(func() { auditMWRecord = orig })

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mw := AuditMiddleware(next)
	claims := &auth.Claims{UserID: "admin-1"}

	for _, path := range []string{"/v1/admin/settings", "/admin/maintenance"} {
		r := httptest.NewRequest(http.MethodPut, path, nil)
		r = r.WithContext(auth.WithClaims(r.Context(), claims))
		mw.ServeHTTP(httptest.NewRecorder(), r)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected 2 audit records for admin writes, got %d", len(got))
	}
}
