package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// ConversationHandler handles CRUD for chat conversations (sessions).
type ConversationHandler struct{}

func NewConversationHandler() *ConversationHandler {
	return &ConversationHandler{}
}

// Conversation is a chat session returned to the frontend.
type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages,omitempty"`
}

// Message is a single chat message returned to the frontend.
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// List returns all sessions ordered by most recent.
func (h *ConversationHandler) List(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		OK(w, []Conversation{})
		return
	}

	rows, err := db.Pool.Query(r.Context(),
		`SELECT id, COALESCE(title, ''), created_at, updated_at
		 FROM sessions
		 ORDER BY updated_at DESC
		 LIMIT 100`)
	if err != nil {
		InternalError(w, "query sessions: "+err.Error())
		return
	}
	defer rows.Close()

	convs := make([]Conversation, 0)
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			continue
		}
		convs = append(convs, c)
	}

	OK(w, convs)
}

// Get returns a single session with all its messages.
func (h *ConversationHandler) Get(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	// Get session
	var conv Conversation
	err := db.Pool.QueryRow(r.Context(),
		`SELECT id, COALESCE(title, ''), created_at, updated_at FROM sessions WHERE id = $1`, id).
		Scan(&conv.ID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt)
	if err == pgx.ErrNoRows {
		NotFound(w, "conversation not found")
		return
	} else if err != nil {
		InternalError(w, "query session: "+err.Error())
		return
	}

	// Get messages
	rows, err := db.Pool.Query(r.Context(),
		`SELECT id, role, content, created_at
		 FROM messages
		 WHERE session_id = $1
		 ORDER BY created_at ASC`, id)
	if err != nil {
		InternalError(w, "query messages: "+err.Error())
		return
	}
	defer rows.Close()

	conv.Messages = make([]Message, 0)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			continue
		}
		conv.Messages = append(conv.Messages, m)
	}

	OK(w, conv)
}

// Create creates a new session.
func (h *ConversationHandler) Create(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		OK(w, map[string]string{"id": "", "note": "database not available, session not persisted"})
		return
	}

	var body struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.ID == "" {
		BadRequest(w, "id is required")
		return
	}
	if body.Title == "" {
		body.Title = "New Chat"
	}

	_, err := db.Pool.Exec(r.Context(),
		`INSERT INTO sessions (id, title, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, updated_at = NOW()`,
		body.ID, body.Title)
	if err != nil {
		InternalError(w, "create session: "+err.Error())
		return
	}

	OK(w, map[string]string{"id": body.ID})
}

// SaveMessages inserts user + assistant messages into the database.
// Called from the /submit goroutine after streaming completes.
func SaveMessages(ctx context.Context, sessionID, userID, userContent, assistantContent string) {
	if db.Pool == nil {
		return
	}

	// Ensure session exists (create if not)
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, title, created_at, updated_at)
		 VALUES ($1, $2, '', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET updated_at = NOW()`,
		sessionID, nullableStr(userID))
	if err != nil {
		return
	}

	// Save user message
	if userContent != "" {
		userID := genID()
		db.Pool.Exec(ctx,
			`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, 'user', $3, NOW())`,
			userID, sessionID, userContent)
	}

	// Save assistant message
	if assistantContent != "" {
		assistantID := genID()
		db.Pool.Exec(ctx,
			`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, 'assistant', $3, NOW())`,
			assistantID, sessionID, assistantContent)
	}

	// Update session title based on first user message
	db.Pool.Exec(ctx,
		`UPDATE sessions SET title = LEFT($1, 255), updated_at = NOW()
		 WHERE id = $2 AND (title = '' OR title IS NULL)`,
		truncateTitle(userContent), sessionID)
}

// Delete removes a session and its messages (CASCADE).
func (h *ConversationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	_, err := db.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE id = $1`, id)
	if err != nil {
		InternalError(w, "delete session: "+err.Error())
		return
	}

	OK(w, map[string]string{"status": "deleted"})
}

func genID() string {
	// Simple nanosecond-based ID for DB records
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func truncateTitle(s string) string {
	// Take first line or first 120 chars
	idx := strings.Index(s, "\n")
	if idx > 0 {
		s = s[:idx]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	if s == "" {
		s = "New Chat"
	}
	return s
}

func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// getAuthClaims extracts JWT claims optionally — no error if missing.
func getAuthClaims(r *http.Request, a *auth.Authenticator) *auth.Claims {
	tokenStr := ""
	if c, err := r.Cookie("minicc_token"); err == nil && c.Value != "" {
		tokenStr = c.Value
	}
	if tokenStr == "" {
		if ah := r.Header.Get("Authorization"); ah != "" {
			if strings.HasPrefix(ah, "Bearer ") {
				tokenStr = strings.TrimPrefix(ah, "Bearer ")
			}
		}
	}
	if tokenStr == "" {
		return nil
	}
	claims, err := a.ValidateToken(tokenStr)
	if err != nil {
		return nil
	}
	return claims
}
