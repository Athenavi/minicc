package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/db"
)

// ── Enterprise Tools: CRUD for enterprise database tables ──────────────────

// EnterpriseQueryTool runs SQL queries against enterprise tables.
type EnterpriseQueryTool struct{}

func NewEnterpriseQueryTool() *EnterpriseQueryTool { return &EnterpriseQueryTool{} }
func (t *EnterpriseQueryTool) Name() string       { return "enterprise_query" }
func (t *EnterpriseQueryTool) Description() string { return "Query enterprise data (tasks, wiki, OKRs, meetings, tickets, KB, campaigns)." }

var enterpriseTables = map[string]string{
	"tasks":    "enterprise_tasks",
	"wiki":     "wiki_pages",
	"okrs":     "okrs",
	"meetings": "meeting_notes",
	"tickets":  "support_tickets",
	"kb":       "kb_articles",
	"campaigns": "marketing_campaigns",
}

func (t *EnterpriseQueryTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	if db.Pool == nil {
		return map[string]interface{}{
			"output": "Database not available. Enterprise features require PostgreSQL.",
			"count":  0,
		}, nil
	}

	tableKey, _ := input["table"].(string)
	if tableKey == "" {
		// Return overview of all tables
		counts := map[string]int{}
		for key, table := range enterpriseTables {
			var count int
			db.Pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
			counts[key] = count
		}
		return map[string]interface{}{
			"output":  fmt.Sprintf("Enterprise overview: %d tables", len(counts)),
			"counts":  counts,
			"tables":  len(counts),
		}, nil
	}

	table, ok := enterpriseTables[tableKey]
	if !ok {
		return nil, fmt.Errorf("unknown enterprise table: %q (valid: %s)", tableKey, strings.Join(mapKeys(enterpriseTables), ", "))
	}

	limit := 20
	if l, ok := input["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	rows, err := db.Pool.Query(ctx,
		fmt.Sprintf("SELECT id, COALESCE(title, name, ''), created_at, updated_at FROM %s ORDER BY updated_at DESC NULLS LAST, created_at DESC LIMIT $1", table),
		limit)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", table, err)
	}
	defer rows.Close()

	type Record struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	var records []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.Title, &r.CreatedAt, &r.UpdatedAt); err != nil {
			slog.Warn("scan enterprise row", "error", err)
			continue
		}
		records = append(records, r)
	}
	if records == nil {
		records = []Record{}
	}

	return map[string]interface{}{
		"output":  fmt.Sprintf("%s: %d records", tableKey, len(records)),
		"table":   tableKey,
		"records": records,
		"count":   len(records),
	}, nil
}

// EnterpriseInsertTool inserts a record into an enterprise table.
type EnterpriseInsertTool struct{}

func NewEnterpriseInsertTool() *EnterpriseInsertTool { return &EnterpriseInsertTool{} }
func (t *EnterpriseInsertTool) Name() string       { return "enterprise_insert" }
func (t *EnterpriseInsertTool) Description() string { return "Insert a record into an enterprise table." }
func (t *EnterpriseInsertTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	if db.Pool == nil {
		return nil, fmt.Errorf("database not available")
	}

	tableKey, _ := input["table"].(string)
	title, _ := input["title"].(string)
	if tableKey == "" || title == "" {
		return nil, fmt.Errorf("table and title are required")
	}

	table, ok := enterpriseTables[tableKey]
	if !ok {
		return nil, fmt.Errorf("unknown enterprise table: %q", tableKey)
	}

	payload, _ := input["payload"].(string)
	payloadJSON := "{}"
	if payload != "" {
		payloadJSON = payload
	}

	id := fmt.Sprintf("ent_%d", time.Now().UnixNano())
	_, err := db.Pool.Exec(ctx,
		fmt.Sprintf("INSERT INTO %s (id, title, content, created_at, updated_at) VALUES ($1, $2, $3::jsonb, NOW(), NOW())", table),
		id, title, payloadJSON)
	if err != nil {
		return nil, fmt.Errorf("insert %s: %w", table, err)
	}

	return map[string]interface{}{
		"output": fmt.Sprintf("Record created in %s: %s", tableKey, id),
		"id":     id,
		"table":  tableKey,
	}, nil
}

// ── DevOps Tools ──────────────────────────────────────────────────────────

// DevOpsHealthTool returns system health information.
type DevOpsHealthTool struct{}

func NewDevOpsHealthTool() *DevOpsHealthTool { return &DevOpsHealthTool{} }
func (t *DevOpsHealthTool) Name() string       { return "devops_health" }
func (t *DevOpsHealthTool) Description() string { return "Check system health — DB, Redis, uptime." }
func (t *DevOpsHealthTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	health := map[string]string{
		"postgres": "unknown",
		"redis":    "unknown",
	}

	if db.Pool != nil {
		if err := db.Pool.Ping(ctx); err == nil {
			health["postgres"] = "ok"
		} else {
			health["postgres"] = "error: " + err.Error()
		}
	} else {
		health["postgres"] = "not configured"
	}

	if db.Redis != nil {
		if err := db.Redis.Ping(ctx).Err(); err == nil {
			health["redis"] = "ok"
		} else {
			health["redis"] = "error: " + err.Error()
		}
	} else {
		health["redis"] = "not configured"
	}

	return map[string]interface{}{
		"output": fmt.Sprintf("Postgres: %s | Redis: %s", health["postgres"], health["redis"]),
		"health": health,
	}, nil
}

// ── Registration ──────────────────────────────────────────────────────────

func RegisterEnterpriseTools(registry *ToolRegistry) {
	registry.Register(NewEnterpriseQueryTool())
	registry.Register(NewEnterpriseInsertTool())
}

func RegisterDevOpsTools(registry *ToolRegistry) {
	registry.Register(NewDevOpsHealthTool())
}

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
