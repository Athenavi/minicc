package dag

import (
	"fmt"
	"strings"
)

// TopologicalSort performs topological sorting on the DAG.
// Returns nodes in execution order.
func TopologicalSort(dag *WorkflowDAG) ([]WorkflowNode, error) {
	// Build adjacency list and in-degree map
	graph := make(map[string][]string)
	inDegree := make(map[string]int)
	nodeMap := make(map[string]WorkflowNode)
	
	for _, node := range dag.Nodes {
		nodeMap[node.ID] = node
		if _, ok := graph[node.ID]; !ok {
			graph[node.ID] = []string{}
			inDegree[node.ID] = 0
		}
	}
	
	for _, edge := range dag.Edges {
		if _, exists := nodeMap[edge.Source]; !exists {
			return nil, fmt.Errorf("node %s in edge not found", edge.Source)
		}
		if _, exists := nodeMap[edge.Target]; !exists {
			return nil, fmt.Errorf("node %s in edge not found", edge.Target)
		}
		
		graph[edge.Source] = append(graph[edge.Source], edge.Target)
		inDegree[edge.Target]++
	}
	
	// Kahn's algorithm
	queue := []string{}
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	
	var sorted []WorkflowNode
	processed := 0
	
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		
		sorted = append(sorted, nodeMap[nodeID])
		processed++
		
		for _, neighbor := range graph[nodeID] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}
	
	if processed != len(dag.Nodes) {
		return nil, fmt.Errorf("cycle detected in workflow DAG")
	}
	
	return sorted, nil
}

// Validate checks if the DAG has valid structure.
func Validate(dag *WorkflowDAG) error {
	// Check for duplicate node IDs
	nodeIDs := make(map[string]bool)
	for _, node := range dag.Nodes {
		if nodeIDs[node.ID] {
			return fmt.Errorf("duplicate node ID: %s", node.ID)
		}
		nodeIDs[node.ID] = true
	}
	
	// Check for invalid edges
	for _, edge := range dag.Edges {
		if !nodeIDs[edge.Source] {
			return fmt.Errorf("edge references undefined source node: %s", edge.Source)
		}
		if !nodeIDs[edge.Target] {
			return fmt.Errorf("edge references undefined target node: %s", edge.Target)
		}
	}
	
	return nil
}

// ResolveParameters resolves template placeholders in parameters.
func ResolveParameters(params map[string]interface{}, execCtx *ExecutionContext) (map[string]interface{}, error) {
	resolved := make(map[string]interface{})
	
	for key, value := range params {
		resolved[key] = resolveValue(value, execCtx)
	}
	
	return resolved, nil
}

// resolveValue recursively resolves template placeholders.
func resolveValue(value interface{}, execCtx *ExecutionContext) interface{} {
	strValue, ok := value.(string)
	if !ok {
		return value
	}
	
	if !strings.Contains(strValue, "{{") {
		return strValue
	}
	
	// Replace {{nodes.node_id.output}}
	for nodeID, result := range execCtx.NodeResults {
		placeholder := fmt.Sprintf("{{nodes.%s.output}}", nodeID)
		if strings.Contains(strValue, placeholder) {
			if result.Status == "success" {
				return strings.ReplaceAll(strValue, placeholder, fmt.Sprintf("%v", result.Output))
			} else {
				return fmt.Sprintf("<<ERROR: node %s failed>>", nodeID)
			}
		}
	}
	
	// Replace {{input.field_name}}
	for inputKey, inputValue := range execCtx.InputData {
		placeholder := fmt.Sprintf("{{input.%s}}", inputKey)
		if strings.Contains(strValue, placeholder) {
			replaced := strings.ReplaceAll(strValue, placeholder, fmt.Sprintf("%v", inputValue))
			return replaced
		}
	}
	
	return strValue
}
