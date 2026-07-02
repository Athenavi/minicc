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

// Engine is the ReAct agent: Thought → Action → Observation → Thought... → Final Answer.
// It integrates LLM, tools, working memory, episodic memory, and continuous planning.
type Engine struct {
	llm      *llm.Gateway
	tools    *tools.ToolRegistry
	hub      *broadcast.Hub
	episodic *EpisodicMemory
	planner  *Planner
}

func New(llmGateway *llm.Gateway, toolRegistry *tools.ToolRegistry) *Engine {
	return &Engine{
		llm:      llmGateway,
		tools:    toolRegistry,
		episodic: NewEpisodicMemory(100),
		planner:  NewPlanner(),
	}
}

func (e *Engine) SetHub(hub *broadcast.Hub)         { e.hub = hub }
func (e *Engine) EpisodicMemory() *EpisodicMemory    { return e.episodic }

// SystemPrompt returns the ReAct system prompt with memory and planning context.
func (e *Engine) SystemPrompt() string {
	return `You are MiniCC V2, an intelligent agent with:
1. **Tools** — Use them to interact with the environment
2. **Memory** — You have working memory for the current task and episodic memory of past tasks
3. **Planning** — You can decompose complex tasks into steps

Operate in a ReAct (Reasoning + Acting) cycle:

**Thought**: Analyze the current state and decide the next action
**Action**: Call a tool with precise parameters
**Observation**: Review the result and update your understanding
**Repeat** until the task is complete

Guidelines:
- Think step by step. Break complex tasks into smaller actions.
- Use the available tools. Each tool has specific parameters.
- After all tool calls are done, summarize what was accomplished.
- If something fails, try an alternative approach.`
}

// ExecuteTask runs the full ReAct lifecycle with memory and planning.
func (e *Engine) ExecuteTask(ctx context.Context, task string, sessionID string, history []llm.Message) (string, *llm.Usage, error) {
	if e.llm == nil {
		return "", nil, fmt.Errorf("LLM gateway not configured")
	}

	wm := NewWorkingMemory(sessionID, task)
	wm.AddThought("Task received: " + task)

	// Check episodic memory
	pastEpisodes := e.episodic.FindByTool("")
	if len(pastEpisodes) > 0 {
		last := pastEpisodes[len(pastEpisodes)-1]
		wm.Add("context", fmt.Sprintf("Previous similar task: %s (%s)", last.Task, last.Summary), nil)
	}

	// Build tool list for context
	toolList := e.tools.List()
	toolDescs := make([]string, len(toolList))
	toolNames := make([]string, len(toolList))
	for i, t := range toolList {
		toolNames[i] = t.Name()
		toolDescs[i] = fmt.Sprintf("  - %s: %s", t.Name(), t.Description())
	}
	plan := e.planner.CreatePlan(task, toolNames)
	wm.SetState("plan", plan.Progress())

	toolDefs := BuildToolDefs(e.tools)

	sysMsg := e.SystemPrompt() + "\n\nAvailable tools:\n" + strings.Join(toolDescs, "\n")
	allMsgs := make([]llm.Message, 0, len(history)+2)
	allMsgs = append(allMsgs, llm.Message{Role: "system", Content: sysMsg})
	allMsgs = append(allMsgs, history...)
	allMsgs = append(allMsgs, llm.Message{Role: "user", Content: task})

	var totalInput, totalOutput int
	maxTurns := 10
	turnStart := time.Now()

	for turn := 0; turn < maxTurns; turn++ {
		slog.Debug("react turn", "turn", turn+1, "messages", len(allMsgs))

		if turn > 0 && turn%3 == 0 {
			allMsgs = append(allMsgs, llm.Message{
				Role: "system", Content: "Progress so far:\n" + wm.Summarize(),
			})
		}

		finalContent, usage, err := e.executeTurn(ctx, sessionID, &allMsgs, toolDefs, wm)
		if err != nil {
			return finalContent, &llm.Usage{InputTokens: totalInput, OutputTokens: totalOutput}, err
		}

		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens

		lastMsg := allMsgs[len(allMsgs)-1]
		if lastMsg.Role == "assistant" && len(lastMsg.ToolCalls) == 0 {
			e.episodic.Record(Episode{
				ID: fmt.Sprintf("ep_%d", time.Now().UnixNano()), Task: task,
				Summary: truncate(finalContent, 200), ToolsUsed: toolNames,
				Success: true, Duration: time.Since(turnStart), CreatedAt: time.Now(),
			})
			return finalContent, &llm.Usage{InputTokens: totalInput, OutputTokens: totalOutput}, nil
		}
		plan.CompleteStep(fmt.Sprintf("step_%d", turn+1), truncate(finalContent, 100))
		wm.AddThought("Completed step: " + truncate(finalContent, 100))
	}

	e.episodic.Record(Episode{
		ID: fmt.Sprintf("ep_%d", time.Now().UnixNano()), Task: task,
		Summary: "Max turns reached", ToolsUsed: toolNames,
		Success: false, Duration: time.Since(turnStart), CreatedAt: time.Now(),
	})
	return "I've reached the maximum number of reasoning steps.", &llm.Usage{InputTokens: totalInput, OutputTokens: totalOutput}, nil
}

// executeTurn runs one ReAct turn: LLM → tool_calls → execute → feed back.
func (e *Engine) executeTurn(ctx context.Context, sessionID string, allMsgs *[]llm.Message, toolDefs []llm.ToolDef, wm *WorkingMemory) (string, *llm.Usage, error) {
	req := &llm.Request{
		Messages: *allMsgs, Tools: toolDefs,
		MaxTokens: 4096, Temperature: 0.7, Stream: true,
	}

	var assistantContent string
	resp, err := e.llm.ChatStream(ctx, req,
		func(chunk string) {
			assistantContent += chunk
			if e.hub != nil {
				e.hub.Publish(broadcast.Event{Type: "text", Data: map[string]string{"content": chunk}})
			}
		},
		func(tc llm.ToolCall) {
			wm.AddObservation(tc.Name, "Tool call requested: "+tc.Name, true)
			if e.hub != nil {
				data, _ := json.Marshal(map[string]string{
					"tool_call_id": tc.ID, "tool_name": tc.Name,
					"arguments": tc.Arguments, "session_id": sessionID,
				})
				e.hub.Publish(broadcast.Event{Type: "tool_use", Data: json.RawMessage(data)})
			}
		},
	)
	if err != nil {
		return assistantContent, &llm.Usage{}, err
	}

	assistantMsg := llm.Message{Role: "assistant", Content: assistantContent}
	if len(resp.ToolCalls) > 0 {
		assistantMsg.ToolCalls = resp.ToolCalls
	}
	*allMsgs = append(*allMsgs, assistantMsg)

	if len(resp.ToolCalls) == 0 {
		return assistantContent, &llm.Usage{
			InputTokens: resp.InputTokens, OutputTokens: resp.OutputTokens,
			TotalTokens: resp.InputTokens + resp.OutputTokens,
		}, nil
	}

	results := e.executeTools(ctx, sessionID, resp.ToolCalls)
	for _, r := range results {
		content := r.output
		if r.err != nil {
			content = "Error: " + r.err.Error()
			wm.AddObservation(r.name, r.err.Error(), false)
		} else {
			wm.AddObservation(r.name, truncate(r.output, 200), true)
		}
		*allMsgs = append(*allMsgs, llm.Message{
			Role: "tool", ToolCallID: r.toolCallID, Name: r.name, Content: content,
		})
	}

	return assistantContent, &llm.Usage{
		InputTokens: resp.InputTokens, OutputTokens: resp.OutputTokens,
		TotalTokens: resp.InputTokens + resp.OutputTokens,
	}, nil
}

type toolExecResult struct {
	toolCallID string
	name       string
	output     string
	err        error
	durationMs int64
}

func (e *Engine) executeTools(ctx context.Context, sessionID string, toolCalls []llm.ToolCall) []toolExecResult {
	type job struct {
		tc  llm.ToolCall
		res toolExecResult
	}
	ch := make(chan job, len(toolCalls))

	for _, tc := range toolCalls {
		tc := tc
		go func() {
			start := time.Now()
			if e.hub != nil {
				data, _ := json.Marshal(map[string]string{
					"tool_call_id": tc.ID, "tool_name": tc.Name,
					"arguments": tc.Arguments, "session_id": sessionID, "status": "running",
				})
				e.hub.Publish(broadcast.Event{Type: "tool_start", Data: json.RawMessage(data)})
			}

			var args map[string]interface{}
			json.Unmarshal([]byte(tc.Arguments), &args)
			var output string
			var toolErr error
			tool := e.tools.Get(tc.Name)
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

			ms := time.Since(start).Milliseconds()
			if e.hub != nil {
				resultData, _ := json.Marshal(map[string]interface{}{
					"tool_call_id": tc.ID, "tool_name": tc.Name, "output": output,
					"error": map[bool]interface{}{true: toolErr.Error(), false: nil}[toolErr != nil],
					"duration_ms": ms, "session_id": sessionID,
				})
				e.hub.Publish(broadcast.Event{Type: "tool_result", Data: json.RawMessage(resultData)})
				status := "✅"
				if toolErr != nil {
					status = "❌"
				}
				e.hub.Publish(broadcast.Event{Type: "text", Data: map[string]string{
					"content": fmt.Sprintf("\n\n_%s Tool: %s (%dms)_\n", status, tc.Name, ms),
				}})
			}

			ch <- job{tc: tc, res: toolExecResult{
				toolCallID: tc.ID, name: tc.Name, output: output, err: toolErr, durationMs: ms,
			}}
		}()
	}

	results := make([]toolExecResult, len(toolCalls))
	for i := 0; i < len(toolCalls); i++ {
		j := <-ch
		results[i] = j.res
	}
	return results
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
