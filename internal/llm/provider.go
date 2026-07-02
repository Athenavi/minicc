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

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`    // user / assistant / system / tool
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// Request to the LLM.
type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// Response from the LLM.
type Response struct {
	Content      string `json:"content"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Cached       bool   `json:"cached"`
}

// Usage stats.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// Provider is the interface for LLM backends.
type Provider interface {
	Name() string
	Chat(ctx context.Context, req *Request) (*Response, error)
	IsAvailable() bool
}
