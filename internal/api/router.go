package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/agent"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/engine"
	"github.com/athenavi/minicc/internal/llm"
	"github.com/athenavi/minicc/internal/monitor"
	"github.com/athenavi/minicc/internal/session"
	"github.com/athenavi/minicc/internal/tools"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var startTime = time.Now()

func NewRouter(cfg *config.Config, llmGateway *llm.Gateway, toolRegistry *tools.ToolRegistry, eventHub *broadcast.Hub, agentRegistry *agent.Registry, sessionMgr *session.Manager, llmRateLimiter *llm.RateLimiter) *chi.Mux {
	r := chi.NewRouter()

	// Custom 404 handler (returns JSON, not empty body)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusNotFound, APIResponse{
			Success: false,
			Error:   "not found",
		})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusMethodNotAllowed, APIResponse{
			Success: false,
			Error:   "method not allowed",
		})
	})

	// Rate limiter
	rateLimiter := NewRateLimiter(cfg.RateLimitRPM)
	rateLimiter.CleanupVisitors(5 * time.Minute)

	// Global middleware
	r.Use(RecoverMiddleware)
	r.Use(TracingMiddleware)
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
	eng.SetHub(eventHub)
	orchestrator := engine.NewTurnOrchestrator(llmGateway, toolRegistry, eventHub)
	chatHandler := NewChatHandler(eng)

	// Public endpoints (rate limited)
	r.Group(func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Get("/health", handleHealth)
		r.Get("/ready", handleReadiness)
	})

	// Legacy API endpoints (used by frontend chat)
	modeStore := NewModeStore()
	permMgr := NewPermissionManager()
	modeHandler := NewModeHandler(modeStore, permMgr)

	// Agent execution semaphore — max 20 concurrent agent runs
	agentSem := make(chan struct{}, 20)

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

		// Process in background using the TurnOrchestrator (multi-turn agent loop)
		// Acquire semaphore slot (max 20 concurrent runs), drop if full
		select {
		case agentSem <- struct{}{}:
		default:
			slog.Warn("agent semaphore full, dropping submit", "session", body.SessionID)
			return
		}
		go func(content, sessionID, userID string) {
			defer func() { <-agentSem }()
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()

			messages := []llm.Message{
				{Role: "user", Content: content},
			}

			toolDefs := engine.BuildToolDefs(toolRegistry)
			systemPrompt := llm.DeterministicSystemPrompt()

			finalContent, _, err := orchestrator.Execute(ctx, sessionID, messages, systemPrompt, toolDefs)

			// Fallback: if orchestrator returns empty content, do a simple text-only LLM call
			if err != nil || finalContent == "" {
				slog.Warn("orchestrator returned empty, falling back to text-only LLM", "error", err)
				fallbackReq := &llm.Request{
					Messages: []llm.Message{
						{Role: "system", Content: systemPrompt},
						{Role: "user", Content: content},
					},
					MaxTokens:   4096,
					Temperature: 0.7,
					Stream:      true,
				}
				var fbContent string
				_, fbErr := llmGateway.ChatStream(ctx, fallbackReq,
					func(chunk string) {
						fbContent += chunk
						eventHub.Publish(broadcast.Event{Type: "text", Data: map[string]string{"content": chunk}})
					},
					func(tc llm.ToolCall) {},
				)
				if fbErr == nil && fbContent != "" {
					finalContent = fbContent
				} else if finalContent == "" {
					finalContent = "I encountered an error processing your request. Please try again."
					eventHub.Publish(broadcast.Event{Type: "text", Data: map[string]string{"content": finalContent}})
				}
			}

			eventHub.Publish(broadcast.Event{Type: "turn_done", Data: map[string]string{"session_id": sessionID}})

			// Persist to DB if authenticated
			if userID != "" {
				sessionMgr.SaveMessages(ctx, sessionID, userID, content, finalContent)
			}
		}(body.Content, body.SessionID, userID)
	})
	r.Post("/cancel", handleCancel)
	r.Post("/mode", modeHandler.SetMode)
	r.Get("/mode", modeHandler.GetMode)
	r.Post("/approve", modeHandler.ApprovePermission)
	r.Post("/reject", modeHandler.RejectPermission)

	// SSE events endpoint (long-lived connection, no rate limit)
	r.Get("/events", SSEHandler(eventHub))

	// WebSocket endpoint (for real-time bidirectional communication)
	wsHub := NewWebSocketHub()
	r.Get("/ws/{sessionId}", WebSocketHandler(wsHub))

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

	// Editor endpoints (public, rate limited)
	editorHandler := NewEditorHandler(cfg.StorageRoot)
	r.Route("/api/editor", func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Get("/files", editorHandler.ListFiles)
		r.Get("/read", editorHandler.ReadFile)
		r.Post("/write", editorHandler.WriteFile)
	})

	// Conversation endpoints (auth checked manually inside handler)
	conversationHandler := NewConversationHandler(authenticator, sessionMgr)
	r.Route("/v1/conversations", func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Get("/", conversationHandler.List)
		r.Post("/", conversationHandler.Create)
		r.Get("/{id}", conversationHandler.Get)
		r.Delete("/{id}", conversationHandler.Delete)
	})

	// Tool endpoints (auth checked manually inside handler)
	toolHandler := NewToolHandler(toolRegistry)
	r.Route("/v1/tools", func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Get("/", toolHandler.ListTools)
		r.Post("/execute", toolHandler.ExecuteTool)
	})

	// System endpoints (public)
	systemHandler := NewSystemHandler()
	r.Route("/v1/system", func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Get("/health", systemHandler.HealthScores)
		r.Get("/traces", systemHandler.Traces)
	})

	// Workflow endpoints (public)
	// Media library endpoints
	mediaHandler := NewMediaHandler(cfg.StorageRoot)
	r.Route("/v1/media", func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Get("/", mediaHandler.List)
		r.Post("/", mediaHandler.Create)
		r.Post("/upload", mediaHandler.Upload)
		r.Delete("/", mediaHandler.Delete)
	})

	// Agent list endpoint
	r.Route("/v1/agents", func(r chi.Router) {
		r.Use(rateLimiter.Middleware)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			OK(w, agentRegistry.List())
		})
		// Agent dispatch endpoint
		agentRuntime := agent.NewAgentRuntime(llmGateway, toolRegistry, eventHub)
		r.Post("/dispatch", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				SessionID string `json:"session_id"`
				AgentType string `json:"agent_type"`
				Task      string `json:"task"`
				UserID    string `json:"user_id"`
			}
			if err := DecodeJSON(w, r, &body); err != nil {
				BadRequest(w, "invalid request")
				return
			}
			if body.Task == "" || body.AgentType == "" {
				BadRequest(w, "task and agent_type required")
				return
			}
			Accepted(w, map[string]string{"status": "dispatched", "agent_type": body.AgentType})
			// Execute in background
			go agentRuntime.Dispatch(context.Background(), body.SessionID, body.UserID, body.AgentType, body.Task)
		})
	})

	// Protected API v1
	r.Route("/v1", func(r chi.Router) {
		r.Use(AuthMiddleware(authenticator))
		r.Use(LLMRateLimitMiddleware(llmRateLimiter))
		r.Use(rateLimiter.Middleware)

		r.Get("/status", handleStatus)
		r.Get("/profile", authHandler.Profile)

		// Chat
		r.Post("/chat", chatHandler.Chat)

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
		adminHandler := NewAdminHandler(authenticator)
		r.Group(func(r chi.Router) {
			r.Use(RequirePermission(auth.PermAdminRead))
			adminHandler.RegisterRoutes(r)
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

func NotImplemented(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusNotImplemented, APIResponse{
		Success: false,
		Error:   "not implemented yet",
	})
}

func handleCancel(w http.ResponseWriter, r *http.Request) {
	OK(w, map[string]string{"status": "cancelled"})
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

// ── Enterprise list handlers ──

// handleEnterpriseList returns a handler that lists rows from an enterprise table.
func handleEnterpriseList(table string, columns string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db.Pool == nil || table == "" {
			OK(w, []map[string]interface{}{})
			return
		}

		// For brain overview, return counts from all tables
		if table == "" {
			counts := map[string]int{}
			tables := []string{"enterprise_tasks", "wiki_pages", "okrs", "meeting_notes", "support_tickets", "kb_articles", "marketing_campaigns"}
			for _, t := range tables {
				var count int
				db.Pool.QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM %s", t)).Scan(&count)
				counts[t] = count
			}
			OK(w, counts)
			return
		}

		cols := "id"
		if columns != "" {
			cols = "id, " + columns
		}
		query := fmt.Sprintf("SELECT %s FROM %s ORDER BY updated_at DESC NULLS LAST, created_at DESC LIMIT 50", cols, table)

		rows, err := db.Pool.Query(r.Context(), query)
		if err != nil {
			OK(w, []map[string]interface{}{})
			return
		}
		defer rows.Close()

		results := make([]map[string]interface{}, 0)
		colNames := rows.FieldDescriptions()

		for rows.Next() {
			values, err := rows.Values()
			if err != nil {
				continue
			}
			row := make(map[string]interface{})
			for i, val := range values {
				if i < len(colNames) {
					row[string(colNames[i].Name)] = val
				}
			}
			results = append(results, row)
		}

		OK(w, results)
	}
}
