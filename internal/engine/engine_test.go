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

func TestProcessTurn_NoGateway(t *testing.T) {
	e := New(nil, nil)
	_, err := e.ProcessTurn(context.Background(), []llm.Message{
		{Role: "user", Content: "hello"},
	})
	if err == nil {
		t.Fatal("expected error with nil gateway")
	}
}

func TestTurnOrchestrator_New(t *testing.T) {
	o := NewTurnOrchestrator(nil, nil, nil)
	if o == nil {
		t.Fatal("expected non-nil orchestrator")
	}
}

func TestTurnOrchestrator_Execute_NoGateway(t *testing.T) {
	o := NewTurnOrchestrator(nil, nil, nil)
	_, _, err := o.Execute(context.Background(), "session-1", nil, "", nil)
	if err == nil {
		t.Fatal("expected error with nil gateway")
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

func (t *testTool) Name() string        { return t.name }
func (t *testTool) Description() string  { return t.desc }
func (t *testTool) Parameters() map[string]interface{} { return map[string]interface{}{} }
func (t *testTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"output": "done"}, nil
}

func TestToolResult(t *testing.T) {
	r := ToolResult{
		Name:   "test_tool",
		Input:  "{}",
		Output: "done",
	}
	if r.Name != "test_tool" {
		t.Fatalf("expected 'test_tool', got %q", r.Name)
	}
	if r.Error != "" {
		t.Fatalf("expected empty error, got %q", r.Error)
	}
}

func TestTurnResult(t *testing.T) {
	r := &TurnResult{
		Content: "Hello",
		Usage:   &llm.Usage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
	}
	if r.Content != "Hello" {
		t.Fatalf("expected 'Hello', got %q", r.Content)
	}
	if r.Usage.TotalTokens != 30 {
		t.Fatalf("expected 30 total tokens, got %d", r.Usage.TotalTokens)
	}
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

func TestEngine_ProcessTurn_EmptyMessages(t *testing.T) {
	e := New(nil, nil)
	_, err := e.ProcessTurn(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error with nil gateway and nil messages")
	}
}
