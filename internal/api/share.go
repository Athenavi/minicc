package api

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/model"
	"github.com/athenavi/chiron/internal/session"
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
// lowercase, no padding 鈥?16 chars). Shares must not be enumerable, unlike
// snowflake ids, because the id is the whole access control.
func shareToken() (string, error) {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("share: crypto/rand unavailable: %w", err)
	}
	return strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])), nil
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

	var body struct {
		MessageIDs []string `json:"message_ids"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	messageIDs := dedupe(body.MessageIDs)
	if len(messageIDs) == 0 {
		BadRequest(w, "message_ids is required")
		return
	}

	// 娑堟伅褰掑睘鏍￠獙锛氬彧鍏佽鍒嗕韩鏈細璇濈殑娑堟伅
	rows, err := db.Pool.Query(r.Context(),
		`SELECT id FROM messages WHERE session_id = $1 AND id = ANY($2::text[])`, id, messageIDs)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "validate messages failed")
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
		logAndRespond(w, err, http.StatusInternalServerError, "validate messages failed")
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

	// 骞傜瓑锛氬凡鏈夋椿璺冨垎浜洿鎺ヨ繑鍥?
	existing, err := h.activeShare(r, id)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "query share failed")
		return
	}
	if existing != nil {
		OK(w, map[string]interface{}{"share_id": existing.ID, "created_at": existing.CreatedAt})
		return
	}

	token, err := shareToken()
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create share failed")
		return
	}
	_, err = db.Pool.Exec(r.Context(),
		`INSERT INTO conversation_shares (id, session_id, user_id, title, message_ids, created_at)
		 VALUES ($1, $2, $3::uuid, $4, $5, $6)`,
		token, id, claims.UserID, sess.Title, valid, time.Now())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create share failed")
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

	share, err := h.activeShare(r, id)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "query share failed")
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

	if _, err := db.Pool.Exec(r.Context(),
		`UPDATE conversation_shares SET revoked_at = NOW() WHERE session_id = $1 AND revoked_at IS NULL`, id); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "revoke share failed")
		return
	}
	OK(w, map[string]string{"status": "revoked"})
}

// PublicGet renders a share without authentication (GET /v1/share/{id}).
// Returns 410 Gone once the owner revokes the share.
func (h *ShareHandler) PublicGet(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("id")
	if token == "" {
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

	// 鍙毚闇茬敤鎴烽€変腑鐨勬枃鏈秷鎭紙user/assistant锛夛紝鎸夋椂闂存搴?
	rows, err := db.Pool.Query(r.Context(),
		`SELECT role, content, created_at FROM messages
		 WHERE session_id = $1 AND id = ANY($2::text[]) AND role IN ('user', 'assistant')
		 ORDER BY created_at ASC, id ASC`, sessionID, messageIDs)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "query messages failed")
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
		logAndRespond(w, err, http.StatusInternalServerError, "iterate messages failed")
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
