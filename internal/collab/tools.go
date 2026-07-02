package collab

import (
	"context"
	"fmt"

	"github.com/athenavi/minicc/internal/tools"
)

// TaskTool creates a project task.
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

	return map[string]interface{}{
		"output":    fmt.Sprintf("Task created: %s\n  Project: %s\n  Assignee: %s\n  Priority: %s", title, project, assignee, priority),
		"title":     title,
		"project":   project,
		"assignee":  assignee,
		"priority":  priority,
	}, nil
}

// WikiTool creates a wiki page.
type WikiTool struct{}

func NewWikiTool() *WikiTool { return &WikiTool{} }
func (t *WikiTool) Name() string       { return "collab_wiki_create" }
func (t *WikiTool) Description() string { return "Create a wiki page." }

func (t *WikiTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	title, _ := input["title"].(string)
	if title == "" { return nil, fmt.Errorf("title is required") }
	content, _ := input["content"].(string)

	return map[string]interface{}{
		"output":  fmt.Sprintf("Wiki page created: %s (%d chars)", title, len(content)),
		"title":   title,
		"content": content,
	}, nil
}

// WikiSearchTool searches wiki pages.
type WikiSearchTool struct{}

func NewWikiSearchTool() *WikiSearchTool { return &WikiSearchTool{} }
func (t *WikiSearchTool) Name() string       { return "collab_wiki_search" }
func (t *WikiSearchTool) Description() string { return "Search wiki pages by keyword." }

func (t *WikiSearchTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	query, _ := input["query"].(string)
	if query == "" { return nil, fmt.Errorf("query is required") }

	return map[string]interface{}{
		"output": fmt.Sprintf("Wiki search results for '%s':\n  (simulated)", query),
		"query":  query,
	}, nil
}

// OkrTool creates an OKR goal.
type OkrTool struct{}

func NewOkrTool() *OkrTool { return &OkrTool{} }
func (t *OkrTool) Name() string       { return "collab_okr_create" }
func (t *OkrTool) Description() string { return "Create an OKR goal." }

func (t *OkrTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	objective, _ := input["objective"].(string)
	if objective == "" { return nil, fmt.Errorf("objective is required") }

	return map[string]interface{}{
		"output":     fmt.Sprintf("OKR created: %s", objective),
		"objective":  objective,
	}, nil
}

// MessageTool sends a team message.
type MessageTool struct{}

func NewMessageTool() *MessageTool { return &MessageTool{} }
func (t *MessageTool) Name() string       { return "collab_message_send" }
func (t *MessageTool) Description() string { return "Send a message to a team channel." }

func (t *MessageTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	channel, _ := input["channel"].(string)
	content, _ := input["content"].(string)
	if content == "" { return nil, fmt.Errorf("content is required") }

	return map[string]interface{}{
		"output":  fmt.Sprintf("Message sent to #%s", channel),
		"channel": channel,
		"content": content,
	}, nil
}

// MeetingTool summarizes a meeting.
type MeetingTool struct{}

func NewMeetingTool() *MeetingTool { return &MeetingTool{} }
func (t *MeetingTool) Name() string       { return "collab_meeting_summary" }
func (t *MeetingTool) Description() string { return "Summarize meeting notes." }

func (t *MeetingTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	notes, _ := input["notes"].(string)
	if notes == "" { return nil, fmt.Errorf("notes are required") }

	summary := fmt.Sprintf("Meeting Summary:\n- Total length: %d chars\n- Key points: (simulated)", len(notes))
	return map[string]interface{}{
		"output":  summary,
		"summary": summary,
	}, nil
}

func RegisterTools(tr *tools.ToolRegistry) {
	for _, t := range []tools.Tool{NewTaskTool(), NewWikiTool(), NewWikiSearchTool(), NewOkrTool(), NewMessageTool(), NewMeetingTool()} {
		tr.Register(t)
	}
}
