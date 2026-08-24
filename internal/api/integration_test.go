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

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/session"
)

// testRouter builds a fully wired router for integration testing.
// Uses minimal config with local-only storage and no external dependencies.
func testRouter(t *testing.T) http.Handler {
	t.Helper()

	// Config (minimal — no DB, no Redis)
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
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
	cfg := config.Load()
	authenticator := auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration)
	token, err := authenticator.GenerateToken("test-user-id", "test@example.com", "user", db.DefaultTenantID, auth.RolePermissions["user"])
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return token
}

// adminToken 返回 admin 角色的 JWT，用于验证需要管理端权限的路由。
func adminToken(t *testing.T) string {
	t.Helper()
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
	cfg := config.Load()
	authenticator := auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration)
	token, err := authenticator.GenerateToken("admin-id", "admin@example.com", "admin", db.DefaultTenantID, auth.RolePermissions["admin"])
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	return token
}

// ── Health & Readiness ──

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
	// 新契约(2026-08-22)：/ready 反映真实依赖状态；测试环境无 DB/Redis → 503 + deps down
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (deps missing), got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "redis\":\"down") || !strings.Contains(string(body), "postgres\":\"down") {
		t.Fatalf("expected deps down payload, got %s", string(body))
	}
}

// ── Auth Endpoints ──

// ── SSE & Events ──

func TestIntegration_SSE(t *testing.T) {
	// 该测试无数据库：sessionMgr 传 nil 跳过归属校验，聚焦流式行为本身。
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
	cfg := config.Load()
	eventHub := broadcast.NewHub(nil)
	router := NewGatewayRouter(cfg, nil, eventHub, nil, nil, nil, nil)

	// Use a cancellable context so the SSE handler exits
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// S1: /events now requires auth — Authorization header（S 安全修复：不再支持 ?token= 查询参数）
	req := httptest.NewRequest("GET", "/events?client_id=test-client&session_id=test-session", nil)
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

// ── Conversations ──

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

// ── Tools ──

// ── System Endpoints ──

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

	// S 安全修复：traces/spans 仅管理员可见，匿名访问应被拒绝
	req := httptest.NewRequest("GET", "/v1/system/traces", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if resp := w.Result(); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", resp.StatusCode)
	}

	// admin 可访问
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
	adminAuth := auth.NewAuthenticator(config.Load().JWTSecret, time.Hour)
	adminToken, err := adminAuth.GenerateToken("admin-id", "admin@example.com", "admin", db.DefaultTenantID, auth.RolePermissions["admin"])
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	req2 := httptest.NewRequest("GET", "/v1/system/traces", nil)
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	// 无 DB 环境下 traces 查询不可用；仅验证 authMW 不再拒绝管理员请求
	if resp := w2.Result(); resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("admin token should pass authMW, got 401")
	}

	// 普通 user 无权限
	userReq := httptest.NewRequest("GET", "/v1/system/traces", nil)
	userReq.Header.Set("Authorization", "Bearer "+testToken(t))
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, userReq)
	if resp := w3.Result(); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for regular user, got %d", resp.StatusCode)
	}
}

// ── Submit (requires auth) ──

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

// ── Protected Routes (no auth → should get 401) ──

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

// ── Editor Endpoints ──

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
	// S 安全修复：编辑器读写共享工作区，普通 user 必须被拒绝（403）
	router := testRouter(t)

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

// ── 404 for unknown routes ──

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
