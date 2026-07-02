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
	"github.com/athenavi/minicc/internal/agent"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/llm"
	"github.com/athenavi/minicc/internal/session"
	"github.com/athenavi/minicc/internal/tools"
	"github.com/go-chi/chi/v5"
)

// testRouter builds a fully wired router for integration testing.
// Uses minimal config with local-only storage and no external dependencies.
func testRouter(t *testing.T) *chi.Mux {
	t.Helper()

	// Config (minimal — no DB, no Redis)
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
	cfg := config.Load()

	// LLM Gateway (no providers — chat will error gracefully)
	llmGateway := llm.NewGateway(5 * time.Minute)

	// Tool Registry
	toolRegistry := tools.NewToolRegistry()
	tools.RegisterCommonTools(toolRegistry, t.TempDir())
	tools.RegisterShellTools(toolRegistry, nil)
	tools.RegisterWebTools(toolRegistry)
	tools.RegisterSearchTools(toolRegistry, t.TempDir())

	// Event Hub
	eventHub := broadcast.NewHub(nil)

	// Agent Registry
	agentRegistry := agent.NewRegistry()
	agent.RegisterDefaults(agentRegistry)
	agentSessionMgr := agent.NewSessionManager()
	agent.RegisterTools(toolRegistry, agentRegistry, agentSessionMgr)

	// Session Manager
	sessionMgr := session.NewManager()

	// LLM Rate Limiter (local-only)
	llmRateLimiter := llm.NewRateLimiter(nil, 1000, 10000, 100000)

	return NewRouter(cfg, llmGateway, toolRegistry, eventHub, agentRegistry, sessionMgr, llmRateLimiter)
}

// ── Health & Readiness ────────────────────────────────────────────────────

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

// ── Auth Endpoints ────────────────────────────────────────────────────────

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

// ── SSE & Events ──────────────────────────────────────────────────────────

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

// ── Conversations ─────────────────────────────────────────────────────────

func TestIntegration_Conversations_NoAuth(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/v1/conversations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	// Without auth, should return empty list (not crash)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIntegration_CreateConversation(t *testing.T) {
	router := testRouter(t)

	body := `{"id":"test-sess-1","title":"Test Chat"}`
	req := httptest.NewRequest("POST", "/v1/conversations", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ── Tools ─────────────────────────────────────────────────────────────────

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

	body := `{"name":"nonexistent","input":{}}`
	req := httptest.NewRequest("POST", "/v1/tools/execute", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

// ── System Endpoints ──────────────────────────────────────────────────────

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

// ── Submit (Legacy Chat) ──────────────────────────────────────────────────

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

func TestIntegration_Submit_Valid(t *testing.T) {
	router := testRouter(t)

	body := `{"content":"hello","session_id":"sess-1"}`
	req := httptest.NewRequest("POST", "/submit", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	// Returns 202 Accepted and processes in background
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
}

// ── Install Endpoints ─────────────────────────────────────────────────────

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

// ── Protected Routes (no auth → should get 401) ──────────────────────────

func TestIntegration_ProtectedRoutes_Unauthorized(t *testing.T) {
	router := testRouter(t)

	protectedPaths := []string{
		"GET /v1/status",
		"GET /v1/profile",
		"POST /v1/chat",
		"GET /v1/tasks",
		"GET /v1/metrics",
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

// ── Editor Endpoints ──────────────────────────────────────────────────────

func TestIntegration_EditorListFiles(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/api/editor/files", nil)
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
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatal("expected 200 for write")
	}

	// Read it back
	req = httptest.NewRequest("GET", "/api/editor/read?path=test.txt", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ── Media Library ─────────────────────────────────────────────────────────

func TestIntegration_MediaList(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/v1/media", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ── LLM Metrics (unauthorized) ────────────────────────────────────────────

func TestIntegration_LLMMetrics_Unauthorized(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/v1/llm/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// ── Admin Endpoints (unauthorized) ────────────────────────────────────────

func TestIntegration_Admin_Unauthorized(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/v1/admin/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// ── 404 for unknown routes ────────────────────────────────────────────────

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
