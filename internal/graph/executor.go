package graph

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/athenavi/minicc/internal/tools"
)

type NodeResult struct {
	NodeID     string      `json:"node_id"`
	Status     string      `json:"status"` // completed / error / skipped
	Output     string      `json:"output,omitempty"`
	Error      string      `json:"error,omitempty"`
	DurationMs int64       `json:"duration_ms"`
}

type GraphEvent struct {
	Type     string      `json:"type"` // node_started / node_completed / node_error / done
	NodeID   string      `json:"node_id,omitempty"`
	Output   string      `json:"output,omitempty"`
	Error    string      `json:"error,omitempty"`
	Progress float64     `json:"progress,omitempty"`
}

type Executor struct {
	mu      sync.RWMutex
	results map[string]*NodeResult
	events  []GraphEvent
}

func NewExecutor() *Executor {
	return &Executor{
		results: make(map[string]*NodeResult),
	}
}

func (ex *Executor) Execute(ctx context.Context, graph *StateGraph, initialState map[string]interface{}) (map[string]interface{}, []GraphEvent) {
	state := make(map[string]interface{})
	for k, v := range initialState {
		state[k] = v
	}

	ex.mu.Lock()
	ex.results = make(map[string]*NodeResult)
	ex.events = nil
	ex.mu.Unlock()

	nodeMap := make(map[string]GraphNode)
	for _, n := range graph.Nodes {
		nodeMap[n.ID] = n
	}

	// Build adjacency
	adj := make(map[string][]string)
	inDeg := make(map[string]int)
	for _, n := range graph.Nodes {
		adj[n.ID] = nil
		inDeg[n.ID] = 0
	}
	for _, e := range graph.Edges {
		adj[e.SourceID] = append(adj[e.SourceID], e.TargetID)
		inDeg[e.TargetID]++
	}

	// Start with nodes that have no dependencies
	var ready []string
	for id, d := range inDeg {
		if d == 0 {
			ready = append(ready, id)
		}
	}

	completed := make(map[string]bool)

	for len(ready) > 0 {
		// Execute ready nodes (sequentially for simplicity)
		var nextReady []string

		for _, nodeID := range ready {
			if completed[nodeID] {
				continue
			}

			node := nodeMap[nodeID]

			ex.addEvent(GraphEvent{Type: "node_started", NodeID: nodeID})

			start := time.Now()
			result := ex.executeNode(node, state)
			result.DurationMs = time.Since(start).Milliseconds()

			ex.mu.Lock()
			ex.results[nodeID] = &result
			ex.mu.Unlock()

			if result.Status == "completed" {
				state[nodeID] = result.Output
				completed[nodeID] = true
				ex.addEvent(GraphEvent{
					Type:   "node_completed",
					NodeID: nodeID,
					Output: result.Output,
				})
			} else {
				ex.addEvent(GraphEvent{
					Type:   "node_error",
					NodeID: nodeID,
					Error:  result.Error,
				})
			}

			// Check conditions and update downstream readiness
			for _, targetID := range adj[nodeID] {
				if !completed[targetID] {
					// Check edge conditions
					shouldRun := true
					for _, e := range graph.Edges {
						if e.SourceID == nodeID && e.TargetID == targetID && e.Condition != "" {
							if !evaluateCondition(e.Condition, result.Output) {
								shouldRun = false
								break
							}
						}
					}
					if shouldRun {
						// Check all predecessors are done
						allDone := true
						for _, e2 := range graph.Edges {
							if e2.TargetID == targetID {
								if !completed[e2.SourceID] {
									allDone = false
									break
								}
							}
						}
						if allDone && !completed[targetID] {
							nextReady = append(nextReady, targetID)
						}
					}
				}
			}
		}

		ready = nextReady
	}

	ex.addEvent(GraphEvent{
		Type:     "done",
		Progress: 1.0,
	})

	return state, ex.getEvents()
}

func (ex *Executor) executeNode(node GraphNode, state map[string]interface{}) NodeResult {
	result := NodeResult{NodeID: node.ID}

	switch node.NodeType {
	case NodeInput:
		if v, ok := state[node.ID]; ok {
			result.Output = fmt.Sprintf("%v", v)
		} else if v, ok := state["input"]; ok {
			result.Output = fmt.Sprintf("%v", v)
		} else {
			result.Output = fmt.Sprintf("[input] %s", node.Label)
		}
		result.Status = "completed"

	case NodeLLM:
		prompt, _ := node.Config["prompt"].(string)
		result.Output = fmt.Sprintf("[llm] %s (simulated: %s)", node.Label, truncateStr(prompt, 50))
		result.Status = "completed"

	case NodeTool:
		toolName, _ := node.Config["tool"].(string)
		result.Output = fmt.Sprintf("[tool] %s executed", toolName)
		result.Status = "completed"

	case NodeCondition:
		result.Output = fmt.Sprintf("[condition] %s", node.Label)
		result.Status = "completed"

	case NodeOutput:
		result.Output = fmt.Sprintf("[output] %s", node.Label)
		result.Status = "completed"

	default:
		result.Status = "error"
		result.Error = fmt.Sprintf("unknown node type: %s", node.NodeType)
	}

	return result
}

func (ex *Executor) addEvent(event GraphEvent) {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	ex.events = append(ex.events, event)
}

func (ex *Executor) getEvents() []GraphEvent {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	events := make([]GraphEvent, len(ex.events))
	copy(events, ex.events)
	return events
}

func (ex *Executor) Results() map[string]*NodeResult {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	results := make(map[string]*NodeResult)
	for k, v := range ex.results {
		results[k] = v
	}
	return results
}

func evaluateCondition(condition, output string) bool {
	cond := strings.ToLower(strings.TrimSpace(condition))
	out := strings.ToLower(output)

	switch {
	case cond == "true", cond == "success":
		return true
	case strings.HasPrefix(cond, "contains:"):
		return strings.Contains(out, strings.TrimPrefix(cond, "contains:"))
	case strings.HasPrefix(cond, "equals:"):
		return out == strings.TrimPrefix(cond, "equals:")
	default:
		return out != ""
	}
}

// ── Tools ─────────────────────────────────────────────────────────────────

type CreateTool struct{}

func NewCreateTool() *CreateTool { return &CreateTool{} }
func (t *CreateTool) Name() string       { return "graph_create" }
func (t *CreateTool) Description() string { return "Create a StateGraph workflow from node/edge definitions." }

func (t *CreateTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	builder := NewBuilder()

	if nodes, ok := input["nodes"].([]interface{}); ok {
		for _, n := range nodes {
			if node, ok := n.(map[string]interface{}); ok {
				id, _ := node["id"].(string)
				label, _ := node["label"].(string)
				nodeType, _ := node["node_type"].(string)
				config, _ := node["config"].(map[string]interface{})
				builder.AddNode(id, label, NodeType(nodeType), config)
			}
		}
	}

	if edges, ok := input["edges"].([]interface{}); ok {
		for _, e := range edges {
			if edge, ok := e.(map[string]interface{}); ok {
				src, _ := edge["source_id"].(string)
				tgt, _ := edge["target_id"].(string)
				cond, _ := edge["condition"].(string)
				label, _ := edge["label"].(string)
				builder.AddEdge(src, tgt, cond, label)
			}
		}
	}

	if entry, ok := input["entry_point"].(string); ok {
		builder.SetEntryPoint(entry)
	}

	graph, result := builder.Compile()
	if !result.Valid {
		return nil, fmt.Errorf("compile errors: %s", strings.Join(result.Errors, "; "))
	}

	return map[string]interface{}{
		"output":           fmt.Sprintf("Graph '%s' created (%d nodes, %d edges)\nTopo: %s", name, len(graph.Nodes), len(graph.Edges), FormatTopo(result.TopologicalOrder)),
		"node_count":       len(graph.Nodes),
		"edge_count":       len(graph.Edges),
		"topological_order": result.TopologicalOrder,
	}, nil
}

type RunTool struct{}

func NewRunTool() *RunTool { return &RunTool{} }
func (t *RunTool) Name() string       { return "graph_run" }
func (t *RunTool) Description() string { return "Create and execute a StateGraph workflow in one step." }

func (t *RunTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	builder := NewBuilder()
	if nodes, ok := input["nodes"].([]interface{}); ok {
		for _, n := range nodes {
			if node, ok := n.(map[string]interface{}); ok {
				id, _ := node["id"].(string)
				label, _ := node["label"].(string)
				nodeType, _ := node["node_type"].(string)
				config, _ := node["config"].(map[string]interface{})
				builder.AddNode(id, label, NodeType(nodeType), config)
			}
		}
	}
	if edges, ok := input["edges"].([]interface{}); ok {
		for _, e := range edges {
			if edge, ok := e.(map[string]interface{}); ok {
				src, _ := edge["source_id"].(string)
				tgt, _ := edge["target_id"].(string)
				cond, _ := edge["condition"].(string)
				label, _ := edge["label"].(string)
				builder.AddEdge(src, tgt, cond, label)
			}
		}
	}
	if entry, ok := input["entry_point"].(string); ok {
		builder.SetEntryPoint(entry)
	}

	graph, compileResult := builder.Compile()
	if !compileResult.Valid {
		return nil, fmt.Errorf("compile errors: %s", strings.Join(compileResult.Errors, "; "))
	}

	initialState, _ := input["initial_state"].(map[string]interface{})
	executor := NewExecutor()
	state, events := executor.Execute(ctx, graph, initialState)

	slog.Info("graph executed", "name", name, "nodes", len(graph.Nodes), "events", len(events))

	return map[string]interface{}{
		"output":  fmt.Sprintf("Graph '%s' executed (%d nodes)\nStatus: completed", name, len(graph.Nodes)),
		"state":   state,
		"events":  events,
		"results": executor.Results(),
	}, nil
}

func RegisterTools(tr *tools.ToolRegistry) {
	tr.Register(NewCreateTool())
	tr.Register(NewRunTool())
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
