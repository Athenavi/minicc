package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/model"

	"github.com/jackc/pgx/v5"
)

const (
	redisKeyPrefix = "session:"
	redisTTL       = 2 * time.Hour
)

// Manager provides session CRUD with Redis hot cache + PostgreSQL persistence.
// All methods degrade gracefully when Redis or PG is unavailable.
type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

// ── Session CRUD ──────────────────────────────────────────────────────────

// GetSession retrieves a session by ID. Checks Redis first, falls back to PG.
func (m *Manager) GetSession(ctx context.Context, id string) (*model.Session, error) {
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}

	// 1. Redis hot path
	if db.Redis != nil {
		data, err := db.Redis.Get(ctx, redisKeyPrefix+id).Bytes()
		if err == nil {
			var s model.Session
			if json.Unmarshal(data, &s) == nil {
				return &s, nil
			}
		}
	}

	// 2. PG cold path
	if db.Pool == nil {
		return nil, fmt.Errorf("database not available")
	}

	var s model.Session
	err := db.Pool.QueryRow(ctx,
		`SELECT id, COALESCE(user_id, ''), COALESCE(title, ''), created_at, updated_at
		 FROM sessions WHERE id = $1`, id).
		Scan(&s.ID, &s.UserID, &s.Title, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", id)
	} else if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}

	// Warm Redis cache
	m.cacheSession(ctx, &s)
	return &s, nil
}

// CreateSession inserts a new session into PG and caches in Redis.
func (m *Manager) CreateSession(ctx context.Context, id, userID, title string) (*model.Session, error) {
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if title == "" {
		title = "New Chat"
	}

	now := time.Now()
	s := &model.Session{
		ID:        id,
		UserID:    userID,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if db.Pool != nil {
		_, err := db.Pool.Exec(ctx,
			`INSERT INTO sessions (id, user_id, title, created_at, updated_at)
			 VALUES ($1, NULLIF($2, ''), $3, $4, $5)
			 ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, updated_at = EXCLUDED.updated_at`,
			s.ID, s.UserID, s.Title, s.CreatedAt, s.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("create session: %w", err)
		}
	}

	m.cacheSession(ctx, s)
	return s, nil
}

// ListSessions returns sessions for a given user, newest first.
func (m *Manager) ListSessions(ctx context.Context, userID string) ([]model.Session, error) {
	if db.Pool == nil {
		return nil, nil
	}

	rows, err := db.Pool.Query(ctx,
		`SELECT id, COALESCE(user_id, ''), COALESCE(title, ''), created_at, updated_at
		 FROM sessions
		 WHERE user_id = $1
		 ORDER BY updated_at DESC
		 LIMIT 100`, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var s model.Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
			slog.Warn("scan session row", "error", err)
			continue
		}
		sessions = append(sessions, s)
		// Warm cache for each session
		m.cacheSession(ctx, &s)
	}

	if sessions == nil {
		sessions = []model.Session{}
	}
	return sessions, nil
}

// DeleteSession removes a session from PG (CASCADE deletes messages) and Redis cache.
func (m *Manager) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("session id is required")
	}

	if db.Pool != nil {
		_, err := db.Pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id)
		if err != nil {
			return fmt.Errorf("delete session: %w", err)
		}
	}

	m.evictCache(ctx, id)
	return nil
}

// ── Message helpers ───────────────────────────────────────────────────────

// SaveMessage inserts a single message into the session's message log.
// Auto-creates the session if it doesn't exist yet.
func (m *Manager) SaveMessage(ctx context.Context, sessionID, role, content string) error {
	if db.Pool == nil {
		return nil
	}

	// Ensure session exists (create if not)
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, title, created_at, updated_at)
		 VALUES ($1, NULL, '', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET updated_at = NOW()`,
		sessionID)
	if err != nil {
		return fmt.Errorf("ensure session: %w", err)
	}

	msgID := fmt.Sprintf("%d", time.Now().UnixNano())
	_, err = db.Pool.Exec(ctx,
		`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, $3, $4, NOW())`,
		msgID, sessionID, role, content)
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}

	m.evictCache(ctx, sessionID)
	return nil
}

// SaveMessages is a convenience wrapper that saves user + assistant messages
// in a single call, then updates the session title from the first user message.
func (m *Manager) SaveMessages(ctx context.Context, sessionID, userID, userContent, assistantContent string) {
	if db.Pool == nil {
		return
	}

	// Ensure session exists
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, title, created_at, updated_at)
		 VALUES ($1, NULLIF($2, ''), '', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET updated_at = NOW()`,
		sessionID, nullableStr(userID))
	if err != nil {
		slog.Warn("ensure session", "error", err)
		return
	}

	// Save messages
	if userContent != "" {
		_, err := db.Pool.Exec(ctx,
			`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, 'user', $3, NOW())`,
			genID(), sessionID, userContent)
		if err != nil {
			slog.Warn("save user message", "error", err)
		}
	}
	if assistantContent != "" {
		_, err := db.Pool.Exec(ctx,
			`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, 'assistant', $3, NOW())`,
			genID(), sessionID, assistantContent)
		if err != nil {
			slog.Warn("save assistant message", "error", err)
		}
	}

	// Update session title from first user message
	if userContent != "" {
		db.Pool.Exec(ctx,
			`UPDATE sessions SET title = LEFT($1, 255), updated_at = NOW()
			 WHERE id = $2 AND (title = '' OR title IS NULL)`,
			truncateTitle(userContent), sessionID)
	}
}

// ── Message query ─────────────────────────────────────────────────────────

// GetMessages retrieves all messages for a session, oldest first.
func (m *Manager) GetMessages(ctx context.Context, sessionID string) ([]model.Message, error) {
	if db.Pool == nil {
		return nil, nil
	}

	rows, err := db.Pool.Query(ctx,
		`SELECT id, session_id, role, content, created_at
		 FROM messages
		 WHERE session_id = $1
		 ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var msgs []model.Message
	for rows.Next() {
		var msg model.Message
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.CreatedAt); err != nil {
			slog.Warn("scan message row", "error", err)
			continue
		}
		msgs = append(msgs, msg)
	}

	if msgs == nil {
		msgs = []model.Message{}
	}
	return msgs, nil
}

// ── Cache helpers ─────────────────────────────────────────────────────────

func (m *Manager) cacheSession(ctx context.Context, s *model.Session) {
	if db.Redis == nil {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	if err := db.Redis.Set(ctx, redisKeyPrefix+s.ID, data, redisTTL).Err(); err != nil {
		slog.Warn("session cache set", "error", err)
	}
}

func (m *Manager) evictCache(ctx context.Context, id string) {
	if db.Redis == nil {
		return
	}
	db.Redis.Del(ctx, redisKeyPrefix+id)
}

// ── Helpers ───────────────────────────────────────────────────────────────

func genID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func truncateTitle(s string) string {
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
