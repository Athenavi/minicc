package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/llm"
	"github.com/athenavi/minicc/internal/tools"
)

// TurnOrchestrator manages the multi-turn agent loop:
// LLM → tool_calls → execute → feed back → LLM (repeat) → final text → turn_done.
type TurnOrchestrator struct {
	llm     *llm.Gateway
	tools   *tools.ToolRegistry
	hub     *broadcast.Hub
	ctxMgr  *llm.ContextManager
}

func NewTurnOrchestrator(gateway *llm.Gateway, tools *tools.ToolRegistry, hub *broadcast.Hub) *TurnOrchestrator {
	return &TurnOrchestrator{
		llm:    gateway,
		tools:  tools,
		hub:    hub,
		ctxMgr: llm.NewContextManager(gateway, 30),
	}
}

// Execute runs the full agent turn lifecycle.
// Returns the final assistant content + total usage.
func (o *TurnOrchestrator) Execute(ctx context.Context, sessionID string, messages []llm.Message, systemPrompt string, toolDefs []llm.ToolDef) (string, *llm.Usage, error) {
	var totalInput, totalOutput int
	maxTurns := 10 // safety limit

	// Build full message list
	allMsgs := make([]llm.Message, 0, len(messages)+1)
	if systemPrompt != "" {
		allMsgs = append(allMsgs, llm.Message{Role: "system", Content: systemPrompt})
	}
	allMsgs = append(allMsgs, messages...)

	for turn := 0; turn < maxTurns; turn++ {
		// Compress context if needed (before each LLM round)
		compressed, compressErr := o.ctxMgr.CompressIfNeeded(ctx, allMsgs)
		if compressErr == nil {
			allMsgs = compressed
		}

		req := &llm.Request{
			Messages:    allMsgs,
			Tools:       toolDefs,
			MaxTokens:   4096,
			Temperature: 0.7,
			Stream:      true,
		}

		var assistantContent string
		var executedTools int

		resp, err := o.llm.ChatStream(ctx, req,
			// onChunk: emit text events
			func(chunk string) {
				assistantContent += chunk
				o.hub.Publish(broadcast.Event{Type: "text", Data: map[string]string{"content": chunk}})
			},
			// onToolCall: emit tool_use events
			func(tc llm.ToolCall) {
				// Emit tool_use event so frontend can show it
				data, _ := json.Marshal(map[string]string{
					"tool_call_id": tc.ID,
					"tool_name":    tc.Name,
					"arguments":    tc.Arguments,
					"session_id":   sessionID,
				})
				o.hub.Publish(broadcast.Event{Type: "tool_use", Data: json.RawMessage(data)})
			},
		)

		if err != nil {
			return assistantContent, &llm.Usage{InputTokens: totalInput, OutputTokens: totalOutput}, err
		}

		totalInput += resp.InputTokens
		totalOutput += resp.OutputTokens

		// Add assistant message to history
		assistantMsg := llm.Message{Role: "assistant", Content: assistantContent}
		if len(resp.ToolCalls) > 0 {
			assistantMsg.ToolCalls = resp.ToolCalls
		}
		allMsgs = append(allMsgs, assistantMsg)

		// If no tool calls, we're done
		if len(resp.ToolCalls) == 0 {
			break
		}

		// Execute each tool call
		for _, tc := range resp.ToolCalls {
			executedTools++
			stepStart := time.Now()

			// Parse arguments
			var args map[string]interface{}
			json.Unmarshal([]byte(tc.Arguments), &args)

			// Execute tool
			var output string
			var toolErr error

			tool := o.tools.Get(tc.Name)
			if tool == nil {
				toolErr = fmt.Errorf("tool not found: %s", tc.Name)
			} else {
				var result map[string]interface{}
				result, toolErr = tool.Execute(ctx, args)
				if toolErr == nil {
					if out, ok := result["output"]; ok {
						output = fmt.Sprintf("%v", out)
					} else {
						outJSON, _ := json.Marshal(result)
						output = string(outJSON)
					}
				}
			}

			durationMs := time.Since(stepStart).Milliseconds()

			// Emit tool_result event
			resultData, _ := json.Marshal(map[string]interface{}{
				"tool_call_id": tc.ID,
				"tool_name":    tc.Name,
				"output":       output,
				"error":        map[bool]interface{}{true: toolErr.Error(), false: nil}[toolErr != nil],
				"duration_ms":  durationMs,
				"session_id":   sessionID,
			})
			o.hub.Publish(broadcast.Event{Type: "tool_result", Data: json.RawMessage(resultData)})

			// Also append inline markdown for display
			status := "✅"
			if toolErr != nil {
				status = "❌"
			}
			o.hub.Publish(broadcast.Event{Type: "text", Data: map[string]string{"content": fmt.Sprintf("\n\n_%s Tool: %s (%dms)_\n", status, tc.Name, durationMs)}})

			// Add tool result message for next LLM call
			resultContent := output
			if toolErr != nil {
				resultContent = "Error: " + toolErr.Error()
			}
			allMsgs = append(allMsgs, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Name,
				Content:    resultContent,
			})
		}

		slog.Info("turn completed",
			"turn", turn+1,
			"tools_executed", executedTools,
			"input_tokens", resp.InputTokens,
			"output_tokens", resp.OutputTokens,
		)
	}

	// Build the final display content — includes tool annotations so
	// streaming display matches persisted content.
	var finalContent strings.Builder
	for i, msg := range allMsgs {
		if msg.Role == "assistant" && msg.Content != "" {
			if finalContent.Len() > 0 {
				finalContent.WriteString("\n\n")
			}
			finalContent.WriteString(msg.Content)
		}
		// After each assistant message with tool_calls, check for
		// the corresponding tool messages and add annotations.
		// The tool messages immediately follow the assistant message.
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			// Look ahead for tool results (they come right after)
			for j := i + 1; j < len(allMsgs) && j < i+1+len(msg.ToolCalls); j++ {
				if allMsgs[j].Role == "tool" {
					// Find which tool call this corresponds to
					tcID := allMsgs[j].ToolCallID
					tcName := allMsgs[j].Name
					isErr := strings.HasPrefix(allMsgs[j].Content, "Error:")
					status := "✅"
					if isErr { status = "❌" }
					annotation := fmt.Sprintf("\n\n_%s Tool: %s_\n", status, tcName)
					finalContent.WriteString(annotation)
					_ = tcID
					// Include error message if present
					if isErr {
						finalContent.WriteString(fmt.Sprintf("_Error: %s_\n\n", strings.TrimPrefix(allMsgs[j].Content, "Error: ")))
					}
				}
			}
		}
	}

	return finalContent.String(), &llm.Usage{InputTokens: totalInput, OutputTokens: totalOutput}, nil
}

// BuildToolDefs converts the tool registry into LLM tool definitions.
func BuildToolDefs(registry *tools.ToolRegistry) []llm.ToolDef {
	list := registry.List()
	defs := make([]llm.ToolDef, 0, len(list))
	for _, t := range list {
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		})
	}
	return defs
}
