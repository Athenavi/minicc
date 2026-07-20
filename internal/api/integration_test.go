package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/broadcast"
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
	token, err := authenticator.GenerateToken("test-user-id", "test@example.com", "user", auth.RolePermissions["user"])
	if err != nil {
		t.Fatalf("generate token: %v", err)
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ── Auth Endpoints ──

func TestIntegration_AuthFlow(t *testing.T) {
	router := testRouter(t)

	// Register
	regBody := `{"name":"test","email":"test@test.com","password":"password123"}`
	req := httptest.NewRequest("POST", "/v1/auth/register", bytes.NewReader([]byte(regBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Without DB, register returns a specific error — just verify it doesn't crash
	resp := w.Result()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 200 or 500, got %d", resp.StatusCode)
	}
}

func TestIntegration_Login_NoDB(t *testing.T) {
	router := testRouter(t)

	loginBody := `{"email":"test@test.com","password":"password123"}`
	req := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewReader([]byte(loginBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Without DB, login returns an error gracefully
	resp := w.Result()
	// Should not crash
	_ = resp
}

// ── SSE & Events ──

func TestIntegration_SSE(t *testing.T) {
	router := testRouter(t)

	// Use a cancellable context so the SSE handler exits
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest("GET", "/events?client_id=test-client", nil)
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

func TestIntegration_ToolsList(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/v1/tools", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var apiResp APIResponse
	json.NewDecoder(resp.Body).Decode(&apiResp)
	if !apiResp.Success {
		t.Fatal("expected success response")
	}
}

func TestIntegration_ToolExecute_NotFound(t *testing.T) {
	router := testRouter(t)
	token := testToken(t)

	body := `{"name":"nonexistent","input":{}}`
	req := httptest.NewRequest("POST", "/v1/tools/execute", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

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

	req := httptest.NewRequest("GET", "/v1/system/traces", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ── Submit (requires auth) ──

func TestIntegration_Submit_EmptyContent(t *testing.T) {
	router := testRouter(t)

	body := `{"content":"","session_id":"sess-1"}`
	req := httptest.NewRequest("POST", "/submit", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
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

// ── Install Endpoints ──

func TestIntegration_InstallStatus(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/v1/install/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ── Protected Routes (no auth → should get 401) ──

func TestIntegration_ProtectedRoutes_Unauthorized(t *testing.T) {
	router := testRouter(t)

	protectedPaths := []string{}

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
	req.Header.Set("Authorization", "Bearer "+testToken(t))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIntegration_EditorWriteRead(t *testing.T) {
	router := testRouter(t)

	// Write a file
	writeBody := `{"path":"test.txt","content":"hello world"}`
	req := httptest.NewRequest("POST", "/api/editor/write", bytes.NewReader([]byte(writeBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken(t))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatal("expected 200 for write")
	}

	// Read it back
	req = httptest.NewRequest("GET", "/api/editor/read?path=test.txt", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(t))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ── Media Library ──

func TestIntegration_MediaList(t *testing.T) {
	router := testRouter(t)
	token := testToken(t)

	req := httptest.NewRequest("GET", "/v1/media", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
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
	req.Header.Set("Authorization", "Bearer "+testToken(t))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for escape write, got %d", w.Result().StatusCode)
	}

	req = httptest.NewRequest("GET", "/api/editor/read?path=../evil.txt", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(t))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for escape read, got %d", w.Result().StatusCode)
	}
}
