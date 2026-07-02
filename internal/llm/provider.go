package llm

import (
	"context"
	"errors"
)

var (
	ErrNoProvider     = errors.New("no LLM provider available")
	ErrAllFailed      = errors.New("all LLM providers failed")
	ErrRateLimited    = errors.New("rate limited")
	ErrTimeout        = errors.New("LLM request timed out")
)

// ── Function Calling Types ──

// ToolDef describes a tool the LLM can call.
type ToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema object
}

// ToolCall represents a function call requested by the LLM.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ── Message ──

type Message struct {
	Role       string     `json:"role"`                 // user / assistant / system / tool
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`  // assistant → tool calls
	ToolCallID string     `json:"tool_call_id,omitempty"` // tool → result correlation
	Name       string     `json:"name,omitempty"`
}

// ── Request / Response ──

type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type Response struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	Model        string     `json:"model"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
	Cached       bool       `json:"cached"`
	FinishReason string     `json:"finish_reason"` // "stop" | "tool_calls" | "length"
}

// ── Usage ──

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ── Provider Interface ──

type Provider interface {
	Name() string
	Chat(ctx context.Context, req *Request) (*Response, error)
	ChatStream(ctx context.Context, req *Request, onChunk func(string), onToolCall func(ToolCall)) (*Response, error)
	IsAvailable() bool
}
