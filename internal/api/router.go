package api

import (
	"net/http"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/engine"
	"github.com/athenavi/minicc/internal/llm"
	"github.com/athenavi/minicc/internal/monitor"
	"github.com/athenavi/minicc/internal/tools"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var startTime = time.Now()

func NewRouter(cfg *config.Config, llmGateway *llm.Gateway, toolRegistry *tools.ToolRegistry, eventHub *broadcast.Hub) *chi.Mux {
	r := chi.NewRouter()

	// Rate limiter
	rateLimiter := NewRateLimiter(cfg.RateLimitRPM)
	rateLimiter.CleanupVisitors(5 * time.Minute)

	// Global middleware
	r.Use(RecoverMiddleware)
	r.Use(LoggingMiddleware)
	r.Use(SecurityHeadersMiddleware)
	r.Use(CORSMiddleware("*"))
	r.Use(MonitoringMiddleware)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	// Auth
	authenticator := auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration)
	authHandler := NewAuthHandler(cfg)

	// Engine
	eng := engine.New(llmGateway, toolRegistry)
	chatHandler := NewChatHandler(eng)

	// Public endpoints (rate limited)
	r.Group(func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Get("/health", handleHealth)
		r.Get("/ready", handleReadiness)
	})

	// SSE events endpoint (long-lived connection, no rate limit)
	r.Get("/events", func(w http.ResponseWriter, r *http.Request) {
		claims := auth.GetClaims(r.Context())
		subID := "anon"
		if claims != nil {
			subID = claims.UserID
		}
		handleSSE(w, r, eventHub, subID)
	})

	// Auth endpoints (rate limited)
	r.Route("/v1/auth", func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Post("/login", authHandler.Login)
		r.Post("/register", authHandler.Register)
		r.Post("/refresh", authHandler.Refresh)
	})

	// Protected API v1
	r.Route("/v1", func(r chi.Router) {
		r.Use(AuthMiddleware(authenticator))
		r.Use(rateLimiter.Middleware)

		r.Get("/status", handleStatus)
		r.Get("/profile", authHandler.Profile)

		// Chat
		r.Post("/chat", chatHandler.Chat)
		r.Get("/sessions", NotImplemented)
		r.Get("/sessions/{id}", NotImplemented)

		// Tools
		r.Get("/tools", NotImplemented)
		r.Post("/tools/execute", NotImplemented)

		// Metrics
		r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
			OK(w, monitor.Snapshot())
		})

		// LLM metrics
		r.Get("/llm/metrics", func(w http.ResponseWriter, r *http.Request) {
			OK(w, llmGateway.Metrics())
		})
		r.Get("/llm/cache", func(w http.ResponseWriter, r *http.Request) {
			OK(w, llmGateway.CacheStats())
		})

		// Admin
		r.Group(func(r chi.Router) {
			r.Use(RequirePermission(auth.PermAdminRead))
			r.Get("/admin/metrics", func(w http.ResponseWriter, r *http.Request) {
				OK(w, monitor.Snapshot())
			})
			r.Get("/admin/users", NotImplemented)
		})
	})

	return r
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    map[string]string{"status": "ok"},
	})
}

func handleReadiness(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    map[string]string{"status": "ready"},
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"version": "2.0.0",
			"uptime":  time.Since(startTime).String(),
			"metrics": monitor.Snapshot(),
		},
	})
}

func handleSSE(w http.ResponseWriter, r *http.Request, hub *broadcast.Hub, subID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		InternalError(w, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := hub.Subscribe(subID)
	defer hub.Unsubscribe(subID)

	// Send initial connected event
	w.Write([]byte(broadcast.FormatSSE(broadcast.Event{Type: "connected", Data: map[string]string{"id": subID}})))
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			w.Write([]byte(broadcast.FormatSSE(event)))
			flusher.Flush()
		case <-time.After(15 * time.Second):
			// Keep-alive
			w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}

func NotImplemented(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusNotImplemented, APIResponse{
		Success: false,
		Error:   "not implemented yet",
	})
}
