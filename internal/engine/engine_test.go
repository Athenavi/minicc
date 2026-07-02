package engine

import (
	"context"
	"testing"

	"github.com/athenavi/minicc/internal/llm"
	"github.com/athenavi/minicc/internal/tools"
)

func TestNew(t *testing.T) {
	e := New(nil, nil)
	if e == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestBuildToolDefs_Empty(t *testing.T) {
	registry := tools.NewToolRegistry()
	defs := BuildToolDefs(registry)
	if len(defs) != 0 {
		t.Fatalf("expected 0 defs, got %d", len(defs))
	}
}

func TestExecuteTask_NoGateway(t *testing.T) {
	e := New(nil, nil)
	_, _, err := e.ExecuteTask(context.Background(), "hello", "session-1", nil)
	if err == nil {
		t.Fatal("expected error with nil gateway")
	}
}

func TestSystemPrompt(t *testing.T) {
	e := New(nil, nil)
	prompt := e.SystemPrompt()
	if prompt == "" {
		t.Fatal("expected non-empty system prompt")
	}
}

func TestTurnOrchestrator_New(t *testing.T) {
	o := NewTurnOrchestrator(nil, nil, nil)
	if o == nil {
		t.Fatal("expected non-nil orchestrator")
	}
}

func TestBuildToolDefs_WithTools(t *testing.T) {
	registry := tools.NewToolRegistry()

	registry.Register(&testTool{name: "read_file", desc: "Read a file"})
	registry.Register(&testTool{name: "write_file", desc: "Write a file"})

	defs := BuildToolDefs(registry)
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}

	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["read_file"] {
		t.Fatal("expected read_file in defs")
	}
	if !names["write_file"] {
		t.Fatal("expected write_file in defs")
	}
}

type testTool struct {
	name string
	desc string
}

func (t *testTool) Name() string                     { return t.name }
func (t *testTool) Description() string               { return t.desc }
func (t *testTool) Parameters() map[string]interface{} { return map[string]interface{}{} }
func (t *testTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"output": "done"}, nil
}

func TestBuildToolDefs_Description(t *testing.T) {
	registry := tools.NewToolRegistry()
	registry.Register(&testTool{name: "test", desc: "A test tool"})

	defs := BuildToolDefs(registry)
	if len(defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs))
	}
	if defs[0].Description != "A test tool" {
		t.Fatalf("expected 'A test tool', got %q", defs[0].Description)
	}
}

func TestExecuteTask_WithHistory(t *testing.T) {
	e := New(nil, nil)
	history := []llm.Message{
		{Role: "user", Content: "previous message"},
		{Role: "assistant", Content: "previous response"},
	}
	_, _, err := e.ExecuteTask(context.Background(), "new task", "session-1", history)
	if err == nil {
		t.Fatal("expected error with nil gateway")
	}
}
