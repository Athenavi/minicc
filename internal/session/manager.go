package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/id"
	"github.com/athenavi/minicc/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	redisKeyPrefix = "session:"
	redisTTL       = 2 * time.Hour
)

// Manager provides session CRUD with Redis hot cache + PostgreSQL persistence.
// All methods degrade gracefully when Redis or PG is unavailable.
type Manager struct {
	pool *pgxpool.Pool
	rdb  db.RedisClient
}

func NewManager(pool *pgxpool.Pool, rdb db.RedisClient) *Manager {
	return &Manager{pool: pool, rdb: rdb}
}

// ── Session CRUD ──────────────────────────────────────────────────────────

// GetSession retrieves a session by ID. Checks Redis first, falls back to PG.
func (m *Manager) GetSession(ctx context.Context, id string) (*model.Session, error) {
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}

	// 1. Redis hot path
	if m.rdb != nil {
		data, err := m.rdb.Get(ctx, redisKeyPrefix+id).Bytes()
		if err == nil {
			var s model.Session
			if json.Unmarshal(data, &s) == nil {
				return &s, nil
			}
			// Corrupt cache entry — delete so next read falls through to PG
			m.rdb.Del(ctx, redisKeyPrefix+id)
		}
	}

	// 2. PG cold path
	if m.pool == nil {
		return nil, fmt.Errorf("database not available")
	}

	var s model.Session
	err := m.pool.QueryRow(ctx,
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

// DefaultTenantID is the default tenant for single-tenant deployments.
const DefaultTenantID = "00000000-0000-0000-0000-000000000001"

// CreateSession inserts a new session into PG and caches in Redis.
// If id is empty, returns an error.
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

	var uid *string
	if userID != "" {
		uid = &userID
	}

	if m.pool != nil {
		_, err := m.pool.Exec(ctx,
			`INSERT INTO sessions (id, tenant_id, user_id, title, created_at, updated_at)
			 VALUES ($1, $2, $3::uuid, $4, $5, $6)
			 ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, updated_at = EXCLUDED.updated_at`,
			id, DefaultTenantID, uid, title, now, now)
		if err != nil {
			return nil, fmt.Errorf("create session: %w", err)
		}
	}

	m.cacheSession(ctx, s)
	return s, nil
}

// ListSessions returns sessions for a given user, newest first.
func (m *Manager) ListSessions(ctx context.Context, userID string) ([]model.Session, error) {
	if m.pool == nil {
		return nil, nil
	}

	rows, err := m.pool.Query(ctx,
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
		m.cacheSession(ctx, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}

	if sessions == nil {
		sessions = []model.Session{}
	}
	return sessions, nil
}

// DeleteSession removes a session from PG (CASCADE deletes messages) and Redis cache.
// Evict cache first so a subsequent GetSession falls through to PG (source of truth)
// even if Redis eviction fails silently.
func (m *Manager) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("session id is required")
	}

	m.evictCache(ctx, id)

	if m.pool != nil {
		_, err := m.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id)
		if err != nil {
			return fmt.Errorf("delete session: %w", err)
		}
	}

	return nil
}

// ── Message helpers ───────────────────────────────────────────────────────

// SaveMessage inserts a single message into the session's message log.
func (m *Manager) SaveMessage(ctx context.Context, sessionID, role, content string) error {
	if m.pool == nil {
		return nil
	}

	_, err := m.pool.Exec(ctx,
		`INSERT INTO sessions (id, tenant_id, user_id, title, created_at, updated_at)
		 VALUES ($1, $2, NULL, '', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET updated_at = NOW()`,
		sessionID, DefaultTenantID)
	if err != nil {
		return fmt.Errorf("ensure session: %w", err)
	}

	msgID := genID()
	_, err = m.pool.Exec(ctx,
		`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, $3, $4, NOW())`,
		msgID, sessionID, role, content)
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}

	m.evictCache(ctx, sessionID)
	return nil
}

// SaveMessages saves user + assistant messages and updates the session title.
func (m *Manager) SaveMessages(ctx context.Context, sessionID, userID, userContent, assistantContent string) {
	if m.pool == nil {
		return
	}

	_, err := m.pool.Exec(ctx,
		`INSERT INTO sessions (id, tenant_id, user_id, title, created_at, updated_at)
		 VALUES ($1, $2, NULLIF($3, ''), '', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET updated_at = NOW()`,
		sessionID, DefaultTenantID, userID)
	if err != nil {
		slog.Warn("ensure session", "error", err)
		return
	}

	if userContent != "" {
		_, err := m.pool.Exec(ctx,
			`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, 'user', $3, NOW())`,
			genID(), sessionID, userContent)
		if err != nil {
			slog.Warn("save user message", "error", err)
		}
	}
	if assistantContent != "" {
		_, err := m.pool.Exec(ctx,
			`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, 'assistant', $3, NOW())`,
			genID(), sessionID, assistantContent)
		if err != nil {
			slog.Warn("save assistant message", "error", err)
		}
	}

	if userContent != "" {
		_, err := m.pool.Exec(ctx,
			`UPDATE sessions SET title = LEFT($1, 255), updated_at = NOW()
			 WHERE id = $2 AND (title = '' OR title IS NULL)`,
			truncateTitle(userContent), sessionID)
		if err != nil {
			slog.Warn("update session title", "error", err)
		}
	}
	m.evictCache(ctx, sessionID)
}

// ── Message query ─────────────────────────────────────────────────────────

// GetMessages retrieves messages for a session, oldest first, with optional limit.
// If limit <= 0, returns all messages (legacy behavior).
func (m *Manager) GetMessages(ctx context.Context, sessionID string, limit ...int) ([]model.Message, error) {
	if m.pool == nil {
		return nil, nil
	}

	query := `SELECT id, session_id, role, content, created_at
		   FROM messages
		   WHERE session_id = $1
		   ORDER BY created_at ASC`
	args := []interface{}{sessionID}

	if len(limit) > 0 && limit[0] > 0 {
		// 子查询：先取最新的 N 条，再按正序排列，保持"最早优先"的返回契约
		query = `SELECT id, session_id, role, content, created_at FROM (
			   SELECT id, session_id, role, content, created_at
			   FROM messages
			   WHERE session_id = $1
			   ORDER BY created_at DESC
			   LIMIT $2
		   ) sub ORDER BY created_at ASC`
		args = append(args, limit[0])
	}

	rows, err := m.pool.Query(ctx, query, args...)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	if msgs == nil {
		msgs = []model.Message{}
	}
	return msgs, nil
}

// ── Cache helpers ─────────────────────────────────────────────────────────

func (m *Manager) cacheSession(ctx context.Context, s *model.Session) {
	if m.rdb == nil {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	if err := m.rdb.Set(ctx, redisKeyPrefix+s.ID, data, redisTTL).Err(); err != nil {
		slog.Warn("session cache set", "error", err)
	}
}

func (m *Manager) evictCache(ctx context.Context, id string) {
	if m.rdb == nil {
		return
	}
	m.rdb.Del(ctx, redisKeyPrefix+id)
}

// ── Helpers ───────────────────────────────────────────────────────────────

func genID() string {
	return id.NextID()
}

func truncateTitle(s string) string {
	s = strings.TrimSpace(s)
	idx := strings.Index(s, "\n")
	if idx >= 0 {
		s = s[:idx]
	}
	if utf8.RuneCountInString(s) > 120 {
		runes := []rune(s)
		s = string(runes[:120])
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
