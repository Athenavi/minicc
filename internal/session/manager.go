package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/id"
	"github.com/athenavi/chiron/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	redisKeyPrefix = "session:"
	redisTTL       = 2 * time.Hour
)

// ErrSessionNotFound 琛ㄧず浼氳瘽涓嶅瓨鍦紙SSE 绔偣鎹鏀捐灏氭湭鍒涘缓鐨勬柊浼氳瘽杩炴帴锛夈€?var ErrSessionNotFound = errors.New("session not found")

// Manager provides session CRUD with Redis hot cache + PostgreSQL persistence.
// All methods degrade gracefully when Redis or PG is unavailable.
type Manager struct {
	pool *pgxpool.Pool
	rdb  db.RedisClient
}

func NewManager(pool *pgxpool.Pool, rdb db.RedisClient) *Manager {
	return &Manager{pool: pool, rdb: rdb}
}

// 鈹€鈹€ Session CRUD 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

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
			// Corrupt cache entry 鈥?delete so next read falls through to PG
			m.rdb.Del(ctx, redisKeyPrefix+id)
		}
	}

	// 2. PG cold path
	if m.pool == nil {
		return nil, fmt.Errorf("database not available")
	}

	var s model.Session
	err := m.pool.QueryRow(ctx,
		`SELECT id, COALESCE(user_id::text, ''), COALESCE(title, ''), COALESCE(pinned, false), created_at, updated_at
		 FROM sessions WHERE id = $1`, id).
		Scan(&s.ID, &s.UserID, &s.Title, &s.Pinned, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
	} else if err != nil {
		// 闈炴硶 uuid 鏍煎紡锛堝鍓嶇鏃х増 fallback id "session_xxx"锛夛細浼氳瘽蹇呯劧涓嶅瓨鍦紝
		// 鎸?not found 澶勭悊锛堝惁鍒?SSE 绔偣浼?500銆佸墠绔Е鍙?杩炴帴宸叉柇寮€"锛?		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "22P02" {
			return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
		}
		return nil, fmt.Errorf("query session: %w", err)
	}

	// Warm Redis cache
	m.cacheSession(ctx, &s)
	return &s, nil
}

// DefaultTenantID is the default tenant for single-tenant deployments.
// 鍗曚竴鏉ユ簮瑙?internal/db/seed.go銆?const DefaultTenantID = db.DefaultTenantID

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
		`SELECT id, COALESCE(user_id::text, ''), COALESCE(title, ''), COALESCE(pinned, false), created_at, updated_at
		 FROM sessions
		 WHERE user_id = $1
		 ORDER BY pinned DESC, updated_at DESC
		 LIMIT 100`, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var s model.Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.Title, &s.Pinned, &s.CreatedAt, &s.UpdatedAt); err != nil {
			slog.Warn("scan session row", "error", err)
			continue
		}
		sessions = append(sessions, s)
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

// UpdateSession updates a session's title and/or pinned flag, then refreshes
// the Redis cache. updated_at advances only when the title changes (pinning is
// a list-order preference, not activity). At least one field must be non-nil.
func (m *Manager) UpdateSession(ctx context.Context, id string, title *string, pinned *bool) (*model.Session, error) {
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if title == nil && pinned == nil {
		return nil, fmt.Errorf("nothing to update")
	}

	var sets []string
	var args []interface{}
	if title != nil {
		sets = append(sets, fmt.Sprintf("title = $%d", len(args)+1))
		args = append(args, *title)
		sets = append(sets, fmt.Sprintf("updated_at = $%d", len(args)+1))
		args = append(args, time.Now())
	}
	if pinned != nil {
		sets = append(sets, fmt.Sprintf("pinned = $%d", len(args)+1))
		args = append(args, *pinned)
	}
	args = append(args, id)

	if m.pool != nil {
		_, err := m.pool.Exec(ctx,
			`UPDATE sessions SET `+strings.Join(sets, ", ")+` WHERE id = $`+strconv.Itoa(len(args)),
			args...)
		if err != nil {
			return nil, fmt.Errorf("update session: %w", err)
		}
	}

	m.evictCache(ctx, id)
	return m.GetSession(ctx, id)
}

// 鈹€鈹€ Message helpers 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

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

	msgID, err := genID()
	if err != nil {
		return fmt.Errorf("generate message id: %w", err)
	}
	_, err = m.pool.Exec(ctx,
		`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, $3, $4, NOW())`,
		msgID, sessionID, role, content)
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}

	m.evictCache(ctx, sessionID)
	return nil
}

// SaveUserMessage persists the user message immediately at submit time
// (S 淇锛氫笂涓嬫枃涓㈠け 鈥?SSE 涓柇/鍋滄鏃朵笉鍐嶄涪澶辩敤鎴锋秷鎭紝鍘嗗彶鍙画).
func (m *Manager) SaveUserMessage(ctx context.Context, sessionID, userID, userContent string) {
	if m.pool == nil || userContent == "" {
		return
	}
	_, err := m.pool.Exec(ctx,
		`INSERT INTO sessions (id, tenant_id, user_id, title, created_at, updated_at)
		 VALUES ($1, $2, NULLIF($3, '')::uuid, '', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET updated_at = NOW()`,
		sessionID, DefaultTenantID, userID)
	if err != nil {
		slog.Warn("ensure session", "error", err)
		return
	}
	msgID, err := genID()
	if err != nil {
		slog.Warn("generate message id", "error", err)
		return
	}
	_, err = m.pool.Exec(ctx,
		`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, 'user', $3, NOW())`,
		msgID, sessionID, userContent)
	if err != nil {
		slog.Warn("save user message", "error", err)
	}
	_, err = m.pool.Exec(ctx,
		`UPDATE sessions SET title = LEFT($1, 255), updated_at = NOW()
		 WHERE id = $2 AND (title = '' OR title IS NULL)`,
		truncateTitle(userContent), sessionID)
	if err != nil {
		slog.Warn("update session title", "error", err)
	}
	m.evictCache(ctx, sessionID)
}

// SaveAssistantMessage persists the assistant reply (with optional OpenAI-format
// tool_calls JSON) after streaming completes.
func (m *Manager) SaveAssistantMessage(ctx context.Context, sessionID, assistantContent, toolCallsJSON string) {
	if m.pool == nil {
		return
	}
	if toolCallsJSON == "" {
		toolCallsJSON = "[]"
	}
	// 鍏佽"绾伐鍏疯皟鐢ㄨ疆"锛坈ontent 绌?+ tool_calls 闈炵┖锛夎惤搴擄紙S 淇锛?	if assistantContent == "" && toolCallsJSON == "[]" {
		return
	}
	msgID, err := genID()
	if err != nil {
		slog.Warn("generate message id", "error", err)
		return
	}
	_, err = m.pool.Exec(ctx,
		`INSERT INTO messages (id, session_id, role, content, tool_calls, created_at)
		 VALUES ($1, $2, 'assistant', $3, $4::jsonb, NOW())`,
		msgID, sessionID, assistantContent, toolCallsJSON)
	if err != nil {
		slog.Warn("save assistant message", "error", err)
	}
	m.evictCache(ctx, sessionID)
}

// SaveToolCall persists a tool call record (S 淇锛氬伐鍏疯皟鐢ㄨ繃绋嬭惤搴擄紝鍒锋柊鍚庢樉绀轰竴鑷?.
func (m *Manager) SaveToolCall(ctx context.Context, sessionID, toolCallID, toolName, inputJSON string) {
	if m.pool == nil || toolCallID == "" {
		return
	}
	_, err := m.pool.Exec(ctx,
		`INSERT INTO tool_calls (id, session_id, tool_name, input, created_at)
		 VALUES ($1, $2, $3, $4::jsonb, NOW())
		 ON CONFLICT (id) DO UPDATE SET tool_name = EXCLUDED.tool_name, input = EXCLUDED.input`,
		toolCallID, sessionID, toolName, inputJSON)
	if err != nil {
		slog.Warn("save tool call", "error", err)
	}
}

// UpdateToolCall stores the tool result on the matching record (S 淇).
func (m *Manager) UpdateToolCall(ctx context.Context, toolCallID, output string, isError bool) {
	if m.pool == nil || toolCallID == "" {
		return
	}
	_, err := m.pool.Exec(ctx,
		`UPDATE tool_calls SET output = $2, is_error = $3
		 WHERE id = $1`,
		toolCallID, output, isError)
	if err != nil {
		slog.Warn("update tool call", "error", err)
	}
}

// GetToolCallsPage retrieves a page of tool call records older than the cursor,
// aligned with GetMessagesPage pagination (created_at|id cursor).
func (m *Manager) GetToolCallsPage(ctx context.Context, sessionID string, limit int, before string) ([]model.ToolCall, error) {
	if m.pool == nil || limit <= 0 {
		return nil, nil
	}
	query := `SELECT id, session_id, tool_name, input, output, is_error, created_at
		 FROM tool_calls WHERE session_id = $1`
	args := []interface{}{sessionID}
	if before != "" {
		parts := strings.SplitN(before, "|", 2)
		if len(parts) == 2 {
			t, err := time.Parse(time.RFC3339Nano, parts[0])
			if err == nil {
				query += ` AND (created_at < $2 OR (created_at = $2 AND id < $3))`
				args = append(args, t, parts[1])
			}
		}
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit+1)

	rows, err := m.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ToolCall
	for rows.Next() {
		var tc model.ToolCall
		var inputJSON []byte
		var created time.Time
		if err := rows.Scan(&tc.ID, &tc.SessionID, &tc.ToolName, &inputJSON, &tc.Output, &tc.IsError, &created); err != nil {
			return nil, err
		}
		tc.Input = string(inputJSON)
		tc.CreatedAt = created
		out = append(out, tc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > limit {
		out = out[:limit]
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// GetToolCalls returns all tool call records for a session, oldest first.
func (m *Manager) GetToolCalls(ctx context.Context, sessionID string) ([]model.ToolCall, error) {
	if m.pool == nil {
		return nil, nil
	}
	rows, err := m.pool.Query(ctx,
		`SELECT id, session_id, tool_name, input, output, is_error, created_at
		 FROM tool_calls WHERE session_id = $1 ORDER BY created_at ASC, id ASC`,
		sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ToolCall
	for rows.Next() {
		var tc model.ToolCall
		var inputJSON []byte
		var created time.Time
		if err := rows.Scan(&tc.ID, &tc.SessionID, &tc.ToolName, &inputJSON, &tc.Output, &tc.IsError, &created); err != nil {
			return nil, err
		}
		tc.Input = string(inputJSON)
		tc.CreatedAt = created
		out = append(out, tc)
	}
	return out, rows.Err()
}

// SaveMessages saves user + assistant messages and updates the session title.
func (m *Manager) SaveMessages(ctx context.Context, sessionID, userID, userContent, assistantContent string) {
	if m.pool == nil {
		return
	}

	_, err := m.pool.Exec(ctx,
		`INSERT INTO sessions (id, tenant_id, user_id, title, created_at, updated_at)
		 VALUES ($1, $2, NULLIF($3, '')::uuid, '', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET updated_at = NOW()`,
		sessionID, DefaultTenantID, userID)
	if err != nil {
		slog.Warn("ensure session", "error", err)
		return
	}

	if userContent != "" {
		msgID, err := genID()
		if err != nil {
			slog.Warn("generate message id", "error", err)
			return
		}
		_, err = m.pool.Exec(ctx,
			`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, 'user', $3, NOW())`,
			msgID, sessionID, userContent)
		if err != nil {
			slog.Warn("save user message", "error", err)
		}
	}
	if assistantContent != "" {
		msgID, err := genID()
		if err != nil {
			slog.Warn("generate message id", "error", err)
			return
		}
		_, err = m.pool.Exec(ctx,
			`INSERT INTO messages (id, session_id, role, content, created_at) VALUES ($1, $2, 'assistant', $3, NOW())`,
			msgID, sessionID, assistantContent)
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

// 鈹€鈹€ Message query 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// MessagePage is a page of messages plus the cursor for loading earlier pages.
type MessagePage struct {
	Messages []model.Message
	HasMore  bool
	// Cursor 鎸囧悜鏈〉鏈€鏃╀竴鏉★紙created_at|id锛夛紝鍓嶇鐢ㄥ畠璇锋眰鏇存棭涓€椤?	Cursor string
}

// GetMessagesPage retrieves a page of messages (newest-first internally, returned
// oldest-first). When before is empty, returns the newest `limit` messages; when
// before is set (format "RFC3339Nano|msgID"), returns the `limit` messages older
// than that cursor. HasMore reports whether even older messages exist.
func (m *Manager) GetMessagesPage(ctx context.Context, sessionID string, limit int, before string) (MessagePage, error) {
	page := MessagePage{}
	if m.pool == nil || limit <= 0 {
		return page, nil
	}
	// 澶氬彇 1 鏉＄敤浜庡垽鏂槸鍚﹁繕鏈夋洿鏃╃殑鏁版嵁
	query := `SELECT id, session_id, role, content, COALESCE(tool_calls::text, ''), created_at
		   FROM messages
		   WHERE session_id = $1`
	args := []interface{}{sessionID}

	if before != "" {
		// 娓告爣鏍煎紡 created_at|id锛堝鍚堟父鏍囷紝鍚?created_at 涔熺ǔ瀹氬垎椤碉級
		parts := strings.SplitN(before, "|", 2)
		if len(parts) == 2 {
			t, err := time.Parse(time.RFC3339Nano, parts[0])
			if err == nil {
				query += ` AND (created_at < $2 OR (created_at = $2 AND id < $3))`
				args = append(args, t, parts[1])
			}
		}
	}

	query += ` ORDER BY created_at DESC, id DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit+1)

	rows, err := m.pool.Query(ctx, query, args...)
	if err != nil {
		return page, fmt.Errorf("query messages page: %w", err)
	}
	defer rows.Close()

	var msgs []model.Message
	for rows.Next() {
		var msg model.Message
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.ToolCalls, &msg.CreatedAt); err != nil {
			slog.Warn("scan message row", "error", err)
			continue
		}
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("iterate messages page: %w", err)
	}

	if len(msgs) > limit {
		page.HasMore = true
		msgs = msgs[:limit]
	}
	// 鍊掑簭缈昏浆涓烘搴忥紙鏈€鏃╀紭鍏堬級
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	page.Messages = msgs
	if len(msgs) > 0 {
		first := msgs[0]
		page.Cursor = first.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + first.ID
	}
	return page, nil
}

// GetMessages retrieves messages for a session, oldest first, with optional limit.
// If limit <= 0, returns all messages (legacy behavior).
func (m *Manager) GetMessages(ctx context.Context, sessionID string, limit ...int) ([]model.Message, error) {
	if m.pool == nil {
		return nil, nil
	}

	query := `SELECT id, session_id, role, content, COALESCE(tool_calls::text, ''), created_at
		   FROM messages
		   WHERE session_id = $1
		   ORDER BY created_at ASC`
	args := []interface{}{sessionID}

	if len(limit) > 0 && limit[0] > 0 {
		// 瀛愭煡璇細鍏堝彇鏈€鏂扮殑 N 鏉★紝鍐嶆寜姝ｅ簭鎺掑垪锛屼繚鎸?鏈€鏃╀紭鍏?鐨勮繑鍥炲绾?		query = `SELECT id, session_id, role, content, COALESCE(tool_calls::text, ''), created_at FROM (
			   SELECT id, session_id, role, content, tool_calls, created_at
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
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.ToolCalls, &msg.CreatedAt); err != nil {
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

// 鈹€鈹€ Cache helpers 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

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

// 鈹€鈹€ Helpers 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func genID() (string, error) {
	return id.UUID()
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
