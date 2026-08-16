package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/engine"
	"github.com/athenavi/minicc/internal/monitor"
)

// ToolHandler handles tool listing and execution (Python-backed).
type ToolHandler struct {
	python        *engine.PythonClient
	authenticator *auth.Authenticator
}

func NewToolHandler(python *engine.PythonClient, authenticator *auth.Authenticator) *ToolHandler {
	return &ToolHandler{python: python, authenticator: authenticator}
}

// ListTools returns tools from Python service only.
func (h *ToolHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	type toolInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	result := make([]toolInfo, 0)
	if h.python != nil && h.python.IsConnected() {
		var py struct {
			Tools []struct {
				Function struct {
					Name        string `json:"name"`
					Description string `json:"description"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := h.python.GetJSON(r.Context(), "/v1/tools", &py); err == nil {
			for _, t := range py.Tools {
				result = append(result, toolInfo{Name: t.Function.Name, Description: t.Function.Description})
			}
		} else {
			slog.Warn("tool list: python fallback failed", "error", err)
		}
	}
	OK(w, map[string]interface{}{"tools": result})
}

// ExecuteTool proxies execution to Python service.
func (h *ToolHandler) ExecuteTool(w http.ResponseWriter, r *http.Request) {
	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		Unauthorized(w, "authentication required")
		return
	}

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

	monitor.IncToolCall()
	start := time.Now()

	if h.python == nil || !h.python.IsConnected() {
		BadRequest(w, "tool not available: "+body.Name)
		return
	}
	req := struct {
		Name  string                 `json:"name"`
		Input map[string]interface{} `json:"input"`
	}{Name: body.Name, Input: body.Input}
	var pyResult map[string]interface{}
	if err := h.python.PostJSON(r.Context(), "/v1/tools/execute", req, &pyResult); err != nil {
		slog.Warn("tool execute: python proxy failed", "tool", body.Name, "error", err)
		monitor.IncToolError()
		InternalError(w, "python tool execution failed")
		return
	}
	elapsed := time.Since(start)
	_ = recordToolCall(r, body.Name, body.Input, body.SessionID, pyResult, nil, elapsed)
	if pyResult == nil {
		pyResult = make(map[string]interface{})
	}
	OK(w, pyResult)
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
