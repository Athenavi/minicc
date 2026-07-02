package collab

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/tools"
)

// ── helpers ──

func genID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func nullableStr(s string) *string {
	if s == "" { return nil }
	return &s
}

// ── TaskTool ──

type TaskTool struct{}

func NewTaskTool() *TaskTool { return &TaskTool{} }
func (t *TaskTool) Name() string       { return "collab_task_create" }
func (t *TaskTool) Description() string { return "Create a new project task." }

func (t *TaskTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	title, _ := input["title"].(string)
	if title == "" { return nil, fmt.Errorf("title is required") }
	project, _ := input["project"].(string)
	assignee, _ := input["assignee"].(string)
	priority, _ := input["priority"].(string)
	if priority == "" { priority = "medium" }
	desc, _ := input["description"].(string)

	id := genID("task")

	if db.Pool != nil {
		db.Pool.Exec(ctx,
			`INSERT INTO enterprise_tasks (id, title, description, project, assignee, priority, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, 'open', NOW(), NOW())`,
			id, title, desc, nullableStr(project), nullableStr(assignee), priority)
	}

	return map[string]interface{}{
		"output":    fmt.Sprintf("Task created: %s (ID: %s)\n  Project: %s\n  Assignee: %s\n  Priority: %s", title, id, project, assignee, priority),
		"id":        id,
		"title":     title,
		"project":   project,
		"assignee":  assignee,
		"priority":  priority,
	}, nil
}

// ── WikiTool ──

type WikiTool struct{}

func NewWikiTool() *WikiTool { return &WikiTool{} }
func (t *WikiTool) Name() string       { return "collab_wiki_create" }
func (t *WikiTool) Description() string { return "Create a wiki page." }

func (t *WikiTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	title, _ := input["title"].(string)
	if title == "" { return nil, fmt.Errorf("title is required") }
	content, _ := input["content"].(string)

	id := genID("wiki")

	if db.Pool != nil {
		db.Pool.Exec(ctx,
			`INSERT INTO wiki_pages (id, title, content, created_at, updated_at) VALUES ($1, $2, $3, NOW(), NOW())`,
			id, title, content)
	}

	return map[string]interface{}{
		"output":  fmt.Sprintf("Wiki page created: %s (ID: %s, %d chars)", title, id, len(content)),
		"id":      id,
		"title":   title,
		"content": content,
	}, nil
}

// ── WikiSearchTool ──

type WikiSearchTool struct{}

func NewWikiSearchTool() *WikiSearchTool { return &WikiSearchTool{} }
func (t *WikiSearchTool) Name() string       { return "collab_wiki_search" }
func (t *WikiSearchTool) Description() string { return "Search wiki pages by keyword." }

func (t *WikiSearchTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	query, _ := input["query"].(string)
	if query == "" { return nil, fmt.Errorf("query is required") }

	results := []map[string]string{}
	if db.Pool != nil {
		rows, err := db.Pool.Query(ctx,
			`SELECT id, title FROM wiki_pages
			 WHERE title ILIKE '%' || $1 || '%' OR content ILIKE '%' || $1 || '%'
			 ORDER BY updated_at DESC LIMIT 20`, query)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, title string
				if rows.Scan(&id, &title) == nil {
					results = append(results, map[string]string{"id": id, "title": title})
				}
			}
		}
	}

	if len(results) == 0 {
		return map[string]interface{}{
			"output": fmt.Sprintf("No wiki pages found for '%s'", query),
			"total":  0,
		}, nil
	}

	output := fmt.Sprintf("Wiki search results for '%s' (%d found):\n", query, len(results))
	for _, r := range results {
		output += fmt.Sprintf("  - %s (ID: %s)\n", r["title"], r["id"])
	}

	return map[string]interface{}{
		"output":  output,
		"total":   len(results),
		"results": results,
	}, nil
}

// ── OkrTool ──

type OkrTool struct{}

func NewOkrTool() *OkrTool { return &OkrTool{} }
func (t *OkrTool) Name() string       { return "collab_okr_create" }
func (t *OkrTool) Description() string { return "Create an OKR goal." }

func (t *OkrTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	objective, _ := input["objective"].(string)
	if objective == "" { return nil, fmt.Errorf("objective is required") }
	quarter, _ := input["quarter"].(string)

	krRaw, _ := input["key_results"].([]interface{})
	krJSON, _ := json.Marshal(krRaw)

	id := genID("okr")

	if db.Pool != nil {
		db.Pool.Exec(ctx,
			`INSERT INTO okrs (id, objective, key_results, quarter, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 'active', NOW(), NOW())`,
			id, objective, string(krJSON), nullableStr(quarter))
	}

	return map[string]interface{}{
		"output":    fmt.Sprintf("OKR created: %s (ID: %s)", objective, id),
		"id":        id,
		"objective": objective,
		"quarter":   quarter,
	}, nil
}

// ── MessageTool ──

type MessageTool struct{}

func NewMessageTool() *MessageTool { return &MessageTool{} }
func (t *MessageTool) Name() string       { return "collab_message_send" }
func (t *MessageTool) Description() string { return "Send a message to a team channel." }

func (t *MessageTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	channel, _ := input["channel"].(string)
	content, _ := input["content"].(string)
	if content == "" { return nil, fmt.Errorf("content is required") }

	// Store in messages table with a special session_id
	if db.Pool != nil {
		msgID := genID("msg")
		db.Pool.Exec(ctx,
			`INSERT INTO messages (id, session_id, role, content, created_at)
			 VALUES ($1, 'collab_channel_' || $2, 'assistant', $3, NOW())`,
			msgID, channel, content)
	}

	return map[string]interface{}{
		"output":  fmt.Sprintf("Message sent to #%s (%d chars)", channel, len(content)),
		"channel": channel,
	}, nil
}

// ── MeetingTool ──

type MeetingTool struct{}

func NewMeetingTool() *MeetingTool { return &MeetingTool{} }
func (t *MeetingTool) Name() string       { return "collab_meeting_summary" }
func (t *MeetingTool) Description() string { return "Summarize meeting notes." }

func (t *MeetingTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	notes, _ := input["notes"].(string)
	if notes == "" { return nil, fmt.Errorf("notes are required") }
	title, _ := input["title"].(string)
	if title == "" { title = "Meeting " + time.Now().Format("2006-01-02") }

	summary := fmt.Sprintf("Meeting Summary for '%s':\n- Total length: %d chars\n- Recorded at: %s",
		title, len(notes), time.Now().Format(time.RFC3339))

	id := genID("mtg")

	if db.Pool != nil {
		db.Pool.Exec(ctx,
			`INSERT INTO meeting_notes (id, title, notes, summary, created_at)
			 VALUES ($1, $2, $3, $4, NOW())`,
			id, title, notes, summary)
	}

	return map[string]interface{}{
		"output":  summary,
		"id":      id,
		"title":   title,
		"summary": summary,
	}, nil
}

func RegisterTools(tr *tools.ToolRegistry) {
	for _, t := range []tools.Tool{NewTaskTool(), NewWikiTool(), NewWikiSearchTool(), NewOkrTool(), NewMessageTool(), NewMeetingTool()} {
		tr.Register(t)
	}
}
