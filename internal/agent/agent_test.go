package agent

import (
	"testing"

	"github.com/athenavi/minicc/internal/tools"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(r.List()) != 0 {
		t.Fatalf("expected empty registry, got %d", len(r.List()))
	}
}

func TestRegisterAndList(t *testing.T) {
	r := NewRegistry()
	r.Register("code", "Code Agent", "Writes and analyzes code")
	agents := r.List()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Type != "code" {
		t.Fatalf("expected 'code', got %q", agents[0].Type)
	}
}

func TestRegisterDefaults(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	agents := r.List()
	if len(agents) == 0 {
		t.Fatal("expected default agents to be registered")
	}
}

func TestGetAgent(t *testing.T) {
	r := NewRegistry()
	r.Register("code", "Code Agent", "Writes code")
	a := r.Get("code")
	if a == nil {
		t.Fatal("expected to find agent")
	}
	if a.Type != "code" {
		t.Fatalf("expected 'code', got %q", a.Type)
	}
}

func TestGetAgent_Missing(t *testing.T) {
	r := NewRegistry()
	a := r.Get("nonexistent")
	if a != nil {
		t.Fatal("expected nil for missing agent")
	}
}

func TestNewSessionManager(t *testing.T) {
	sm := NewSessionManager()
	if sm == nil {
		t.Fatal("expected non-nil session manager")
	}
}

func TestSessionManager_Create(t *testing.T) {
	sm := NewSessionManager()
	s := sm.Create("test-session", "do something")
	if s == nil {
		t.Fatal("expected non-nil session")
	}
	if s.Name != "test-session" {
		t.Fatalf("expected 'test-session', got %q", s.Name)
	}
	if s.Status != "pending" {
		t.Fatalf("expected status 'pending', got %q", s.Status)
	}
}

func TestSessionManager_Get(t *testing.T) {
	sm := NewSessionManager()
	created := sm.Create("test", "task")
	got := sm.Get(created.ID)
	if got == nil {
		t.Fatal("expected to find session")
	}
	if got.ID != created.ID {
		t.Fatal("expected same ID")
	}
}

func TestSessionManager_Get_Missing(t *testing.T) {
	sm := NewSessionManager()
	got := sm.Get("nonexistent")
	if got != nil {
		t.Fatal("expected nil for missing session")
	}
}

func TestSessionManager_List(t *testing.T) {
	sm := NewSessionManager()
	sm.Create("a", "task a")
	sm.Create("b", "task b")
	sessions := sm.List()
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestSessionManager_UpdateStatus(t *testing.T) {
	sm := NewSessionManager()
	s := sm.Create("test", "task")
	sm.UpdateStatus(s.ID, "running", "")
	updated := sm.Get(s.ID)
	if updated.Status != "running" {
		t.Fatalf("expected status 'running', got %q", updated.Status)
	}
}

func TestNewAgentRuntime(t *testing.T) {
	r := NewAgentRuntime(nil, nil, nil)
	if r == nil {
		t.Fatal("expected non-nil runtime")
	}
}

func TestNewCodeAgentTool(t *testing.T) {
	tool := NewCodeAgentTool()
	if tool.Name() != "code_agent" {
		t.Fatalf("expected 'code_agent', got %q", tool.Name())
	}
}

func TestNewAgentSessionTool(t *testing.T) {
	sm := NewSessionManager()
	tool := NewAgentSessionTool(sm)
	if tool.Name() != "agent_session_create" {
		t.Fatalf("expected 'agent_session_create', got %q", tool.Name())
	}
}

func TestNewAgentSessionListTool(t *testing.T) {
	sm := NewSessionManager()
	tool := NewAgentSessionListTool(sm)
	if tool.Name() != "agent_session_list" {
		t.Fatalf("expected 'agent_session_list', got %q", tool.Name())
	}
}

func TestRegisterTools(t *testing.T) {
	reg := tools.NewToolRegistry()
	sm := NewSessionManager()
	agentReg := NewRegistry()
	RegisterDefaults(agentReg)
	RegisterTools(reg, agentReg, sm)
	if reg.Get("code_agent") == nil {
		t.Fatal("expected code_agent to be registered")
	}
	if reg.Get("agent_session_create") == nil {
		t.Fatal("expected agent_session_create to be registered")
	}
}
