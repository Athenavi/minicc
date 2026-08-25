package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/id"
	"github.com/athenavi/chiron/internal/session"
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
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"` // S 淇锛氬伐鍏疯皟鐢ㄨ繃绋嬭惤搴擄紝鍒锋柊鍚庤繕鍘?
	Cursor    string      `json:"cursor,omitempty"`     // P 鎬ц兘淇锛氬垎椤垫父鏍囷紙鍔犺浇鏇存棭娑堟伅锛?
	HasMore   bool        `json:"has_more"`             // P 鎬ц兘淇锛氭槸鍚﹁繕鏈夋洿鏃╃殑娑堟伅
}

// Message is a single chat message returned to the frontend.
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	ToolCalls string    `json:"tool_calls,omitempty"` // assistant 娑堟伅鐨?OpenAI 鏍煎紡 tool_calls锛圫 淇锛?
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

// List returns sessions for the current user.
func (h *ConversationHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
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

	claims := auth.GetClaims(r.Context())

	sess, err := h.sessionMgr.GetSession(r.Context(), id)
	if err != nil {
		NotFound(w, "conversation not found")
		return
	}

	if sess.UserID != claims.UserID {
		Forbidden(w, "access denied")
		return
	}

	// 鍒嗛〉鍙傛暟锛氶粯璁よ繑鍥炴渶杩?50 鏉★紙P 鎬ц兘淇锛氶灞忎笉鍔犺浇鍏ㄩ噺锛屼笂婊氬姞杞芥洿鏃╋級
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	before := r.URL.Query().Get("before")

	// P 鎬ц兘锛歮essages 涓?tool_calls 涓や釜鏌ヨ骞惰锛坧gxpool 骞跺彂瀹夊叏锛?
	// 浣跨敤 select 纭繚 context 鍙栨秷鏃朵笉娉勬紡 goroutine
	type pageResult struct {
		page session.MessagePage
		err  error
	}
	pageCh := make(chan pageResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("conversation message fetch panic", "session", id, "panic", r)
				pageCh <- pageResult{err: fmt.Errorf("panic: %v", r)}
			}
		}()
		p, e := h.sessionMgr.GetMessagesPage(r.Context(), id, limit, before)
		pageCh <- pageResult{p, e}
	}()
	toolCalls, _ := h.sessionMgr.GetToolCallsPage(r.Context(), id, limit, before)

	// Wait for messages or context cancellation
	var pr pageResult
	select {
	case pr = <-pageCh:
	case <-r.Context().Done():
		// Context cancelled; return what we have from toolCalls
		pr = pageResult{err: r.Context().Err()}
		slog.Warn("conversation fetch context cancelled", "session", id)
	}
	page := pr.page
	err = pr.err
	if err != nil {
		page = session.MessagePage{} // zero value, safe to access fields
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
			ToolCalls: m.ToolCalls, // S 淇锛歛ssistant 娑堟伅鐨?tool_calls 闅忚鎯呰繑鍥?
			CreatedAt: m.CreatedAt,
		})
	}
	// S 淇锛氬伐鍏疯皟鐢ㄨ繃绋嬭惤搴?鈥?闅忎細璇濊鎯呰繑鍥烇紝鍓嶇鍒锋柊鍚庤繕鍘熷伐鍏峰崱鐗?
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
		body.Title = "鏂板璇?
	}

	claims := auth.GetClaims(r.Context())
	userID := claims.UserID

	sessionID, err := id.UUID()
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "generate id failed")
		return
	}
	sess, err := h.sessionMgr.CreateSession(r.Context(), sessionID, userID, body.Title)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create session failed")
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

	claims := auth.GetClaims(r.Context())

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
		logAndRespond(w, err, http.StatusInternalServerError, "delete session failed")
		return
	}

	OK(w, map[string]string{"status": "deleted"})
}


// Update updates a session's title and/or pinned flag (session menu: 閲嶅懡鍚?缃《).
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

	claims := auth.GetClaims(r.Context())

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
		logAndRespond(w, err, http.StatusInternalServerError, "update session failed")
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

func userIDFromClaims(claims *auth.Claims) string {
	if claims == nil {
		return ""
	}
	return claims.UserID
}
