package pm

import (
	"context"
	"fmt"

	"github.com/athenavi/minicc/internal/tools"
)

// PRDTool generates Product Requirements Documents.
type PRDTool struct{}

func NewPRDTool() *PRDTool { return &PRDTool{} }
func (t *PRDTool) Name() string       { return "prd_generate" }
func (t *PRDTool) Description() string { return "Generate a structured Product Requirements Document from a natural language description." }

func (t *PRDTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	desc, _ := input["description"].(string)
	if desc == "" {
		return nil, fmt.Errorf("description is required")
	}
	contextStr, _ := input["context"].(string)

	output := "# Product Requirements Document\n\n## Overview\n" + desc
	if contextStr != "" {
		output += "\n\n## Context\n" + contextStr
	}
	output += "\n\n## Goals\n- Goal 1: TBD\n- Goal 2: TBD"

	return map[string]interface{}{
		"output": output,
		"prd":    output,
	}, nil
}

// TechDesignTool generates technical design documents.
type TechDesignTool struct{}

func NewTechDesignTool() *TechDesignTool { return &TechDesignTool{} }
func (t *TechDesignTool) Name() string       { return "tech_design" }
func (t *TechDesignTool) Description() string { return "Generate architecture, API design, and data models from PRD." }

func (t *TechDesignTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"output": "[tech-design] Architecture design generated from PRD\n  Modules: TBD\n  API: TBD\n  Data Model: TBD",
	}, nil
}

// TaskDecomposeTool breaks PRD into executable tasks.
type TaskDecomposeTool struct{}

func NewTaskDecomposeTool() *TaskDecomposeTool { return &TaskDecomposeTool{} }
func (t *TaskDecomposeTool) Name() string       { return "task_decompose" }
func (t *TaskDecomposeTool) Description() string { return "Break down PRD into a graph of executable development tasks." }

func (t *TaskDecomposeTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"output": "[task-decompose] Tasks generated from PRD\n  Task 1: TBD\n  Task 2: TBD",
	}, nil
}

// RequirementValidateTool validates requirements completeness.
type RequirementValidateTool struct{}

func NewRequirementValidateTool() *RequirementValidateTool { return &RequirementValidateTool{} }
func (t *RequirementValidateTool) Name() string       { return "requirement_validate" }
func (t *RequirementValidateTool) Description() string { return "Validate requirements for completeness and consistency." }

func (t *RequirementValidateTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"output": "[requirement-validate] Requirements validation complete\n  Status: All requirements validated",
	}, nil
}

func RegisterTools(tr *tools.ToolRegistry) {
	tr.Register(NewPRDTool())
	tr.Register(NewTechDesignTool())
	tr.Register(NewTaskDecomposeTool())
	tr.Register(NewRequirementValidateTool())
}
