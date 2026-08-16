package api

import (
	"crypto/rand"
	"encoding/base32"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/model"
	"github.com/athenavi/minicc/internal/session"
)

// ShareHandler manages public conversation shares (rendered at /share/{id}
// without authentication, like chat.deepseek.com/share/{id}).
type ShareHandler struct {
	authenticator *auth.Authenticator
	sessionMgr    *session.Manager
}

func NewShareHandler(a *auth.Authenticator, sm *session.Manager) *ShareHandler {
	return &ShareHandler{authenticator: a, sessionMgr: sm}
}

// shareToken generates a random unguessable share id (80 bits entropy, base32
// lowercase, no padding — 16 chars). Shares must not be enumerable, unlike
// snowflake ids, because the id is the whole access control.
func shareToken() string {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("share: crypto/rand unavailable: " + err.Error())
	}
	return strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]))
}

// Create shares a conversation (POST /v1/conversations/{id}/share).
// The body lists the message ids to expose; only text messages are rendered.
// Idempotent: an active share for the session is returned as-is.
func (h *ShareHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var body struct {
		MessageIDs []string `json:"message_ids"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	messageIDs := dedupe(body.MessageIDs)
	if len(messageIDs) == 0 {
		BadRequest(w, "message_ids is required")
		return
	}
	if db.Pool == nil {
		InternalError(w, "database not available")
		return
	}

	// 消息归属校验：只允许分享本会话的消息
	rows, err := db.Pool.Query(r.Context(),
		`SELECT id FROM messages WHERE session_id = $1 AND id = ANY($2::text[])`, id, messageIDs)
	if err != nil {
		InternalError(w, "validate messages: "+err.Error())
		return
	}
	owned := make(map[string]bool)
	for rows.Next() {
		var mid string
		if rows.Scan(&mid) == nil {
			owned[mid] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		InternalError(w, "validate messages: "+err.Error())
		return
	}
	valid := make([]string, 0, len(messageIDs))
	for _, mid := range messageIDs {
		if owned[mid] {
			valid = append(valid, mid)
		}
	}
	if len(valid) == 0 {
		BadRequest(w, "message_ids does not match any message in this conversation")
		return
	}

	// 幂等：已有活跃分享直接返回
	existing, err := h.activeShare(r, id)
	if err != nil {
		InternalError(w, "query share: "+err.Error())
		return
	}
	if existing != nil {
		OK(w, map[string]interface{}{"share_id": existing.ID, "created_at": existing.CreatedAt})
		return
	}

	token := shareToken()
	_, err = db.Pool.Exec(r.Context(),
		`INSERT INTO conversation_shares (id, session_id, user_id, title, message_ids, created_at)
		 VALUES ($1, $2, $3::uuid, $4, $5, $6)`,
		token, id, claims.UserID, sess.Title, valid, time.Now())
	if err != nil {
		InternalError(w, "create share: "+err.Error())
		return
	}

	OK(w, map[string]interface{}{"share_id": token, "created_at": time.Now()})
}

// GetActive returns the active share for a conversation (GET /v1/conversations/{id}/share).
func (h *ShareHandler) GetActive(w http.ResponseWriter, r *http.Request) {
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

	share, err := h.activeShare(r, id)
	if err != nil {
		InternalError(w, "query share: "+err.Error())
		return
	}
	if share == nil {
		NotFound(w, "no active share")
		return
	}
	OK(w, map[string]interface{}{"share_id": share.ID, "created_at": share.CreatedAt})
}

// Revoke cancels the active share (DELETE /v1/conversations/{id}/share).
// The public link stops resolving afterwards.
func (h *ShareHandler) Revoke(w http.ResponseWriter, r *http.Request) {
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

	if db.Pool != nil {
		if _, err := db.Pool.Exec(r.Context(),
			`UPDATE conversation_shares SET revoked_at = NOW() WHERE session_id = $1 AND revoked_at IS NULL`, id); err != nil {
			InternalError(w, "revoke share: "+err.Error())
			return
		}
	}
	OK(w, map[string]string{"status": "revoked"})
}

// PublicGet renders a share without authentication (GET /share/{id}).
// Returns 410 Gone once the owner revokes the share.
func (h *ShareHandler) PublicGet(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("id")
	if token == "" {
		NotFound(w, "share not found")
		return
	}
	if db.Pool == nil {
		NotFound(w, "share not found")
		return
	}

	var shareID, sessionID, title string
	var messageIDs []string
	var createdAt time.Time
	var revokedAt *time.Time
	err := db.Pool.QueryRow(r.Context(),
		`SELECT id, session_id, title, message_ids, created_at, revoked_at
		 FROM conversation_shares WHERE id = $1`, token).
		Scan(&shareID, &sessionID, &title, &messageIDs, &createdAt, &revokedAt)
	if err != nil {
		NotFound(w, "share not found")
		return
	}
	if revokedAt != nil {
		JSON(w, http.StatusGone, APIResponse{Success: false, Error: "share revoked"})
		return
	}

	// 只暴露用户选中的文本消息（user/assistant），按时间正序
	rows, err := db.Pool.Query(r.Context(),
		`SELECT role, content, created_at FROM messages
		 WHERE session_id = $1 AND id = ANY($2::text[]) AND role IN ('user', 'assistant')
		 ORDER BY created_at ASC, id ASC`, sessionID, messageIDs)
	if err != nil {
		InternalError(w, "query messages: "+err.Error())
		return
	}
	defer rows.Close()

	type sharedMessage struct {
		Role      string    `json:"role"`
		Content   string    `json:"content"`
		CreatedAt time.Time `json:"created_at"`
	}
	messages := make([]sharedMessage, 0, 16)
	for rows.Next() {
		var m sharedMessage
		if err := rows.Scan(&m.Role, &m.Content, &m.CreatedAt); err != nil {
			continue
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		InternalError(w, "iterate messages: "+err.Error())
		return
	}

	OK(w, map[string]interface{}{
		"id":         shareID,
		"title":      title,
		"created_at": createdAt,
		"messages":   messages,
	})
}

// activeShare returns the session's non-revoked share, or nil.
func (h *ShareHandler) activeShare(r *http.Request, sessionID string) (*model.ConversationShare, error) {
	if db.Pool == nil {
		return nil, nil
	}
	share := &model.ConversationShare{}
	err := db.Pool.QueryRow(r.Context(),
		`SELECT id, created_at FROM conversation_shares
		 WHERE session_id = $1 AND revoked_at IS NULL LIMIT 1`, sessionID).
		Scan(&share.ID, &share.CreatedAt)
	if err != nil {
		return nil, nil // no active share (or DB row missing)
	}
	return share, nil
}

func dedupe(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
