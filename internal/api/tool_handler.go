package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/tools"
)

// ToolHandler handles tool listing and execution.
type ToolHandler struct {
	registry *tools.ToolRegistry
}

func NewToolHandler(registry *tools.ToolRegistry) *ToolHandler {
	return &ToolHandler{registry: registry}
}

// ListTools returns all registered tools.
func (h *ToolHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	list := h.registry.List()
	type toolInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	result := make([]toolInfo, 0, len(list))
	for _, t := range list {
		result = append(result, toolInfo{Name: t.Name(), Description: t.Description()})
	}
	OK(w, map[string]interface{}{"tools": result})
}

// ExecuteTool runs a tool by name with the given input.
func (h *ToolHandler) ExecuteTool(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string                 `json:"name"`
		Input     map[string]interface{} `json:"input"`
		SessionID string                 `json:"session_id"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request body")
		return
	}
	if body.Name == "" {
		BadRequest(w, "tool name is required")
		return
	}
	if body.Input == nil {
		body.Input = make(map[string]interface{})
	}

	start := time.Now()
	result, err := h.registry.Execute(r.Context(), body.Name, body.Input)
	elapsed := time.Since(start)

	// Record tool execution in DB
	toolCallID := recordToolCall(r, body.Name, body.Input, body.SessionID, result, err, elapsed)

	// Add tool_call_id to result for reference
	if err != nil {
		resp := map[string]interface{}{
			"error": err.Error(),
		}
		if toolCallID != "" {
			resp["tool_call_id"] = toolCallID
		}
		InternalError(w, err.Error())
		return
	}

	if result == nil {
		result = make(map[string]interface{})
	}
	result["tool_call_id"] = toolCallID
	OK(w, result)
}

func recordToolCall(r *http.Request, toolName string, input map[string]interface{}, sessionID string, output map[string]interface{}, execErr error, elapsed time.Duration) string {
	if db.Pool == nil {
		return ""
	}

	inputJSON, _ := json.Marshal(input)
	outputStr := ""
	if output != nil {
		if out, ok := output["output"]; ok {
			outputStr = fmt.Sprintf("%v", out)
		} else {
			outJSON, _ := json.Marshal(output)
			outputStr = string(outJSON)
		}
	}
	if execErr != nil {
		outputStr = execErr.Error()
	}

	durationMs := elapsed.Milliseconds()

	// Generate a unique ID
	id := fmt.Sprintf("tc_%d", time.Now().UnixNano())

	_, err := db.Pool.Exec(r.Context(),
		`INSERT INTO tool_calls (id, session_id, tool_name, input, output, is_error, duration_ms, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
		id, nullableStr(sessionID), toolName, string(inputJSON), outputStr, execErr != nil, durationMs)
	if err != nil {
		slog.Warn("record tool call failed", "error", err)
		return ""
	}
	return id
}


