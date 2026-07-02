package graph

import (
	"fmt"
	"strings"
)

type NodeType string

const (
	NodeInput     NodeType = "input"
	NodeLLM       NodeType = "llm"
	NodeTool      NodeType = "tool"
	NodeCondition NodeType = "condition"
	NodeOutput    NodeType = "output"
)

type GraphNode struct {
	ID       string                 `json:"id"`
	Label    string                 `json:"label"`
	NodeType NodeType               `json:"node_type"`
	Config   map[string]interface{} `json:"config,omitempty"`
}

type GraphEdge struct {
	SourceID  string `json:"source_id"`
	TargetID  string `json:"target_id"`
	Condition string `json:"condition,omitempty"`
	Label     string `json:"label,omitempty"`
}

type StateGraph struct {
	Name       string      `json:"name"`
	EntryPoint string      `json:"entry_point"`
	Nodes      []GraphNode `json:"nodes"`
	Edges      []GraphEdge `json:"edges"`
}

type CompileResult struct {
	Valid           bool     `json:"valid"`
	Errors          []string `json:"errors,omitempty"`
	TopologicalOrder []string `json:"topological_order,omitempty"`
}

type Builder struct {
	nodes      map[string]GraphNode
	edges      []GraphEdge
	entryPoint string
}

func NewBuilder() *Builder {
	return &Builder{
		nodes: make(map[string]GraphNode),
	}
}

func (b *Builder) AddNode(id, label string, nodeType NodeType, config map[string]interface{}) *Builder {
	b.nodes[id] = GraphNode{ID: id, Label: label, NodeType: nodeType, Config: config}
	return b
}

func (b *Builder) AddEdge(source, target string, condition, label string) *Builder {
	b.edges = append(b.edges, GraphEdge{SourceID: source, TargetID: target, Condition: condition, Label: label})
	return b
}

func (b *Builder) SetEntryPoint(id string) *Builder {
	b.entryPoint = id
	return b
}

func (b *Builder) Compile() (*StateGraph, *CompileResult) {
	var errors []string

	if len(b.nodes) == 0 {
		errors = append(errors, "graph has no nodes")
		return nil, &CompileResult{Valid: false, Errors: errors}
	}

	entry := b.entryPoint
	if entry == "" {
		for id := range b.nodes {
			entry = id
			break
		}
	}

	if _, ok := b.nodes[entry]; !ok {
		errors = append(errors, fmt.Sprintf("entry point '%s' not found", entry))
		return nil, &CompileResult{Valid: false, Errors: errors}
	}

	for _, e := range b.edges {
		if _, ok := b.nodes[e.SourceID]; !ok {
			errors = append(errors, fmt.Sprintf("edge source '%s' not found", e.SourceID))
		}
		if _, ok := b.nodes[e.TargetID]; !ok {
			errors = append(errors, fmt.Sprintf("edge target '%s' not found", e.TargetID))
		}
	}

	if len(errors) > 0 {
		return nil, &CompileResult{Valid: false, Errors: errors}
	}

	// Topological sort (Kahn's algorithm)
	inDeg := make(map[string]int)
	for id := range b.nodes {
		inDeg[id] = 0
	}
	adj := make(map[string][]string)
	for id := range b.nodes {
		adj[id] = nil
	}
	for _, e := range b.edges {
		adj[e.SourceID] = append(adj[e.SourceID], e.TargetID)
		inDeg[e.TargetID]++
	}

	var queue []string
	for id, d := range inDeg {
		if d == 0 {
			queue = append(queue, id)
		}
	}

	var topo []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		topo = append(topo, node)
		for _, neighbor := range adj[node] {
			inDeg[neighbor]--
			if inDeg[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(topo) != len(b.nodes) {
		errors = append(errors, "graph contains a cycle")
		return nil, &CompileResult{Valid: false, Errors: errors}
	}

	// Build node list in topological order
	nodeList := make([]GraphNode, 0, len(topo))
	nodeMap := make(map[string]bool)
	for _, id := range topo {
		nodeMap[id] = true
	}
	for _, id := range topo {
		nodeList = append(nodeList, b.nodes[id])
	}

	graph := &StateGraph{
		Name:       "compiled_graph",
		EntryPoint: entry,
		Nodes:      nodeList,
		Edges:      b.edges,
	}

	return graph, &CompileResult{
		Valid:            true,
		TopologicalOrder: topo,
	}
}

// FormatTopo returns a human-readable topological order string.
func FormatTopo(order []string) string {
	return strings.Join(order, " → ")
}
