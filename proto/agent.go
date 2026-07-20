// Package proto provides generated types for the AgentEngine gRPC service.
// Manually maintained (protoc not available in CI).
// Source: proto/agent.proto, proto/common.proto
package proto

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
)

// ── Common types ──

type Message struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type ToolDef struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	ParametersJSON string `json:"parameters_json"`
}

type LLMConfig struct {
	Model       string  `json:"model"`
	MaxTokens   int32   `json:"max_tokens"`
	Temperature float32 `json:"temperature"`
}

type Usage struct {
	InputTokens  int32 `json:"input_tokens"`
	OutputTokens int32 `json:"output_tokens"`
	TotalTokens  int32 `json:"total_tokens"`
}

type HealthCheckRequest struct{}
type HealthCheckResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

// ── Agent types ──

type RunRequest struct {
	SessionID    string      `json:"session_id"`
	UserID       string      `json:"user_id"`
	TenantID     string      `json:"tenant_id"`
	Content      string      `json:"content"`
	SystemPrompt string      `json:"system_prompt"`
	History      []*Message  `json:"history"`
	Tools        []*ToolDef  `json:"tools"`
	LlmConfig    *LLMConfig  `json:"llm_config"`
	MaxTurns     int32       `json:"max_turns"`
}

type ToolCallRequest struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// RunEvent represents a single event from the streaming Run response.
type RunEvent struct {
	TextChunk *string
	ToolCall  *ToolCallRequest
	Usage     *Usage
	Error     *string
	Done      bool
}

// ── Client ──

// AgentClient wraps a gRPC connection to the AgentEngine service.
type AgentClient struct {
	conn   *grpc.ClientConn
	stream AgentEngine_RunClient
}

// AgentEngine_RunClient is the client stream for the Run RPC.
type AgentEngine_RunClient interface {
	Recv() (*RunEvent, error)
	CloseSend() error
}

// NewAgentClient creates a new AgentClient from an existing gRPC connection.
func NewAgentClient(conn *grpc.ClientConn) *AgentClient {
	return &AgentClient{conn: conn}
}

// Run starts a streaming agent inference call.
// Since we don't have generated proto code, we use a simplified approach:
// the Python engine exposes an HTTP endpoint as fallback.
func (c *AgentClient) Run(req *RunRequest) (*RunStream, error) {
	if c.conn == nil {
		return nil, status.Error(codes.Unavailable, "gRPC connection not available")
	}
	// For now, return an error indicating gRPC is not fully wired.
	// The Controller will fall back to direct LLM calls.
	return nil, status.Error(codes.Unimplemented, "gRPC Run not yet implemented — use HTTP fallback")
}

// RunStream is a simplified stream interface.
type RunStream struct {
	events chan RunEvent
	err    error
}

func (s *RunStream) Recv() (*RunEvent, error) {
	event, ok := <-s.events
	if !ok {
		return nil, io.EOF
	}
	return &event, s.err
}
