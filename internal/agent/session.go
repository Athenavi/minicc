package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Session represents a sub-agent session.
type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Task      string    `json:"task"`
	Status    string    `json:"status"` // pending / running / completed / error
	Result    string    `json:"result,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SessionManager manages sub-agent sessions.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	counter  int64
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

func (sm *SessionManager) Create(name, task string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.counter++
	s := &Session{
		ID:        fmt.Sprintf("session_%d_%d", time.Now().Unix(), sm.counter),
		Name:      name,
		Task:      task,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	sm.sessions[s.ID] = s
	return s
}

func (sm *SessionManager) Get(id string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[id]
}

func (sm *SessionManager) List() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	list := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		list = append(list, s)
	}
	return list
}

func (sm *SessionManager) UpdateStatus(id, status, result string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[id]; ok {
		s.Status = status
		s.Result = result
		s.UpdatedAt = time.Now()
	}
}

// CodeAgentTool handles code-related tasks.
type CodeAgentTool struct{}

func NewCodeAgentTool() *CodeAgentTool {
	return &CodeAgentTool{}
}

func (t *CodeAgentTool) Name() string       { return "code_agent" }
func (t *CodeAgentTool) Description() string { return "Execute a code-related task — write, modify, analyze, debug code." }

func (t *CodeAgentTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	task, _ := input["task"].(string)
	if task == "" {
		return nil, fmt.Errorf("task is required")
	}

	filePath, _ := input["file_path"].(string)
	language, _ := input["language"].(string)

	var parts []string
	parts = append(parts, fmt.Sprintf("Code task received: %s", truncate(task, 100)))
	if filePath != "" {
		parts = append(parts, fmt.Sprintf("  File: %s", filePath))
	} else {
		parts = append(parts, "  File: auto")
	}
	if language != "" {
		parts = append(parts, fmt.Sprintf("  Language: %s", language))
	} else {
		parts = append(parts, "  Language: auto")
	}
	parts = append(parts, "  (executed via sub-agent)")

	return map[string]interface{}{
		"output":   strings.Join(parts, "\n"),
		"task":     task,
		"file":     filePath,
		"language": language,
	}, nil
}

// AgentSessionTool creates a sub-agent session.
type AgentSessionTool struct {
	sm *SessionManager
}

func NewAgentSessionTool(sm *SessionManager) *AgentSessionTool {
	return &AgentSessionTool{sm: sm}
}

func (t *AgentSessionTool) Name() string       { return "agent_session_create" }
func (t *AgentSessionTool) Description() string { return "Create a sub-agent session for an independent task." }

func (t *AgentSessionTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	name, _ := input["name"].(string)
	task, _ := input["task"].(string)
	if name == "" || task == "" {
		return nil, fmt.Errorf("name and task are required")
	}

	s := t.sm.Create(name, task)
	return map[string]interface{}{
		"output":     fmt.Sprintf("Session created: %s (%s)", s.Name, s.ID),
		"session_id": s.ID,
		"status":     s.Status,
	}, nil
}

// AgentSessionListTool lists all sub-agent sessions.
type AgentSessionListTool struct {
	sm *SessionManager
}

func NewAgentSessionListTool(sm *SessionManager) *AgentSessionListTool {
	return &AgentSessionListTool{sm: sm}
}

func (t *AgentSessionListTool) Name() string       { return "agent_session_list" }
func (t *AgentSessionListTool) Description() string { return "List all sub-agent sessions." }

func (t *AgentSessionListTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	sessions := t.sm.List()
	lines := []string{fmt.Sprintf("Sessions (%d):", len(sessions))}
	for _, s := range sessions {
		lines = append(lines, fmt.Sprintf("  [%s] %s — %s (%s)", s.Status, s.Name, s.Task[:min(len(s.Task), 60)], s.ID))
	}
	return map[string]interface{}{
		"output":   strings.Join(lines, "\n"),
		"sessions": sessions,
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
