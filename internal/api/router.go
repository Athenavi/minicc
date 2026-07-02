package api

import (
	"context"
	"log/slog"
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
	r.Use(CORSMiddleware(cfg.CORSOrigins))
	r.Use(MonitoringMiddleware)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	// Auth
	authenticator := auth.NewAuthenticator(cfg.JWTSecret, cfg.JWTExpiration)
	authHandler := NewAuthHandler(cfg)

	// Install
	installHandler := NewInstallHandler(cfg)

	// Engine
	eng := engine.New(llmGateway, toolRegistry)
	chatHandler := NewChatHandler(eng)

	// Public endpoints (rate limited)
	r.Group(func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Get("/health", handleHealth)
		r.Get("/ready", handleReadiness)
	})

	// Legacy API endpoints (used by frontend chat)
	r.Post("/submit", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Content   string `json:"content"`
			SessionID string `json:"session_id"`
		}
		if err := DecodeJSON(w, r, &body); err != nil {
			BadRequest(w, "invalid request")
			return
		}
		if body.Content == "" {
			BadRequest(w, "content is required")
			return
		}

		// Return 202 Accepted — processing continues in background
		Accepted(w, map[string]string{"status": "accepted", "session_id": body.SessionID})

		// Process in background: call LLM and stream tokens via SSE
		go func(content, sessionID string) {
			req := &llm.Request{
				Messages: []llm.Message{
					{Role: "system", Content: "You are MiniCC V2, an AI coding assistant. Respond concisely and helpfully."},
					{Role: "user", Content: content},
				},
				MaxTokens:   4096,
				Temperature: 0.7,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			resp, err := llmGateway.ChatStream(ctx, req, func(chunk string) {
				eventHub.Publish(broadcast.Event{Type: "text", Data: map[string]string{"content": chunk}})
			})

			if err != nil {
				slog.Error("llm stream failed", "error", err)
				eventHub.Publish(broadcast.Event{Type: "error", Data: map[string]string{"error": err.Error()}})
				eventHub.Publish(broadcast.Event{Type: "turn_done", Data: map[string]string{"session_id": sessionID}})
				return
			}

			_ = resp // usage info available if needed
			eventHub.Publish(broadcast.Event{Type: "turn_done", Data: map[string]string{"session_id": sessionID}})
		}(body.Content, body.SessionID)
	})
	r.Post("/cancel", handleCancel)
	r.Post("/approve", handleApprove)
	r.Post("/mode", handleMode)

	// SSE events endpoint (long-lived connection, no rate limit)
	r.Get("/events", func(w http.ResponseWriter, r *http.Request) {
		subID := r.URL.Query().Get("client_id")
		if subID == "" {
			subID = "anon"
		}
		handleSSE(w, r, eventHub, subID)
	})

	// Auth endpoints (rate limited)
	r.Route("/v1/auth", func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Post("/login", authHandler.Login)
		r.Post("/register", authHandler.Register)
		r.Post("/refresh", authHandler.Refresh)
		r.Post("/logout", authHandler.Logout)
	})

	// Install endpoints (no auth, rate limited)
	r.Route("/v1/install", func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Get("/status", installHandler.Status)
		r.Post("/setup", installHandler.Setup)
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

func handleCancel(w http.ResponseWriter, r *http.Request) {
	OK(w, map[string]string{"status": "cancelled"})
}

func handleApprove(w http.ResponseWriter, r *http.Request) {
	OK(w, map[string]string{"status": "approved"})
}

func handleMode(w http.ResponseWriter, r *http.Request) {
	OK(w, map[string]string{"status": "ok", "mode": "ask"})
}
