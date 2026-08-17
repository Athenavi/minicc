package dag

import (
	"context"
	"fmt"
	"time"
)

// NodeType represents the type of a workflow node.
type NodeType string

const (
	NodeTypeHTTPAPI    NodeType = "http_api"
	NodeTypePythonFunc NodeType = "python_function"
	NodeTypeLLMCall    NodeType = "llm_call"
)

// WorkflowNode represents a single node in a workflow DAG.
type WorkflowNode struct {
	ID        string                 `json:"id"`
	Type      NodeType               `json:"type"`
	Endpoint  string                 `json:"endpoint,omitempty"`
	Function  string                 `json:"function,omitempty"`
	Model     string                 `json:"model,omitempty"`
	Prompt    string                 `json:"prompt,omitempty"`
	Params    map[string]interface{} `json:"params"`
	OutputVar string                 `json:"output_var"`
	Retries   int                    `json:"retries"`
	TimeoutMs int                    `json:"timeout_ms"`
}

// WorkflowEdge represents an edge in the workflow DAG.
type WorkflowEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// WorkflowDAG represents a directed acyclic graph for workflow orchestration.
type WorkflowDAG struct {
	Nodes []WorkflowNode `json:"nodes"`
	Edges []WorkflowEdge `json:"edges"`
}

// ExecutionContext holds the state during workflow execution.
type ExecutionContext struct {
	NodeResults map[string]NodeResult
	InputData   map[string]interface{}
	Cancel      context.CancelFunc
}

// NodeResult holds the result of a node execution.
type NodeResult struct {
	NodeID      string                 `json:"node_id"`
	Status      string                 `json:"status"` // success/failed/skipped
	DurationMs  int64                  `json:"duration_ms"`
	Output      map[string]interface{} `json:"output"`
	Error       string                 `json:"error,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

// WorkflowExecutor executes a workflow DAG.
type WorkflowExecutor struct {
	dag *WorkflowDAG
}

// NewWorkflowExecutor creates a new workflow executor.
func NewWorkflowExecutor(dag *WorkflowDAG) *WorkflowExecutor {
	return &WorkflowExecutor{dag: dag}
}

// Execute runs the workflow with the given input data.
func (e *WorkflowExecutor) Execute(ctx context.Context, inputData map[string]interface{}) (map[string]interface{}, error) {
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	
	executionContext := &ExecutionContext{
		NodeResults: make(map[string]NodeResult),
		InputData:   inputData,
		Cancel:      cancel,
	}
	
	// Perform topological sort
	orderedNodes, err := TopologicalSort(e.dag)
	if err != nil {
		return nil, fmt.Errorf("topological sort failed: %w", err)
	}
	
	// Execute nodes in order
	for _, node := range orderedNodes {
		select {
		case <-execCtx.Done():
			return nil, fmt.Errorf("workflow cancelled")
		default:
		}
		
		result := e.executeNode(execCtx, node, executionContext)
		executionContext.NodeResults[node.ID] = result
		
		if result.Status == "failed" {
			return nil, fmt.Errorf("node %s failed: %s", node.ID, result.Error)
		}
	}
	
	// Collect final output
	finalOutput := make(map[string]interface{})
	for _, node := range orderedNodes {
		if result, ok := executionContext.NodeResults[node.ID]; ok && result.Status == "success" {
			if node.OutputVar != "" {
				finalOutput[node.OutputVar] = result.Output
			}
		}
	}
	
	return finalOutput, nil
}

// executeNode executes a single workflow node.
func (e *WorkflowExecutor) executeNode(ctx context.Context, node WorkflowNode, execCtx *ExecutionContext) NodeResult {
	start := time.Now()
	result := NodeResult{
		NodeID:    node.ID,
		Status:    "pending",
		Timestamp: start,
	}
	
	// Resolve parameters
	resolvedParams, err := ResolveParameters(node.Params, execCtx)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("parameter resolution failed: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	
	// Execute based on node type
	var output map[string]interface{}
	var execErr error
	
	switch node.Type {
	case NodeTypeHTTPAPI:
		output, execErr = e.executeHTTPAPI(ctx, node, resolvedParams)
	case NodeTypePythonFunc:
		output, execErr = e.executePythonFunc(ctx, node, resolvedParams)
	case NodeTypeLLMCall:
		output, execErr = e.executeLLMCall(ctx, node, resolvedParams)
	default:
		execErr = fmt.Errorf("unsupported node type: %s", node.Type)
	}
	
	duration := time.Since(start).Milliseconds()
	
	if execErr != nil {
		result.Status = "failed"
		result.Error = execErr.Error()
	} else {
		result.Status = "success"
		result.Output = output
	}
	
	result.DurationMs = duration
	return result
}

// executeHTTPAPI executes an HTTP API node.
func (e *WorkflowExecutor) executeHTTPAPI(ctx context.Context, node WorkflowNode, params map[string]interface{}) (map[string]interface{}, error) {
	// TODO: Implement HTTP client execution
	// For now, return mock data
	return map[string]interface{}{
		"node_id": node.ID,
		"params":  params,
		"status":  "mock_success",
	}, nil
}

// executePythonFunc executes a Python function node.
func (e *WorkflowExecutor) executePythonFunc(ctx context.Context, node WorkflowNode, params map[string]interface{}) (map[string]interface{}, error) {
	// TODO: Implement Python function invocation via subprocess or gRPC
	return map[string]interface{}{
		"node_id": node.ID,
		"function": node.Function,
		"params":  params,
		"status":  "mock_success",
	}, nil
}

// executeLLMCall executes an LLM call node.
func (e *WorkflowExecutor) executeLLMCall(ctx context.Context, node WorkflowNode, params map[string]interface{}) (map[string]interface{}, error) {
	// TODO: Implement LLM API call
	return map[string]interface{}{
		"node_id": node.ID,
		"model":   node.Model,
		"prompt":  node.Prompt,
		"status":  "mock_success",
	}, nil
}
