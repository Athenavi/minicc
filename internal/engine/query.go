package engine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/athenavi/minicc/internal/llm"
	"github.com/athenavi/minicc/internal/tools"
)

// Engine is the core loop: receives messages, calls LLM, executes tools, returns results.
type Engine struct {
	llm   *llm.Gateway
	tools *tools.ToolRegistry
}

func New(llmGateway *llm.Gateway, toolRegistry *tools.ToolRegistry) *Engine {
	return &Engine{
		llm:    llmGateway,
		tools:  toolRegistry,
	}
}

// TurnResult represents the result of a single turn.
type TurnResult struct {
	Content    string       `json:"content"`
	ToolCalls  []ToolResult `json:"tool_calls,omitempty"`
	Usage      *llm.Usage   `json:"usage,omitempty"`
}

type ToolResult struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// ProcessTurn handles one user message and returns the assistant response.
func (e *Engine) ProcessTurn(ctx context.Context, messages []llm.Message) (*TurnResult, error) {
	if e.llm == nil {
		return nil, fmt.Errorf("LLM gateway not configured")
	}
	req := &llm.Request{
		Model:       "default",
		Messages:    messages,
		MaxTokens:   4096,
		Temperature: 0.7,
	}

	resp, err := e.llm.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	result := &TurnResult{
		Content: resp.Content,
		Usage: &llm.Usage{
			InputTokens:  resp.InputTokens,
			OutputTokens: resp.OutputTokens,
			TotalTokens:  resp.InputTokens + resp.OutputTokens,
		},
	}

	// TODO: Parse tool calls from response content (Phase 2 enhancement)
	slog.Debug("turn processed",
		"input_tokens", resp.InputTokens,
		"output_tokens", resp.OutputTokens,
		"cached", resp.Cached,
	)

	return result, nil
}
