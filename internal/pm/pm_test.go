package pm

import (
	"context"
	"testing"

	"github.com/athenavi/minicc/internal/tools"
)

func TestNewPRDTool(t *testing.T) {
	tool := NewPRDTool()
	if tool.Name() != "prd_generate" {
		t.Fatalf("expected 'prd_generate', got %q", tool.Name())
	}
}

func TestPRDTool_Execute_NoDescription(t *testing.T) {
	tool := NewPRDTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty description")
	}
}

func TestPRDTool_Execute_WithDescription(t *testing.T) {
	tool := NewPRDTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"description": "Build a login system",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	output, _ := result["output"].(string)
	if output == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestNewTechDesignTool(t *testing.T) {
	tool := NewTechDesignTool()
	if tool.Name() != "tech_design" {
		t.Fatalf("expected 'tech_design', got %q", tool.Name())
	}
}

func TestTechDesignTool_Execute(t *testing.T) {
	tool := NewTechDesignTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	output, _ := result["output"].(string)
	if output == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestNewTaskDecomposeTool(t *testing.T) {
	tool := NewTaskDecomposeTool()
	if tool.Name() != "task_decompose" {
		t.Fatalf("expected 'task_decompose', got %q", tool.Name())
	}
}

func TestTaskDecomposeTool_Execute(t *testing.T) {
	tool := NewTaskDecomposeTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	output, _ := result["output"].(string)
	if output == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestNewRequirementValidateTool(t *testing.T) {
	tool := NewRequirementValidateTool()
	if tool.Name() != "requirement_validate" {
		t.Fatalf("expected 'requirement_validate', got %q", tool.Name())
	}
}

func TestRequirementValidateTool_Execute(t *testing.T) {
	tool := NewRequirementValidateTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	output, _ := result["output"].(string)
	if output == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestRegisterTools(t *testing.T) {
	registry := tools.NewToolRegistry()
	RegisterTools(registry)

	expected := []string{"prd_generate", "tech_design", "task_decompose", "requirement_validate"}
	for _, name := range expected {
		if registry.Get(name) == nil {
			t.Fatalf("expected %q to be registered", name)
		}
	}
}
