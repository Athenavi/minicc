package llm

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// ── Context Manager ──
// Manages conversation context: compresses history when it grows too long,
// maintains persistent memory across sessions.

type ContextManager struct {
	mu              sync.RWMutex
	maxMessages     int           // max messages before compression
	summaryInterval int           // compress every N messages past the limit
	llm             *Gateway      // used for summarization
}

func NewContextManager(llm *Gateway, maxMessages int) *ContextManager {
	return &ContextManager{
		maxMessages:     maxMessages,
		summaryInterval: 10,
		llm:             llm,
	}
}

// CompressIfNeeded checks if the message history exceeds the limit and
// compresses the oldest messages into a summary.
func (cm *ContextManager) CompressIfNeeded(ctx context.Context, messages []Message) ([]Message, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Separate system prompt from conversation
	var systemPrompt string
	convMsgs := make([]Message, 0)
	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
		} else {
			convMsgs = append(convMsgs, m)
		}
	}

	// Check if compression is needed
	if len(convMsgs) <= cm.maxMessages {
		return messages, nil // no compression needed
	}

	// Determine how many messages to compress
	compressCount := len(convMsgs) - cm.maxMessages + cm.summaryInterval
	if compressCount < 2 {
		compressCount = 2
	}
	// Must be even (pairs of user+assistant or tool rounds)
	if compressCount%2 != 0 {
		compressCount++
	}
	if compressCount > len(convMsgs)-2 {
		compressCount = len(convMsgs) - 2
	}

	toCompress := convMsgs[:compressCount]
	remaining := convMsgs[compressCount:]

	// Build summary
	summary := cm.summarize(ctx, toCompress, systemPrompt)

	slog.Info("context compressed",
		"compressed", compressCount,
		"remaining", len(remaining),
		"summary_length", len(summary),
	)

	// Replace compressed messages with summary
	result := make([]Message, 0, len(remaining)+2)
	if systemPrompt != "" {
		result = append(result, Message{Role: "system", Content: systemPrompt})
	}
	// Add summary as a system message
	result = append(result, Message{Role: "system", Content: fmt.Sprintf(
		"[Context Summary of previous conversation]:\n%s", summary)})
	result = append(result, remaining...)

	return result, nil
}

// summarize creates a concise summary of the given messages.
func (cm *ContextManager) summarize(ctx context.Context, messages []Message, systemPrompt string) string {
	if cm.llm == nil || len(messages) == 0 {
		return cm.fallbackSummary(messages)
	}

	// Build a compact version of the messages
	var sb strings.Builder
	for _, m := range messages {
		role := m.Role
		content := m.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", role, content))
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				sb.WriteString(fmt.Sprintf("  → called tool: %s\n", tc.Name))
			}
		}
	}

	prompt := fmt.Sprintf(
		"Summarize this conversation concisely. Keep all decisions, code changes, and key facts. "+
			"Original: max %d chars.\n\n%s", 500, sb.String())

	req := &Request{
		Messages: []Message{
			{Role: "system", Content: "You are a conversation summarizer. Be concise."},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   1024,
		Temperature: 0.3,
	}

	resp, err := cm.llm.Chat(ctx, req)
	if err != nil || resp == nil || resp.Content == "" {
		slog.Warn("summarization failed, using fallback", "error", err)
		return cm.fallbackSummary(messages)
	}

	return resp.Content
}

func (cm *ContextManager) fallbackSummary(messages []Message) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Conversation with %d messages. ", len(messages)))

	// Count by role
	userCount, asstCount, toolCount := 0, 0, 0
	for _, m := range messages {
		switch m.Role {
		case "user": userCount++
		case "assistant": asstCount++
		case "tool": toolCount++
		}
	}
	sb.WriteString(fmt.Sprintf("User: %d, Assistant: %d, Tool calls: %d.", userCount, asstCount, toolCount))

	// Include first and last user message
	for _, m := range messages {
		if m.Role == "user" {
			content := m.Content
			if len(content) > 100 { content = content[:100] + "..." }
			sb.WriteString(fmt.Sprintf("\nUser: %s", content))
			break
		}
	}

	return sb.String()
}

// ── Token Budget Tracker ──

type TokenBudget struct {
	mu         sync.Mutex
	maxTokens  int
	usedTokens int
}

func NewTokenBudget(maxTokens int) *TokenBudget {
	return &TokenBudget{maxTokens: maxTokens}
}

func (tb *TokenBudget) Record(tokens int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.usedTokens += tokens
}

func (tb *TokenBudget) Remaining() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	remaining := tb.maxTokens - tb.usedTokens
	if remaining < 0 { remaining = 0 }
	return remaining
}

func (tb *TokenBudget) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.usedTokens = 0
}

func (tb *TokenBudget) Stats() map[string]interface{} {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return map[string]interface{}{
		"max_tokens":  tb.maxTokens,
		"used_tokens": tb.usedTokens,
		"remaining":   tb.maxTokens - tb.usedTokens,
	}
}
