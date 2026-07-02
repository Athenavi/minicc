package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/db"
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

		// Optional auth: if logged in, persist to DB
		claims := getAuthClaims(r, authenticator)
		userID := ""
		if claims != nil {
			userID = claims.UserID
		}

		// Return 202 Accepted — processing continues in background
		Accepted(w, map[string]string{"status": "accepted", "session_id": body.SessionID})

		// Process in background: call LLM and stream tokens via SSE
		go func(content, sessionID, userID string) {
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

			var fullContent string
			resp, err := llmGateway.ChatStream(ctx, req, func(chunk string) {
				fullContent += chunk
				eventHub.Publish(broadcast.Event{Type: "text", Data: map[string]string{"content": chunk}})
			})

			if err != nil {
				slog.Error("llm stream failed", "error", err)
				eventHub.Publish(broadcast.Event{Type: "error", Data: map[string]string{"error": err.Error()}})
				eventHub.Publish(broadcast.Event{Type: "turn_done", Data: map[string]string{"session_id": sessionID}})
				return
			}

			_ = resp
			eventHub.Publish(broadcast.Event{Type: "turn_done", Data: map[string]string{"session_id": sessionID}})

			// Persist to DB if authenticated
			if userID != "" {
				SaveMessages(ctx, sessionID, userID, content, fullContent)
				// Detect and save task dispatch from LLM response
				if taskName, source := detectTaskDispatch(fullContent); taskName != "" {
					createAndPublishTask(ctx, sessionID, userID, taskName, source, eventHub)
				}
			}
		}(body.Content, body.SessionID, userID)
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

	// Conversation endpoints (auth checked manually inside handler)
	conversationHandler := NewConversationHandler(authenticator)
	r.Route("/v1/conversations", func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Get("/", conversationHandler.List)
		r.Post("/", conversationHandler.Create)
		r.Get("/{id}", conversationHandler.Get)
		r.Delete("/{id}", conversationHandler.Delete)
	})

	// Protected API v1
	r.Route("/v1", func(r chi.Router) {
		r.Use(AuthMiddleware(authenticator))
		r.Use(rateLimiter.Middleware)

		r.Get("/status", handleStatus)
		r.Get("/profile", authHandler.Profile)

		// Chat
		r.Post("/chat", chatHandler.Chat)

		// Tools
		r.Get("/tools", NotImplemented)
		r.Post("/tools/execute", NotImplemented)

		// Tasks (running/completed/failed)
		r.Get("/tasks", handleTasksList)

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

// ── Task dispatch detection (from LLM response text) ──

var dispatchPatterns = []string{
	"已分派至", "已分配至", "已派发至",
	"dispatched to", "assigned to",
}

// detectTaskDispatch scans LLM response for known dispatch keywords
// and returns (taskName, source) if found.
func detectTaskDispatch(response string) (string, string) {
	for _, pattern := range dispatchPatterns {
		idx := strings.Index(response, pattern)
		if idx < 0 {
			continue
		}
		// Extract text after the pattern (up to next newline or 80 chars)
		start := idx + len(pattern)
		if start >= len(response) {
			continue
		}
		end := start + 80
		if end > len(response) {
			end = len(response)
		}
		snippet := response[start:end]
		// Take up to newline or colon/comma
		termIdx := strings.IndexAny(snippet, "\n，,：:")
		if termIdx > 0 {
			snippet = snippet[:termIdx]
		}
		snippet = strings.TrimSpace(snippet)
		if snippet != "" {
			return snippet, pattern
		}
	}
	return "", ""
}

// createAndPublishTask inserts a task record and emits tool_dispatch SSE event.
func createAndPublishTask(ctx context.Context, sessionID, userID, taskName, source string, hub *broadcast.Hub) {
	if db.Pool == nil {
		return
	}

	payload := map[string]interface{}{
		"session_id": sessionID,
		"source":     source,
		"task":       taskName,
		"dispatch":   true,
	}
	payloadJSON, _ := json.Marshal(payload)

	taskID := genID()
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO tasks (id, user_id, type, status, payload, created_at, updated_at)
		 VALUES ($1, $2, 'tool', 'running', $3, NOW(), NOW())`,
		taskID, userID, string(payloadJSON))
	if err != nil {
		slog.Warn("create task failed", "error", err)
		return
	}

	// Emit tool_dispatch SSE event so the frontend can show it inline
	hub.Publish(broadcast.Event{
		Type: "tool_dispatch",
		Data: map[string]string{
			"tool_name":  taskName,
			"task_id":    taskID,
			"session_id": sessionID,
		},
	})

	slog.Info("task dispatched", "id", taskID, "name", taskName, "user", userID)
}

// ── Task list API ──

func handleTasksList(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		OK(w, []map[string]interface{}{})
		return
	}

	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, "not authenticated")
		return
	}

	rows, err := db.Pool.Query(r.Context(),
		`SELECT id, type, status, payload, error, created_at, updated_at
		 FROM tasks
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT 50`, claims.UserID)
	if err != nil {
		InternalError(w, "query tasks: "+err.Error())
		return
	}
	defer rows.Close()

	tasks := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, typ, status string
		var payload, error sql.NullString
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &typ, &status, &payload, &error, &createdAt, &updatedAt); err != nil {
			continue
		}

		task := map[string]interface{}{
			"id":         id,
			"type":       typ,
			"status":     status,
			"created_at": createdAt,
			"updated_at": updatedAt,
		}
		if payload.Valid {
			var p interface{}
			json.Unmarshal([]byte(payload.String), &p)
			task["payload"] = p
		} else {
			task["payload"] = map[string]interface{}{}
		}
		if error.Valid {
			task["error"] = error.String
		}
		tasks = append(tasks, task)
	}

	OK(w, tasks)
}
