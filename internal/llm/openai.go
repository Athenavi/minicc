package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type OpenAIProvider struct {
	name    string
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewOpenAIProvider(apiKey, baseURL, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return &OpenAIProvider{
		name:    "openai",
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:    20,
				IdleConnTimeout: 30 * time.Second,
			},
		},
	}
}

func (p *OpenAIProvider) Name() string { return p.name }
func (p *OpenAIProvider) IsAvailable() bool { return p.apiKey != "" }

// ── JSON types matching the OpenAI API ──

type openAIMessage struct {
	Role       string            `json:"role"`
	Content    string            `json:"content,omitempty"`
	ToolCalls  []openAIToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	Name       string            `json:"name,omitempty"`
}

type openAIToolDef struct {
	Type       string `json:"type"`
	Function   struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

type openAIToolCall struct {
	Index    *int            `json:"index,omitempty"`
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function openAIFunction  `json:"function"`
}

type openAIFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIRequest struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	Tools       []openAIToolDef  `json:"tools,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content   string           `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// openAIStreamChunk is a single SSE delta from a streaming response.
type openAIStreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content   string           `json:"content,omitempty"`
			ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// ── Non-streaming Chat ──

func (p *OpenAIProvider) Chat(ctx context.Context, req *Request) (*Response, error) {
	openReq := p.buildRequest(req, false)
	data, _ := json.Marshal(openReq)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		slog.Warn("openai error", "status", resp.StatusCode, "body", string(respBody[:min(len(respBody), 500)]))
		var errResp openAIResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("openai: %s (%s)", errResp.Error.Message, errResp.Error.Code)
		}
		return nil, fmt.Errorf("openai: HTTP %d", resp.StatusCode)
	}

	var result openAIResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices")
	}

	choice := result.Choices[0]
	respModel := result.Model
	if respModel == "" { respModel = p.model }

	return &Response{
		Content:      choice.Message.Content,
		Model:        respModel,
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
		ToolCalls:    p.toToolCalls(choice.Message.ToolCalls),
	}, nil
}

// ── Streaming Chat with Function Calling ──

func (p *OpenAIProvider) ChatStream(ctx context.Context, req *Request, onChunk func(string), onToolCall func(ToolCall)) (*Response, error) {
	openReq := p.buildRequest(req, true)
	data, _ := json.Marshal(openReq)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		slog.Warn("openai stream error", "status", resp.StatusCode, "body", string(respBody[:min(len(respBody), 500)]))
		return nil, fmt.Errorf("openai: HTTP %d", resp.StatusCode)
	}

	result := &Response{Model: p.model}
	var fullContent string

	// Accumulate tool calls across chunks
	type accToolCall struct {
		index     int
		id        string
		name      string
		arguments string
	}
	accMap := make(map[int]*accToolCall)

	br := bufio.NewReader(resp.Body)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) { break }
			return nil, fmt.Errorf("read stream: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") { continue }
		if !strings.HasPrefix(line, "data: ") { continue }

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" { break }

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			slog.Debug("stream parse error", "error", err)
			continue
		}

		if len(chunk.Choices) == 0 { continue }
		choice := chunk.Choices[0]

		// Text content
		if choice.Delta.Content != "" {
			fullContent += choice.Delta.Content
			onChunk(choice.Delta.Content)
		}

		// Tool calls
		for _, tc := range choice.Delta.ToolCalls {
			idx := 0
			if tc.Index != nil { idx = *tc.Index }

			acc, ok := accMap[idx]
			if !ok {
				acc = &accToolCall{index: idx}
				accMap[idx] = acc
			}

			if tc.ID != "" { acc.id = tc.ID }
			if tc.Function.Name != "" { acc.name = tc.Function.Name }
			acc.arguments += tc.Function.Arguments
		}

		// Model
		if chunk.Model != "" { result.Model = chunk.Model }

		// Usage (last chunk)
		if chunk.Usage != nil {
			result.InputTokens = chunk.Usage.PromptTokens
			result.OutputTokens = chunk.Usage.CompletionTokens
		}

		// Finish reason
		if choice.FinishReason != nil {
			result.FinishReason = *choice.FinishReason
		}
	}

	result.Content = fullContent

	// Emit completed tool calls
	for i := 0; i < len(accMap); i++ {
		if acc, ok := accMap[i]; ok {
			tc := ToolCall{ID: acc.id, Name: acc.name, Arguments: acc.arguments}
			result.ToolCalls = append(result.ToolCalls, tc)
			onToolCall(tc)
		}
	}

	return result, nil
}

// ── Helpers ──

func (p *OpenAIProvider) buildRequest(req *Request, stream bool) openAIRequest {
	msgs := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		omsg := openAIMessage{Role: m.Role, Content: m.Content, Name: m.Name, ToolCallID: m.ToolCallID}
		if len(m.ToolCalls) > 0 {
			omsg.ToolCalls = make([]openAIToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				omsg.ToolCalls[j] = openAIToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: openAIFunction{Name: tc.Name, Arguments: tc.Arguments},
				}
			}
		}
		msgs[i] = omsg
	}

	r := openAIRequest{
		Model:       p.model,
		Messages:    msgs,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      stream,
	}

	if len(req.Tools) > 0 {
		r.Tools = make([]openAIToolDef, len(req.Tools))
		for i, t := range req.Tools {
			r.Tools[i] = openAIToolDef{Type: "function"}
			r.Tools[i].Function.Name = t.Name
			r.Tools[i].Function.Description = t.Description
			r.Tools[i].Function.Parameters = t.Parameters
		}
	}

	return r
}

func (p *OpenAIProvider) toToolCalls(apiCalls []openAIToolCall) []ToolCall {
	if len(apiCalls) == 0 { return nil }
	tcs := make([]ToolCall, len(apiCalls))
	for i, tc := range apiCalls {
		tcs[i] = ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments}
	}
	return tcs
}
