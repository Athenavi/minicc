package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
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
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	Pinned    bool        `json:"pinned"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	Messages  []Message   `json:"messages,omitempty"`
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"` // S 修复：工具调用过程落库，刷新后还原
	Cursor    string      `json:"cursor,omitempty"`     // P 性能修复：分页游标（加载更早消息）
	HasMore   bool        `json:"has_more"`             // P 性能修复：是否还有更早的消息
}

// Message is a single chat message returned to the frontend.
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	ToolCalls string    `json:"tool_calls,omitempty"` // assistant 消息的 OpenAI 格式 tool_calls（S 修复）
	CreatedAt time.Time `json:"created_at"`
}

// ToolCall is a persisted tool invocation (input/output) for history rendering.
type ToolCall struct {
	ID        string    `json:"id"`
	ToolName  string    `json:"tool_name"`
	Input     string    `json:"input"`
	Output    string    `json:"output"`
	IsError   bool      `json:"is_error"`
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
			Pinned:    s.Pinned,
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

	// 分页参数：默认返回最近 50 条（P 性能修复：首屏不加载全量，上滚加载更早）
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	before := r.URL.Query().Get("before")

	// P 性能：messages 与 tool_calls 两个查询并行（pgxpool 并发安全）
	type pageResult struct {
		page session.MessagePage
		err  error
	}
	pageCh := make(chan pageResult, 1)
	go func() {
		p, e := h.sessionMgr.GetMessagesPage(r.Context(), id, limit, before)
		pageCh <- pageResult{p, e}
	}()
	toolCalls, _ := h.sessionMgr.GetToolCallsPage(r.Context(), id, limit, before)
	pr := <-pageCh
	page := pr.page
	err = pr.err
	if err != nil {
		page.Messages = nil
	}

	conv := Conversation{
		ID:        sess.ID,
		Title:     sess.Title,
		Pinned:    sess.Pinned,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
		Messages:  make([]Message, 0),
		Cursor:    page.Cursor,
		HasMore:   page.HasMore,
	}
	for _, m := range page.Messages {
		conv.Messages = append(conv.Messages, Message{
			ID:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			ToolCalls: m.ToolCalls, // S 修复：assistant 消息的 tool_calls 随详情返回
			CreatedAt: m.CreatedAt,
		})
	}
	// S 修复：工具调用过程落库 — 随会话详情返回，前端刷新后还原工具卡片
	for _, tc := range toolCalls {
		conv.ToolCalls = append(conv.ToolCalls, ToolCall{
			ID:        tc.ID,
			ToolName:  tc.ToolName,
			Input:     tc.Input,
			Output:    tc.Output,
			IsError:   tc.IsError,
			CreatedAt: tc.CreatedAt,
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

	sessionID := id.UUID()
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


// Update updates a session's title and/or pinned flag (session menu: 重命名/置顶).
func (h *ConversationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		BadRequest(w, "id is required")
		return
	}

	var body struct {
		Title  *string `json:"title"`
		Pinned *bool   `json:"pinned"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.Title == nil && body.Pinned == nil {
		BadRequest(w, "title or pinned is required")
		return
	}
	if body.Title != nil && strings.TrimSpace(*body.Title) == "" {
		BadRequest(w, "title is required")
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

	updated, err := h.sessionMgr.UpdateSession(r.Context(), id, body.Title, body.Pinned)
	if err != nil {
		InternalError(w, "update session: "+err.Error())
		return
	}

	OK(w, Conversation{
		ID:        updated.ID,
		Title:     updated.Title,
		Pinned:    updated.Pinned,
		CreatedAt: updated.CreatedAt,
		UpdatedAt: updated.UpdatedAt,
	})
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
