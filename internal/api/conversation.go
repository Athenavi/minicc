package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/id"
	"github.com/athenavi/minicc/internal/session"
)

// ConversationHandler handles CRUD for chat conversations (sessions).
type ConversationHandler struct {
	authenticator *auth.Authenticator
	sessionMgr    *session.Manager
}

func NewConversationHandler(a *auth.Authenticator, sm *session.Manager) *ConversationHandler {
	return &ConversationHandler{authenticator: a, sessionMgr: sm}
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

// List returns sessions for the current user (or empty for guests).
func (h *ConversationHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		OK(w, []Conversation{})
		return
	}

	sessions, err := h.sessionMgr.ListSessions(r.Context(), claims.UserID)
	if err != nil {
		// Fallback: return empty list
		OK(w, []Conversation{})
		return
	}

	convs := make([]Conversation, 0, len(sessions))
	for _, s := range sessions {
		convs = append(convs, Conversation{
			ID:        s.ID,
			Title:     s.Title,
			CreatedAt: s.CreatedAt,
			UpdatedAt: s.UpdatedAt,
		})
	}
	OK(w, convs)
}

// Get returns a single session with its messages (with optional pagination).
func (h *ConversationHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		Unauthorized(w, "authentication required")
		return
	}

	sess, err := h.sessionMgr.GetSession(r.Context(), id)
	if err != nil {
		NotFound(w, "conversation not found")
		return
	}

	if sess.UserID != claims.UserID {
		Forbidden(w, "access denied")
		return
	}

	// 分页参数：默认返回全部（limit=0）
	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	msgs, err := h.sessionMgr.GetMessages(r.Context(), id, limit)
	if err != nil {
		msgs = nil
	}

	conv := Conversation{
		ID:        sess.ID,
		Title:     sess.Title,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
		Messages:  make([]Message, 0),
	}
	for _, m := range msgs {
		conv.Messages = append(conv.Messages, Message{
			ID:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
		})
	}
	OK(w, conv)
}

// Create creates a new session. If authenticated, links to user account.
func (h *ConversationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.Title == "" {
		body.Title = "新对话"
	}

	claims := getAuthClaims(r, h.authenticator)
	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	sessionID := id.NextID()
	sess, err := h.sessionMgr.CreateSession(r.Context(), sessionID, userID, body.Title)
	if err != nil {
		InternalError(w, "create session: "+err.Error())
		return
	}

	OK(w, Conversation{
		ID:        sess.ID,
		Title:     sess.Title,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
	})
}

// SaveMessages inserts user + assistant messages into the database.
// Called from the /submit goroutine after streaming completes.
func (h *ConversationHandler) SaveMessages(ctx context.Context, sessionID, userID, userContent, assistantContent string) {
	h.sessionMgr.SaveMessages(ctx, sessionID, userID, userContent, assistantContent)
}

// Delete removes a session and its messages (CASCADE).
func (h *ConversationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		Unauthorized(w, "authentication required")
		return
	}

	sess, err := h.sessionMgr.GetSession(r.Context(), id)
	if err != nil {
		NotFound(w, "conversation not found")
		return
	}

	if sess.UserID != claims.UserID {
		Forbidden(w, "access denied")
		return
	}

	if err := h.sessionMgr.DeleteSession(r.Context(), id); err != nil {
		InternalError(w, "delete session: "+err.Error())
		return
	}

	OK(w, map[string]string{"status": "deleted"})
}


// Update updates a session title.
func (h *ConversationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}

	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		Unauthorized(w, "authentication required")
		return
	}

	sess, err := h.sessionMgr.GetSession(r.Context(), id)
	if err != nil {
		NotFound(w, "conversation not found")
		return
	}

	if sess.UserID != claims.UserID {
		Forbidden(w, "access denied")
		return
	}

	if db.Pool == nil {
		OK(w, map[string]string{"status": "updated"})
		return
	}

	_, err = db.Pool.Exec(r.Context(),
		`UPDATE sessions SET title = $1, updated_at = NOW() WHERE id = $2`,
		body.Title, id)
	if err != nil {
		InternalError(w, "update session: "+err.Error())
		return
	}

	OK(w, map[string]string{"status": "updated"})
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

func userIDFromClaims(claims *auth.Claims) string {
	if claims == nil {
		return ""
	}
	return claims.UserID
}
