package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/broadcast"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/session"
)

// testRouter builds a fully wired router for integration testing.
// Uses minimal config with local-only storage and no external dependencies.
func testRouter(t *testing.T) http.Handler {
	t.Helper()

	// Config (minimal 鈥?no DB, no Redis)
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
	os.Setenv("APP_SECRET", "test-app-secret-that-is-at-least-32-bytes-long!")
	cfg := config.Load()

	// Event Hub
	eventHub := broadcast.NewHub(nil)

	// Session Manager
	sessionMgr := session.NewManager(nil, nil)

	return NewGatewayRouter(cfg, nil, eventHub, sessionMgr, nil, nil, nil)
}

// testToken generates a valid JWT token for testing.
func testToken(t *testing.T) string {
	t.Helper()
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
	os.Setenv("APP_SECRET", "test-app-secret-that-is-at-least-32-bytes-long!")
	cfg := config.Load()
	authenticator := auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration)
	token, err := authenticator.GenerateToken("test-user-id", "test@example.com", "user", db.DefaultTenantID, auth.RolePermissions["user"])
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return token
}

// adminToken 杩斿洖 admin 瑙掕壊鐨?JWT锛岀敤浜庨獙璇侀渶瑕佺鐞嗙鏉冮檺鐨勮矾鐢便€?func adminToken(t *testing.T) string {
	t.Helper()
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
	os.Setenv("APP_SECRET", "test-app-secret-that-is-at-least-32-bytes-long!")
	cfg := config.Load()
	authenticator := auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration)
	token, err := authenticator.GenerateToken("admin-id", "admin@example.com", "admin", db.DefaultTenantID, auth.RolePermissions["admin"])
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	return token
}

// 鈹€鈹€ Health & Readiness 鈹€鈹€

func TestIntegration_Health(t *testing.T) {
	router := testRouter(t)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIntegration_Ready(t *testing.T) {
	router := testRouter(t)
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	// 鏂板绾?2026-08-22)锛?ready 鍙嶆槧鐪熷疄渚濊禆鐘舵€侊紱娴嬭瘯鐜鏃?DB/Redis 鈫?503 + deps down
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (deps missing), got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "redis\":\"down") || !strings.Contains(string(body), "postgres\":\"down") {
		t.Fatalf("expected deps down payload, got %s", string(body))
	}
}

// 鈹€鈹€ Auth Endpoints 鈹€鈹€

// 鈹€鈹€ SSE & Events 鈹€鈹€

func TestIntegration_SSE(t *testing.T) {
	// 璇ユ祴璇曟棤鏁版嵁搴擄細sessionMgr 浼?nil 璺宠繃褰掑睘鏍￠獙锛岃仛鐒︽祦寮忚涓烘湰韬€?	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
	os.Setenv("APP_SECRET", "test-app-secret-that-is-at-least-32-bytes-long!")
	cfg := config.Load()
	eventHub := broadcast.NewHub(nil)
	router := NewGatewayRouter(cfg, nil, eventHub, nil, nil, nil, nil)

	// Use a cancellable context so the SSE handler exits
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// S1: /events now requires auth 鈥?Authorization header锛圫 瀹夊叏淇锛氫笉鍐嶆敮鎸??token= 鏌ヨ鍙傛暟锛?	req := httptest.NewRequest("GET", "/events?client_id=test-client&session_id=test-session", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(t))
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	// Run SSE handler in a goroutine since it blocks
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(w, req)
		close(done)
	}()

	// Give it a moment to write the connected event, then cancel
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected 'text/event-stream', got %q", resp.Header.Get("Content-Type"))
	}
}

// TestIntegration_SSE_NoAuth asserts /events now REQUIRES authentication (S1 fix).
func TestIntegration_SSE_NoAuth(t *testing.T) {
	router := testRouter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := httptest.NewRequest("GET", "/events?client_id=unauth-client", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 (auth required), got %d", w.Code)
	}
}

// 鈹€鈹€ Conversations 鈹€鈹€

func TestIntegration_Conversations_NoAuth(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/v1/conversations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	// Without auth, should return 401
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestIntegration_CreateConversation(t *testing.T) {
	router := testRouter(t)

	body := `{"id":"test-sess-1","title":"Test Chat"}`
	req := httptest.NewRequest("POST", "/v1/conversations", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(t))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// 鈹€鈹€ Tools 鈹€鈹€

// 鈹€鈹€ System Endpoints 鈹€鈹€

func TestIntegration_SystemHealth(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/v1/system/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIntegration_SystemTraces(t *testing.T) {
	router := testRouter(t)

	// S 瀹夊叏淇锛歵races/spans 浠呯鐞嗗憳鍙锛屽尶鍚嶈闂簲琚嫆缁?	req := httptest.NewRequest("GET", "/v1/system/traces", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if resp := w.Result(); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", resp.StatusCode)
	}

	// admin 鍙闂?	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
	os.Setenv("APP_SECRET", "test-app-secret-that-is-at-least-32-bytes-long!")
	adminAuth := auth.NewAuthenticator(config.Load().JWTSecret, time.Hour)
	adminToken, err := adminAuth.GenerateToken("admin-id", "admin@example.com", "admin", db.DefaultTenantID, auth.RolePermissions["admin"])
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	req2 := httptest.NewRequest("GET", "/v1/system/traces", nil)
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	// 鏃?DB 鐜涓?traces 鏌ヨ涓嶅彲鐢紱浠呴獙璇?authMW 涓嶅啀鎷掔粷绠＄悊鍛樿姹?	if resp := w2.Result(); resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("admin token should pass authMW, got 401")
	}

	// 鏅€?user 鏃犳潈闄?	userReq := httptest.NewRequest("GET", "/v1/system/traces", nil)
	userReq.Header.Set("Authorization", "Bearer "+testToken(t))
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, userReq)
	if resp := w3.Result(); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for regular user, got %d", resp.StatusCode)
	}
}

// 鈹€鈹€ Submit (requires auth) 鈹€鈹€

func TestIntegration_Submit_EmptyContent(t *testing.T) {
	router := testRouter(t)

	body := `{"content":"","session_id":"sess-1"}`
	req := httptest.NewRequest("POST", "/submit", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(t))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestIntegration_Submit_NoAuth(t *testing.T) {
	router := testRouter(t)

	body := `{"content":"hello","session_id":"sess-1"}`
	req := httptest.NewRequest("POST", "/submit", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// 鈹€鈹€ Protected Routes (no auth 鈫?should get 401) 鈹€鈹€

func TestIntegration_ProtectedRoutes_Unauthorized(t *testing.T) {
	router := testRouter(t)

	protectedPaths := []string{
		"GET /v1/conversations",
		"POST /v1/conversations",
		"GET /v1/conversations/test-id",
		"DELETE /v1/conversations/test-id",
		"GET /v1/billing/balance",
		"GET /v1/billing/history",
		"POST /v1/billing/recharge",
		"GET /api/editor/files",
		"GET /v1/admin/metrics",
		"GET /v1/admin/users",
	}

	for _, path := range protectedPaths {
		parts := strings.SplitN(path, " ", 2)
		method := parts[0]
		url := parts[1]

		req := httptest.NewRequest(method, url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for %s %s, got %d", method, url, resp.StatusCode)
		}
	}
}

// 鈹€鈹€ Editor Endpoints 鈹€鈹€

func TestIntegration_EditorListFiles(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/api/editor/files", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken(t))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIntegration_Editor_UserForbidden(t *testing.T) {
	// S 瀹夊叏淇锛氱紪杈戝櫒璇诲啓鍏变韩宸ヤ綔鍖猴紝鏅€?user 蹇呴』琚嫆缁濓紙403锛?	router := testRouter(t)

	req := httptest.NewRequest("GET", "/api/editor/read?path=secret.txt", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(t))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if resp := w.Result(); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for plain user read, got %d", resp.StatusCode)
	}

	req = httptest.NewRequest("POST", "/api/editor/write", bytes.NewReader([]byte(`{"path":"evil.txt","content":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(t))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if resp := w.Result(); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for plain user write, got %d", resp.StatusCode)
	}
}

func TestIntegration_EditorWriteRead(t *testing.T) {
	router := testRouter(t)

	// Write a file
	writeBody := `{"path":"test.txt","content":"hello world"}`
	req := httptest.NewRequest("POST", "/api/editor/write", bytes.NewReader([]byte(writeBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken(t))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatal("expected 200 for write")
	}

	// Read it back
	req = httptest.NewRequest("GET", "/api/editor/read?path=test.txt", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken(t))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// 鈹€鈹€ 404 for unknown routes 鈹€鈹€

func TestIntegration_NotFound(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/nonexistent-route", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestIntegration_EditorWriteRead_EscapeBlocked(t *testing.T) {
	router := testRouter(t)

	writeBody := "{\"path\":\"../evil.txt\",\"content\":\"bad\"}"
	req := httptest.NewRequest("POST", "/api/editor/write", bytes.NewReader([]byte(writeBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken(t))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for escape write, got %d", w.Result().StatusCode)
	}

	req = httptest.NewRequest("GET", "/api/editor/read?path=../evil.txt", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken(t))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for escape read, got %d", w.Result().StatusCode)
	}
}
