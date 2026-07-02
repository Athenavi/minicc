package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ── Task Planning ─────────────────────────────────────────────────────────

// PlanStep is a single step in a task plan.
type PlanStep struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"` // pending, running, completed, failed, skipped
	Tool        string `json:"tool,omitempty"`
	Dependency  string `json:"dependency,omitempty"` // step ID this depends on
	Result      string `json:"result,omitempty"`
}

// TaskPlan is a decomposition of a task into executable steps.
type TaskPlan struct {
	mu          sync.Mutex
	Task        string     `json:"task"`
	Steps       []PlanStep `json:"steps"`
	CurrentStep int        `json:"current_step"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// NewTaskPlan creates a new plan for a task.
func NewTaskPlan(task string) *TaskPlan {
	return &TaskPlan{
		Task:      task,
		Steps:     make([]PlanStep, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// AddStep appends a step to the plan.
func (tp *TaskPlan) AddStep(desc, tool, dependency string) *PlanStep {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	step := PlanStep{
		ID:          fmt.Sprintf("step_%d", len(tp.Steps)+1),
		Description: desc,
		Status:      "pending",
		Tool:        tool,
		Dependency:  dependency,
	}
	tp.Steps = append(tp.Steps, step)
	tp.UpdatedAt = time.Now()
	return &step
}

// NextStep returns the next pending step whose dependencies are met.
func (tp *TaskPlan) NextStep() *PlanStep {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	for i, step := range tp.Steps {
		if step.Status != "pending" {
			continue
		}
		// Check dependency
		if step.Dependency != "" {
			depMet := false
			for _, s := range tp.Steps {
				if s.ID == step.Dependency && s.Status == "completed" {
					depMet = true
					break
				}
			}
			if !depMet {
				continue
			}
		}
		tp.Steps[i].Status = "running"
		tp.CurrentStep = i
		tp.UpdatedAt = time.Now()
		return &tp.Steps[i]
	}
	return nil
}

// CompleteStep marks a step as completed with a result.
func (tp *TaskPlan) CompleteStep(stepID, result string) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	for i, step := range tp.Steps {
		if step.ID == stepID {
			tp.Steps[i].Status = "completed"
			tp.Steps[i].Result = result
			tp.UpdatedAt = time.Now()
			return
		}
	}
}

// FailStep marks a step as failed.
func (tp *TaskPlan) FailStep(stepID, result string) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	for i, step := range tp.Steps {
		if step.ID == stepID {
			tp.Steps[i].Status = "failed"
			tp.Steps[i].Result = result
			tp.UpdatedAt = time.Now()
			return
		}
	}
}

// Progress returns a summary of plan progress.
func (tp *TaskPlan) Progress() string {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	total := len(tp.Steps)
	completed := 0
	for _, s := range tp.Steps {
		if s.Status == "completed" {
			completed++
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Plan: %s (%d/%d steps completed)", tp.Task, completed, total))
	for _, s := range tp.Steps {
		icon := "⏳"
		switch s.Status {
		case "completed":
			icon = "✅"
		case "running":
			icon = "▶️"
		case "failed":
			icon = "❌"
		case "skipped":
			icon = "⏭️"
		}
		lines = append(lines, fmt.Sprintf("  %s %s: %s", icon, s.ID, s.Description))
		if s.Result != "" && s.Status == "completed" {
			short := s.Result
			if len(short) > 80 {
				short = short[:80] + "..."
			}
			lines = append(lines, fmt.Sprintf("     → %s", short))
		}
	}
	return strings.Join(lines, "\n")
}

// IsComplete returns true if all steps are done or failed.
func (tp *TaskPlan) IsComplete() bool {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	for _, s := range tp.Steps {
		if s.Status == "pending" || s.Status == "running" {
			return false
		}
	}
	return true
}

// ── Planner ───────────────────────────────────────────────────────────────

// Planner creates and manages task plans.
type Planner struct{}

func NewPlanner() *Planner { return &Planner{} }

// CreatePlan generates a plan for a task based on its description.
// In a full implementation, this would use the LLM. For now, it uses
// heuristic-based plan generation.
func (p *Planner) CreatePlan(task string, availableTools []string) *TaskPlan {
	plan := NewTaskPlan(task)

	// Heuristic: decompose based on task keywords
	taskLower := strings.ToLower(task)

	switch {
	case strings.Contains(taskLower, "analyze") || strings.Contains(taskLower, "examine"):
		plan.AddStep("Read input data", "read_file", "")
		plan.AddStep("Analyze content", "", "step_1")
		plan.AddStep("Generate report", "write_file", "step_2")

	case strings.Contains(taskLower, "generate") || strings.Contains(taskLower, "create"):
		plan.AddStep("Plan the output structure", "", "")
		plan.AddStep("Generate content with appropriate tools", "", "step_1")
		plan.AddStep("Save to media library", "media_create", "step_2")

	case strings.Contains(taskLower, "search") || strings.Contains(taskLower, "find"):
		plan.AddStep("Search for files", "search_files", "")
		plan.AddStep("Read matching files", "read_file", "step_1")
		plan.AddStep("Summarize findings", "", "step_2")

	case strings.Contains(taskLower, "summarize") || strings.Contains(taskLower, "summarise"):
		plan.AddStep("Read content", "read_file", "")
		plan.AddStep("Generate summary", "write_file", "step_1")

	default:
		plan.AddStep("Understand the request", "", "")
		plan.AddStep("Execute using appropriate tools", "", "step_1")
		plan.AddStep("Present results", "", "step_2")
	}

	return plan
}
