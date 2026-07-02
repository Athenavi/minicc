package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/athenavi/minicc/internal/tools"
)

// AgentInfo describes a registered agent type.
type AgentInfo struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Registry manages available agent types and routes tasks to them.
type Registry struct {
	mu      sync.RWMutex
	agents  map[string]AgentInfo
}

func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]AgentInfo),
	}
}

func (r *Registry) Register(agentType, name, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[agentType] = AgentInfo{
		Type:        agentType,
		Name:        name,
		Description: description,
	}
}

func (r *Registry) Get(agentType string) *AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[agentType]
	if !ok {
		return nil
	}
	return &a
}

func (r *Registry) List() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]AgentInfo, 0, len(r.agents))
	for _, a := range r.agents {
		list = append(list, a)
	}
	return list
}

// Route determines the best agent type for a given task description.
func (r *Registry) Route(task string) string {
	taskLower := strings.ToLower(task)

	// Keyword-based routing (same logic as Python version)
	rules := []struct {
		keywords []string
		agent    string
	}{
		{[]string{"代码", "code", "写", "修复", "debug", "文件", "python"}, "code"},
		{[]string{"搜索", "search", "知识", "文档", "查询", "rag", "知识库"}, "knowledge"},
		{[]string{"浏览器", "browser", "web", "点击", "填写", "表单", "rpa"}, "rpa"},
		{[]string{"mcp", "api", "tool", "工具"}, "tool"},
	}

	for _, rule := range rules {
		for _, kw := range rule.keywords {
			if strings.Contains(taskLower, kw) {
				return rule.agent
			}
		}
	}

	return "code" // default
}

// ── Tool implementations ─────────────────────────────────────────────────

// DispatchTool dispatches a task to the most suitable agent.
type DispatchTool struct {
	registry *Registry
}

func NewDispatchTool(registry *Registry) *DispatchTool {
	return &DispatchTool{registry: registry}
}

func (t *DispatchTool) Name() string        { return "agent_dispatch" }
func (t *DispatchTool) Description() string  { return "Dispatch a task to the most suitable agent based on task description." }
func (t *DispatchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"task":       map[string]interface{}{"type": "string", "description": "Task description to dispatch"},
		"agent_type": map[string]interface{}{"type": "string", "description": "Agent type: code, knowledge, rpa, tool (auto-detected if empty)"},
	}
}

func (t *DispatchTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	task, _ := input["task"].(string)
	if task == "" {
		return nil, fmt.Errorf("task is required")
	}

	agentType, _ := input["agent_type"].(string)
	if agentType == "" {
		agentType = t.registry.Route(task)
	}

	agent := t.registry.Get(agentType)
	if agent == nil {
		return nil, fmt.Errorf("no agent found for: %s", agentType)
	}

	return map[string]interface{}{
		"output": fmt.Sprintf("Dispatched to '%s'\n  Agent: %s\n  Task: %s",
			agent.Name, agent.Type, task),
		"agent_type": agent.Type,
		"task":       task,
	}, nil
}

// ListTool lists all available agents.
type ListTool struct {
	registry *Registry
}

func NewListTool(registry *Registry) *ListTool {
	return &ListTool{registry: registry}
}

func (t *ListTool) Name() string       { return "agent_list" }
func (t *ListTool) Description() string { return "List all available agents and their capabilities." }
func (t *ListTool) Parameters() map[string]interface{} { return map[string]interface{}{} }

func (t *ListTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	agents := t.registry.List()
	lines := []string{"Available agents:"}
	for _, a := range agents {
		lines = append(lines, fmt.Sprintf("  %s (%s) — %s", a.Name, a.Type, a.Description))
	}

	return map[string]interface{}{
		"output": strings.Join(lines, "\n"),
		"agents": agents,
	}, nil
}

// RegisterDefaults registers the default agents used by MiniCC.
func RegisterDefaults(registry *Registry) {
	registry.Register("code", "Code Agent", "编写、修改、分析代码")
	registry.Register("knowledge", "Knowledge Agent", "检索知识库和文档")
	registry.Register("rpa", "RPA Agent", "控制浏览器和桌面应用")
	registry.Register("tool", "Tool Agent", "调用 MCP 和外部 API")
}

// RegisterTools registers agent tools into the tool registry.
func RegisterTools(tr *tools.ToolRegistry, agentRegistry *Registry, sm *SessionManager) {
	tr.Register(NewDispatchTool(agentRegistry))
	tr.Register(NewListTool(agentRegistry))
	tr.Register(NewCodeAgentTool())
	tr.Register(NewAgentSessionTool(sm))
	tr.Register(NewAgentSessionListTool(sm))
}
