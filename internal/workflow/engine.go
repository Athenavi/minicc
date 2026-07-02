package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/athenavi/minicc/internal/tools"
)

// ─── Types ─────────────────────────────────────────────────────────────────

type Step struct {
	ID          string                 `json:"id"`
	Tool        string                 `json:"tool"`
	Params      map[string]interface{} `json:"params,omitempty"`
	Timeout     int                    `json:"timeout,omitempty"`   // seconds
	Retry       int                    `json:"retry,omitempty"`     // max retries
	RetryDelay  int                    `json:"retry_delay,omitempty"`
	Description string                 `json:"description,omitempty"`
}

type Condition struct {
	ID        string      `json:"id"`
	Condition string      `json:"condition"`
	Then      []*Step     `json:"then"`
	Else      []*Step     `json:"else,omitempty"`
}

type Loop struct {
	ID       string  `json:"id"`
	Over     string  `json:"over"`
	As       string  `json:"as"`
	Steps    []*Step `json:"steps"`
	MaxIter  int     `json:"max_iter,omitempty"`
}

type Trigger struct {
	Type  string `json:"type"` // cron / manual / event
	Cron  string `json:"cron,omitempty"`
	Event string `json:"event,omitempty"`
}

type Definition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Version     string                 `json:"version"`
	Trigger     *Trigger               `json:"trigger,omitempty"`
	Variables   map[string]interface{} `json:"variables,omitempty"`
	Steps       []interface{}          `json:"steps"` // *Step | *Condition | *Loop
	Timeout     int                    `json:"timeout,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
}

type StepResult struct {
	ID       string `json:"id"`
	Tool     string `json:"tool"`
	Status   string `json:"status"` // pending / running / success / failed / skipped
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	Duration int64  `json:"duration_ms"`
}

type ExecResult struct {
	WorkflowID string                `json:"workflow_id"`
	Name       string                `json:"name"`
	Status     string                `json:"status"` // completed / failed / cancelled / timeout
	Steps      []*StepResult         `json:"steps"`
	Error      string                `json:"error,omitempty"`
	Duration   float64               `json:"duration_seconds"`
	Output     string                `json:"output"`
	State      map[string]interface{} `json:"state,omitempty"`
}

// ─── Executor ──────────────────────────────────────────────────────────────

type Executor struct {
	mu        sync.RWMutex
	active    map[string]context.CancelFunc
	defs      map[string]*Definition
	history   map[string]*ExecResult
	counter   int64
}

func NewExecutor() *Executor {
	return &Executor{
		active:  make(map[string]context.CancelFunc),
		defs:    make(map[string]*Definition),
		history: make(map[string]*ExecResult),
	}
}

func (ex *Executor) CreateDef(name, desc string, steps []interface{}) string {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	ex.counter++
	id := fmt.Sprintf("wf_%d_%d", time.Now().Unix(), ex.counter)
	ex.defs[id] = &Definition{
		Name: name, Description: desc, Version: "1.0",
		Steps: steps, Timeout: 3600,
	}
	return id
}

func (ex *Executor) GetDef(id string) *Definition {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	return ex.defs[id]
}

func (ex *Executor) ListDefs() []map[string]interface{} {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	var list []map[string]interface{}
	for id, d := range ex.defs {
		list = append(list, map[string]interface{}{
			"id": id, "name": d.Name, "steps": len(d.Steps), "version": d.Version,
		})
	}
	return list
}

func (ex *Executor) Execute(def *Definition) *ExecResult {
	ex.mu.Lock()
	ex.counter++
	wfID := fmt.Sprintf("exec_%d_%d", time.Now().Unix(), ex.counter)
	ex.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(def.Timeout)*time.Second)
	ex.mu.Lock()
	ex.active[wfID] = cancel
	ex.mu.Unlock()

	defer func() {
		ex.mu.Lock()
		delete(ex.active, wfID)
		ex.mu.Unlock()
	}()

	result := &ExecResult{
		WorkflowID: wfID, Name: def.Name, Status: "running",
	}
	var outputLines []string
	outputLines = append(outputLines, fmt.Sprintf("[workflow] Starting: %s (ID: %s)", def.Name, wfID))
	state := make(map[string]interface{})
	for k, v := range def.Variables {
		state[k] = v
	}

	startTime := time.Now()
	stepResults := ex.executeSteps(ctx, def.Steps, state, &outputLines, 0)

	result.Steps = stepResults
	hasError := false
	for _, s := range stepResults {
		if s.Status == "failed" {
			hasError = true
			result.Error = s.Error
			break
		}
	}

	if ctx.Err() != nil {
		result.Status = "timeout"
		outputLines = append(outputLines, "[workflow] Timed out")
	} else if hasError {
		result.Status = "failed"
	} else {
		result.Status = "completed"
		outputLines = append(outputLines, "[workflow] Completed")
	}

	result.Duration = time.Since(startTime).Seconds()
	result.Output = strings.Join(outputLines, "\n")
	result.State = state

	ex.mu.Lock()
	ex.history[wfID] = result
	if len(ex.history) > 100 {
		for k := range ex.history {
			if len(ex.history) <= 100 {
				break
			}
			delete(ex.history, k)
		}
	}
	ex.mu.Unlock()

	slog.Info("workflow executed", "id", wfID, "name", def.Name, "status", result.Status, "duration", result.Duration)
	return result
}

func (ex *Executor) Cancel(wfID string) bool {
	ex.mu.RLock()
	cancel, ok := ex.active[wfID]
	ex.mu.RUnlock()
	if ok {
		cancel()
		return true
	}
	return false
}

func (ex *Executor) GetResult(wfID string) *ExecResult {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	return ex.history[wfID]
}

func (ex *Executor) Stats() map[string]interface{} {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	completed, failed := 0, 0
	for _, r := range ex.history {
		switch r.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		}
	}
	return map[string]interface{}{
		"definitions":  len(ex.defs),
		"executions":   len(ex.history),
		"completed":    completed,
		"failed":       failed,
		"active":       len(ex.active),
	}
}

func (ex *Executor) executeSteps(ctx context.Context, steps []interface{}, state map[string]interface{}, output *[]string, depth int) []*StepResult {
	var results []*StepResult
	indent := strings.Repeat("  ", depth+1)

	for _, s := range steps {
		if ctx.Err() != nil {
			break
		}

		switch step := s.(type) {
		case *Step:
			r := ex.executeStep(ctx, step, state, output, indent)
			results = append(results, r)

		case *Condition:
			cond := step
			resolved := resolveVariables(cond.Condition, state)
			isTrue := strings.EqualFold(resolved, "true") || strings.EqualFold(resolved, "success")
			*output = append(*output, fmt.Sprintf("%s[condition] %s -> %v", indent, cond.Condition, isTrue))

			branch := cond.Then
			if !isTrue && cond.Else != nil {
				branch = cond.Else
			}
			branchSteps := make([]interface{}, len(branch))
			for i, bs := range branch {
				branchSteps[i] = bs
			}
			childResults := ex.executeSteps(ctx, branchSteps, state, output, depth+1)
			results = append(results, childResults...)

		case *Loop:
			loop := step
			itemsStr := resolveVariables(loop.Over, state)
			items := strings.Split(itemsStr, ",")
			maxIter := loop.MaxIter
			if maxIter <= 0 {
				maxIter = 100
			}
			if len(items) > maxIter {
				items = items[:maxIter]
			}
			*output = append(*output, fmt.Sprintf("%s[loop] %s (%d items, as '%s')", indent, loop.Over, len(items), loop.As))

			for _, item := range items {
				if ctx.Err() != nil {
					break
				}
				state[loop.As] = strings.TrimSpace(item)
				loopSteps := make([]interface{}, len(loop.Steps))
				for i, ls := range loop.Steps {
					loopSteps[i] = ls
				}
				childResults := ex.executeSteps(ctx, loopSteps, state, output, depth+1)
				results = append(results, childResults...)
			}
		}
	}
	return results
}

func (ex *Executor) executeStep(ctx context.Context, step *Step, state map[string]interface{}, output *[]string, indent string) *StepResult {
	r := &StepResult{ID: step.ID, Tool: step.Tool, Status: "running"}
	start := time.Now()

	params := resolveParams(step.Params, state)
	_ = params // used in future tool execution
	*output = append(*output, fmt.Sprintf("%s[step] %s: %s", indent, step.ID, step.Tool))
	if step.Description != "" {
		*output = append(*output, fmt.Sprintf("%s  %s", indent, step.Description))
	}

	timeout := step.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	stepCtx, stepCancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer stepCancel()

	retries := step.Retry
	if retries <= 0 {
		retries = 1
	}

	var lastErr error
	for attempt := 0; attempt < retries; attempt++ {
		if attempt > 0 {
			*output = append(*output, fmt.Sprintf("%s  retry %d/%d", indent, attempt+1, retries))
			time.Sleep(time.Duration(step.RetryDelay) * time.Second)
		}

		select {
		case <-stepCtx.Done():
			r.Status = "failed"
			r.Error = "step timed out"
			*output = append(*output, fmt.Sprintf("%s  timeout after %ds", indent, timeout))
			r.Duration = time.Since(start).Milliseconds()
			return r
		default:
		}

		// Simulate tool execution (Phase 2+ will use real tool registry)
		*output = append(*output, fmt.Sprintf("%s  executed: %s", indent, step.Tool))
		state[step.ID] = map[string]interface{}{
			"result":  fmt.Sprintf("simulated: %s", step.Tool),
			"success": true,
		}
		r.Status = "success"
		r.Output = fmt.Sprintf("simulated: %s", step.Tool)
		lastErr = nil
		break
	}

	if lastErr != nil {
		r.Status = "failed"
		r.Error = lastErr.Error()
		*output = append(*output, fmt.Sprintf("%s  failed: %s", indent, lastErr))
	}

	r.Duration = time.Since(start).Milliseconds()
	return r
}

// ─── Variable Helpers ──────────────────────────────────────────────────────

func resolveVariables(template string, state map[string]interface{}) string {
	result := template
	for k, v := range state {
		placeholder := fmt.Sprintf("{{.%s}}", k)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", v))
	}
	// Date builtins
	now := time.Now()
	result = strings.ReplaceAll(result, "{{.date.YYYYMMDD}}", now.Format("20060102"))
	result = strings.ReplaceAll(result, "{{.date.YYYY-MM-DD}}", now.Format("2006-01-02"))
	result = strings.ReplaceAll(result, "{{.date.HHmmss}}", now.Format("150405"))
	return result
}

func resolveParams(params map[string]interface{}, state map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range params {
		switch val := v.(type) {
		case string:
			result[k] = resolveVariables(val, state)
		case map[string]interface{}:
			result[k] = resolveParams(val, state)
		default:
			result[k] = v
		}
	}
	return result
}

// ─── Tools ─────────────────────────────────────────────────────────────────

type CreateTool struct{ executor *Executor }
func NewCreateTool(ex *Executor) *CreateTool { return &CreateTool{executor: ex} }
func (t *CreateTool) Name() string { return "workflow_create" }
func (t *CreateTool) Description() string { return "Create a workflow definition." }
func (t *CreateTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	name, _ := input["name"].(string)
	if name == "" { return nil, fmt.Errorf("name is required") }
	desc, _ := input["description"].(string)
	var steps []interface{}
	if s, ok := input["steps"].([]interface{}); ok {
		for _, si := range s {
			if sm, ok := si.(map[string]interface{}); ok {
				step := &Step{
					ID: getStr(sm, "id"), Tool: getStr(sm, "tool"),
					Timeout: getInt(sm, "timeout", 60), Retry: getInt(sm, "retry", 0),
				}
				if p, ok := sm["params"].(map[string]interface{}); ok {
					step.Params = p
				}
				steps = append(steps, step)
			}
		}
	}
	id := t.executor.CreateDef(name, desc, steps)
	return map[string]interface{}{
		"output": fmt.Sprintf("Workflow created: %s (ID: %s)", name, id),
		"id":     id, "steps": len(steps),
	}, nil
}

type RunTool struct{ executor *Executor }
func NewRunTool(ex *Executor) *RunTool { return &RunTool{executor: ex} }
func (t *RunTool) Name() string { return "workflow_run" }
func (t *RunTool) Description() string { return "Execute a workflow by ID." }
func (t *RunTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	id, _ := input["workflow_id"].(string)
	def := t.executor.GetDef(id)
	if def == nil {
		return nil, fmt.Errorf("workflow not found: %s", id)
	}
	result := t.executor.Execute(def)
	return map[string]interface{}{
		"output":   result.Output,
		"status":   result.Status,
		"duration": result.Duration,
		"id":       result.WorkflowID,
	}, nil
}

type ListTool struct{ executor *Executor }
func NewListTool(ex *Executor) *ListTool { return &ListTool{executor: ex} }
func (t *ListTool) Name() string { return "workflow_list" }
func (t *ListTool) Description() string { return "List all workflow definitions." }
func (t *ListTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	defs := t.executor.ListDefs()
	stats := t.executor.Stats()
	return map[string]interface{}{
		"output": fmt.Sprintf("Definitions: %d\nStats: %v", len(defs), stats),
		"definitions": defs,
	}, nil
}

type GetTool struct{ executor *Executor }
func NewGetTool(ex *Executor) *GetTool { return &GetTool{executor: ex} }
func (t *GetTool) Name() string { return "workflow_get" }
func (t *GetTool) Description() string { return "Get workflow execution result." }
func (t *GetTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	id, _ := input["workflow_id"].(string)
	r := t.executor.GetResult(id)
	if r == nil { return nil, fmt.Errorf("execution not found: %s", id) }
	return map[string]interface{}{
		"output": r.Output, "status": r.Status,
		"duration": r.Duration, "steps": len(r.Steps),
	}, nil
}

type CancelTool struct{ executor *Executor }
func NewCancelTool(ex *Executor) *CancelTool { return &CancelTool{executor: ex} }
func (t *CancelTool) Name() string { return "workflow_cancel" }
func (t *CancelTool) Description() string { return "Cancel a running workflow." }
func (t *CancelTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	id, _ := input["workflow_id"].(string)
	ok := t.executor.Cancel(id)
	if !ok { return nil, fmt.Errorf("workflow not found or already completed: %s", id) }
	return map[string]interface{}{"output": fmt.Sprintf("Cancelled: %s", id)}, nil
}

type StatsTool struct{ executor *Executor }
func NewStatsTool(ex *Executor) *StatsTool { return &StatsTool{executor: ex} }
func (t *StatsTool) Name() string { return "workflow_stats" }
func (t *StatsTool) Description() string { return "Get workflow engine statistics." }
func (t *StatsTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	stats := t.executor.Stats()
	return map[string]interface{}{"output": fmt.Sprintf("%v", stats), "stats": stats}, nil
}

func RegisterTools(tr *tools.ToolRegistry, ex *Executor) {
	tr.Register(NewCreateTool(ex))
	tr.Register(NewRunTool(ex))
	tr.Register(NewListTool(ex))
	tr.Register(NewGetTool(ex))
	tr.Register(NewCancelTool(ex))
	tr.Register(NewStatsTool(ex))
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok { return v }
	return ""
}
func getInt(m map[string]interface{}, key string, def int) int {
	switch v := m[key].(type) {
	case float64: return int(v)
	case int: return v
	default: return def
	}
}
