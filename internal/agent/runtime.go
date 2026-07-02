package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/llm"
	"github.com/athenavi/minicc/internal/tools"
)

// AgentRuntime executes agent tasks with real-time SSE status updates.
type AgentRuntime struct {
	llm     *llm.Gateway
	tools   *tools.ToolRegistry
	hub     *broadcast.Hub
}

func NewAgentRuntime(llm *llm.Gateway, tools *tools.ToolRegistry, hub *broadcast.Hub) *AgentRuntime {
	return &AgentRuntime{llm: llm, tools: tools, hub: hub}
}

// Agent session statuses
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// System prompts for each agent type (deterministic — no variable content, cache-friendly)
var agentPrompts = map[string]string{
	"code": "You are a Code Agent. Your job is to write, modify, and analyze code. " +
		"You have access to read_file and write_file tools. Always show the code you write.",
	"knowledge": "You are a Knowledge Agent. Your job is to search knowledge bases and documentation. " +
		"You can search wiki_pages and kb_articles. Summarize findings clearly.",
	"rpa": "You are an RPA Agent. Your job is to control browsers, automate tasks, " +
		"and execute enterprise tools. Use the available tools to complete the task.",
	"tool": "You are a Tool Agent. Your job is to call APIs and execute any registered tool. " +
		"Select the right tool for the task and report results.",
}

// Dispatch creates an agent session, executes the task, and updates status.
// Emits SSE events: agent_status, tool_use, tool_result, text.
func (ar *AgentRuntime) Dispatch(ctx context.Context, sessionID, userID, agentType, taskDesc string) {
	// 1. Create agent session in DB
	sessionID = agentSessionID(sessionID, agentType)

	if db.Pool != nil {
		db.Pool.Exec(ctx,
			`INSERT INTO agent_sessions (id, user_id, name, task, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 'running', NOW(), NOW())
			 ON CONFLICT (id) DO UPDATE SET status = 'running', task = EXCLUDED.task, updated_at = NOW()`,
			sessionID, userID, agentType+"_agent", taskDesc)
	}

	ar.emitStatus(sessionID, agentType, StatusRunning, taskDesc, "")

	// 2. Build agent-specific system prompt + tools
	systemPrompt := agentPrompts[agentType]
	if systemPrompt == "" {
		systemPrompt = agentPrompts["tool"]
	}

	// Combine task description into the user message
	taskMessage := fmt.Sprintf("Task: %s\n\nPlease complete this task using the available tools.", taskDesc)

	messages := []llm.Message{
		{Role: "user", Content: taskMessage},
	}

	// 3. Execute via LLM with tools
	allToolDefs := buildAgentToolDefs(ar.tools)
	var fullContent string

	req := &llm.Request{
		Messages:    []llm.Message{{Role: "system", Content: systemPrompt}},
		Tools:       allToolDefs,
		MaxTokens:   8192,
		Temperature: 0.7,
		Stream:      true,
	}
	// Append user message
	req.Messages = append(req.Messages, messages...)

	// Use a simple turn loop (up to 5 turns for agent tasks)
	for turn := 0; turn < 5; turn++ {
		var turnContent string
		resp, err := ar.llm.ChatStream(ctx, req,
			func(chunk string) {
				turnContent += chunk
				fullContent += chunk
				ar.hub.Publish(broadcast.Event{Type: "text", Data: map[string]string{"content": chunk}})
			},
			func(tc llm.ToolCall) {
				data, _ := json.Marshal(map[string]string{
					"tool_call_id": tc.ID, "tool_name": tc.Name,
					"arguments": tc.Arguments, "session_id": sessionID,
				})
				ar.hub.Publish(broadcast.Event{Type: "tool_use", Data: json.RawMessage(data)})
			},
		)

		if err != nil {
			ar.failSession(ctx, sessionID, agentType, taskDesc, err.Error())
			return
		}

		// Add assistant message
		assistantMsg := llm.Message{Role: "assistant", Content: turnContent}
		if len(resp.ToolCalls) > 0 {
			assistantMsg.ToolCalls = resp.ToolCalls
		}
		req.Messages = append(req.Messages, assistantMsg)

		if len(resp.ToolCalls) == 0 {
			break // No tools needed - we're done
		}

		// Execute tools
		for _, tc := range resp.ToolCalls {
			var args map[string]interface{}
			json.Unmarshal([]byte(tc.Arguments), &args)

			tool := ar.tools.Get(tc.Name)
			var output string
			var toolErr error
			if tool == nil {
				toolErr = fmt.Errorf("tool not found: %s", tc.Name)
			} else {
				var result map[string]interface{}
				result, toolErr = tool.Execute(ctx, args)
				if toolErr == nil {
					if out, ok := result["output"]; ok { output = fmt.Sprintf("%v", out) } else { oJ, _ := json.Marshal(result); output = string(oJ) }
				}
			}

			// Emit tool_result
			durationMs := time.Since(time.Now()).Milliseconds()
			rd, _ := json.Marshal(map[string]interface{}{
				"tool_call_id": tc.ID, "tool_name": tc.Name,
				"output": output, "error": map[bool]interface{}{true: toolErr.Error(), false: nil}[toolErr != nil],
				"duration_ms": durationMs, "session_id": sessionID,
			})
			ar.hub.Publish(broadcast.Event{Type: "tool_result", Data: json.RawMessage(rd)})

			// Add tool result
			req.Messages = append(req.Messages, llm.Message{
				Role: "tool", ToolCallID: tc.ID, Name: tc.Name,
				Content: map[bool]string{true: "Error: " + toolErr.Error(), false: output}[toolErr != nil],
			})
		}
	}

	// 4. Complete
	ar.completeSession(ctx, sessionID, agentType, taskDesc, fullContent)
}

func (ar *AgentRuntime) emitStatus(sessionID, agentType, status, task, result string) {
	data, _ := json.Marshal(map[string]string{
		"session_id": sessionID, "agent_type": agentType,
		"status": status, "task": task, "result": result,
	})
	ar.hub.Publish(broadcast.Event{Type: "agent_status", Data: json.RawMessage(data)})
}

func (ar *AgentRuntime) completeSession(ctx context.Context, sessionID, agentType, task, result string) {
	if db.Pool != nil {
		db.Pool.Exec(ctx, `UPDATE agent_sessions SET status = 'completed', result = $1, updated_at = NOW() WHERE id = $2`, result, sessionID)
	}
	ar.emitStatus(sessionID, agentType, StatusCompleted, task, result)
	// Also emit turn_done so the chat flow completes
	ar.hub.Publish(broadcast.Event{Type: "turn_done", Data: map[string]string{"session_id": sessionID}})
}

func (ar *AgentRuntime) failSession(ctx context.Context, sessionID, agentType, task, errMsg string) {
	if db.Pool != nil {
		db.Pool.Exec(ctx, `UPDATE agent_sessions SET status = 'failed', result = $1, updated_at = NOW() WHERE id = $2`, errMsg, sessionID)
	}
	ar.emitStatus(sessionID, agentType, StatusFailed, task, errMsg)
	ar.hub.Publish(broadcast.Event{Type: "turn_done", Data: map[string]string{"session_id": sessionID}})
}

func agentSessionID(sessionID, agentType string) string {
	return fmt.Sprintf("%s_%s", sessionID, agentType)
}

func buildAgentToolDefs(registry *tools.ToolRegistry) []llm.ToolDef {
	list := registry.List()
	defs := make([]llm.ToolDef, 0, len(list))
	for _, t := range list {
		defs = append(defs, llm.ToolDef{
			Name: t.Name(), Description: t.Description(),
			Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		})
	}
	return defs
}
