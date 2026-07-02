package graph

import (
	"context"
	"testing"

	"github.com/athenavi/minicc/internal/tools"
)

func TestNewBuilder(t *testing.T) {
	b := NewBuilder()
	if b == nil {
		t.Fatal("expected non-nil builder")
	}
}

func TestBuilder_AddNode(t *testing.T) {
	b := NewBuilder()
	b.AddNode("node-1", "test", NodeLLM, map[string]interface{}{"model": "gpt-4"})
	g, cr := b.Compile()
	if g == nil {
		t.Fatal("expected non-nil graph")
	}
	if !cr.Valid {
		t.Fatalf("expected valid, got errors: %v", cr.Errors)
	}
	if len(g.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(g.Nodes))
	}
	if g.Nodes[0].ID != "node-1" {
		t.Fatalf("expected 'node-1', got %q", g.Nodes[0].ID)
	}
}

func TestBuilder_AddEdge(t *testing.T) {
	b := NewBuilder()
	b.AddNode("a", "start", NodeInput, nil)
	b.AddNode("b", "end", NodeOutput, nil)
	b.AddEdge("a", "b", "", "")
	g, cr := b.Compile()

	if !cr.Valid {
		t.Fatalf("expected valid, got errors: %v", cr.Errors)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}
	if g.Edges[0].SourceID != "a" || g.Edges[0].TargetID != "b" {
		t.Fatalf("expected edge a→b, got %s→%s", g.Edges[0].SourceID, g.Edges[0].TargetID)
	}
}

func TestBuilder_SetEntryPoint(t *testing.T) {
	b := NewBuilder()
	b.AddNode("start", "entry", NodeInput, nil)
	b.SetEntryPoint("start")
	g, cr := b.Compile()
	if !cr.Valid {
		t.Fatalf("expected valid, got errors: %v", cr.Errors)
	}
	if g.EntryPoint != "start" {
		t.Fatalf("expected 'start', got %q", g.EntryPoint)
	}
}

func TestNewExecutor(t *testing.T) {
	e := NewExecutor()
	if e == nil {
		t.Fatal("expected non-nil executor")
	}
}

func TestExecutor_Execute_EmptyGraph(t *testing.T) {
	e := NewExecutor()
	b := NewBuilder()
	b.AddNode("n1", "node", NodeInput, nil)
	g, _ := b.Compile()
	state, events := e.Execute(context.Background(), g, nil)
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if events == nil {
		t.Fatal("expected non-nil events")
	}
}

func TestBuilder_Compile_Valid(t *testing.T) {
	b := NewBuilder()
	b.AddNode("in", "input", NodeInput, nil)
	b.AddNode("llm", "process", NodeLLM, nil)
	b.AddNode("out", "output", NodeOutput, nil)
	b.AddEdge("in", "llm", "", "")
	b.AddEdge("llm", "out", "", "")
	b.SetEntryPoint("in")

	g, cr := b.Compile()
	if !cr.Valid {
		t.Fatalf("expected valid graph, got errors: %v", cr.Errors)
	}
	if len(cr.TopologicalOrder) == 0 {
		t.Fatal("expected topological order")
	}
	_ = g
}

func TestBuilder_Compile_Empty(t *testing.T) {
	b := NewBuilder()
	g, cr := b.Compile()
	if cr.Valid {
		t.Fatal("expected invalid for empty graph")
	}
	if g != nil {
		t.Fatal("expected nil graph for empty build")
	}
}

func TestRegisterTools(t *testing.T) {
	reg := tools.NewToolRegistry()
	RegisterTools(reg)
	if reg.Get("graph_create") == nil {
		t.Fatal("expected graph_create to be registered")
	}
	if reg.Get("graph_run") == nil {
		t.Fatal("expected graph_run to be registered")
	}
}

func TestNodeTypes(t *testing.T) {
	if NodeInput != "input" {
		t.Fatalf("expected 'input', got %q", NodeInput)
	}
	if NodeOutput != "output" {
		t.Fatalf("expected 'output', got %q", NodeOutput)
	}
	if NodeLLM != "llm" {
		t.Fatalf("expected 'llm', got %q", NodeLLM)
	}
}
