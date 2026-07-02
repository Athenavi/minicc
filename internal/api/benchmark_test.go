package api

import (
	"bytes"
	"net/http/httptest"
	"os"
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

// benchRouter creates a test router for benchmarks.
func benchRouter(b *testing.B) *chi.Mux {
	b.Helper()
	os.Setenv("JWT_SECRET", "bench-secret-that-is-at-least-32-bytes!")
	cfg := config.Load()

	llmGateway := llm.NewGateway(5 * time.Minute)
	toolRegistry := tools.NewToolRegistry()
	tools.RegisterCommonTools(toolRegistry, b.TempDir())
	tools.RegisterShellTools(toolRegistry, nil)
	tools.RegisterWebTools(toolRegistry)
	tools.RegisterSearchTools(toolRegistry, b.TempDir())
	tools.RegisterRPATools(toolRegistry)

	eventHub := broadcast.NewHub(nil)
	agentRegistry := agent.NewRegistry()
	agent.RegisterDefaults(agentRegistry)
	agent.RegisterTools(toolRegistry, agentRegistry, agent.NewSessionManager())

	sessionMgr := session.NewManager()
	llmRateLimiter := llm.NewRateLimiter(nil, 10000, 100000, 1000000)

	return NewRouter(cfg, llmGateway, toolRegistry, eventHub, agentRegistry, sessionMgr, llmRateLimiter)
}

// BenchmarkRouter_Health measures HTTP router throughput for a simple health check.
func BenchmarkRouter_Health(b *testing.B) {
	router := benchRouter(b)
	req := httptest.NewRequest("GET", "/health", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkRouter_ToolsList measures HTTP handler for listing tools.
func BenchmarkRouter_ToolsList(b *testing.B) {
	router := benchRouter(b)
	req := httptest.NewRequest("GET", "/v1/tools", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkRouter_404 measures handling of non-existent routes.
func BenchmarkRouter_404(b *testing.B) {
	router := benchRouter(b)
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkRouter_Submit measures the legacy submit endpoint.
func BenchmarkRouter_Submit(b *testing.B) {
	router := benchRouter(b)
	body := []byte(`{"content":"hello world","session_id":"bench-session"}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/submit", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
	}
}

// BenchmarkRouter_CreateConversation measures session creation.
func BenchmarkRouter_CreateConversation(b *testing.B) {
	router := benchRouter(b)
	body := []byte(`{"id":"bench-sess","title":"Benchmark"}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/conversations", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
	}
}
