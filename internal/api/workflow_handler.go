package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/tools"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// WorkflowHandler provides REST API for workflow definitions and executions.
type WorkflowHandler struct {
	toolRegistry *tools.ToolRegistry
	authenticator *auth.Authenticator
}

func NewWorkflowHandler(tr *tools.ToolRegistry, a *auth.Authenticator) *WorkflowHandler {
	return &WorkflowHandler{toolRegistry: tr, authenticator: a}
}

// ── Definitions ──

func (h *WorkflowHandler) ListDefs(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		OK(w, []map[string]interface{}{})
		return
	}

	rows, err := db.Pool.Query(r.Context(),
		`SELECT id, name, COALESCE(description, ''), version, enabled, created_at, updated_at
		 FROM workflow_definitions
		 ORDER BY updated_at DESC
		 LIMIT 50`)
	if err != nil {
		OK(w, []map[string]interface{}{})
		return
	}
	defer rows.Close()

	defs := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, name, desc, version string
		var enabled bool
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &name, &desc, &version, &enabled, &createdAt, &updatedAt); err != nil {
			continue
		}
		defs = append(defs, map[string]interface{}{
			"id": id, "name": name, "description": desc,
			"version": version, "enabled": enabled,
			"created_at": createdAt, "updated_at": updatedAt,
		})
	}
	OK(w, defs)
}

func (h *WorkflowHandler) GetDef(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		BadRequest(w, "id required")
		return
	}

	var name, desc, version string
	var enabled bool
	var defJSON string
	var createdAt, updatedAt time.Time
	err := db.Pool.QueryRow(r.Context(),
		`SELECT id, name, COALESCE(description, ''), version, enabled, definition::text, created_at, updated_at
		 FROM workflow_definitions WHERE id = $1`, id).
		Scan(&id, &name, &desc, &version, &enabled, &defJSON, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		NotFound(w, "workflow not found")
		return
	} else if err != nil {
		InternalError(w, err.Error())
		return
	}

	var def interface{}
	json.Unmarshal([]byte(defJSON), &def)

	OK(w, map[string]interface{}{
		"id": id, "name": name, "description": desc,
		"version": version, "enabled": enabled,
		"definition": def,
		"created_at": createdAt, "updated_at": updatedAt,
	})
}

func (h *WorkflowHandler) CreateDef(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	// Optional auth — use user_id from JWT if available
	var userID *string
	claims := getAuthClaims(r, h.authenticator)
	if claims != nil {
		userID = &claims.UserID
	}

	var body struct {
		ID          string      `json:"id"`
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Definition  interface{} `json:"definition"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.Name == "" {
		BadRequest(w, "name is required")
		return
	}

	defID := body.ID
	if defID == "" {
		defID = fmt.Sprintf("wf_%d", time.Now().UnixNano())
	}

	defJSON, _ := json.Marshal(body.Definition)

	_, err := db.Pool.Exec(r.Context(),
		`INSERT INTO workflow_definitions (id, user_id, name, description, version, definition, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, '1.0', $5, true, NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description,
		   definition = EXCLUDED.definition, updated_at = NOW()`,
		defID, userID, body.Name, body.Description, string(defJSON))
	if err != nil {
		InternalError(w, "create workflow: "+err.Error())
		return
	}

	OK(w, map[string]string{"id": defID})
}

func (h *WorkflowHandler) DeleteDef(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		BadRequest(w, "id required")
		return
	}
	db.Pool.Exec(r.Context(), `DELETE FROM workflow_definitions WHERE id = $1`, id)
	OK(w, map[string]string{"status": "deleted"})
}

// ── Executions ──

func (h *WorkflowHandler) Execute(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		NotFound(w, "database not available")
		return
	}

	defID := chi.URLParam(r, "id")
	if defID == "" {
		BadRequest(w, "definition id required")
		return
	}

	// Load definition
	var defJSON string
	err := db.Pool.QueryRow(r.Context(),
		`SELECT definition::text FROM workflow_definitions WHERE id = $1`, defID).
		Scan(&defJSON)
	if err == pgx.ErrNoRows {
		NotFound(w, "workflow not found")
		return
	} else if err != nil {
		InternalError(w, err.Error())
		return
	}

	// Parse steps from definition
	var def map[string]interface{}
	json.Unmarshal([]byte(defJSON), &def)

	stepsRaw, _ := def["steps"].([]interface{})

	// Create execution record
	execID := fmt.Sprintf("exec_%d", time.Now().UnixNano())
	now := time.Now()
	_, err = db.Pool.Exec(r.Context(),
		`INSERT INTO workflow_executions (id, definition_id, user_id, status, trigger, started_at, finished_at)
		 VALUES ($1, $2, NULL, 'running', 'manual', $3, NULL)`,
		execID, defID, now)
	if err != nil {
		InternalError(w, "create execution: "+err.Error())
		return
	}

	// Execute steps sequentially via tool registry
	results := make([]map[string]interface{}, 0)
	overallStatus := "completed"

	for _, stepRaw := range stepsRaw {
		step, ok := stepRaw.(map[string]interface{})
		if !ok {
			continue
		}
		toolName, _ := step["tool"].(string)
		if toolName == "" {
			continue
		}

		stepID, _ := step["id"].(string)
		params, _ := step["params"].(map[string]interface{})

		stepStart := time.Now()
		var stepOutput string
		var stepErr string

		tool := h.toolRegistry.Get(toolName)
		if tool == nil {
			stepErr = fmt.Sprintf("tool not found: %s", toolName)
			overallStatus = "failed"
		} else {
			res, err := tool.Execute(r.Context(), params)
			if err != nil {
				stepErr = err.Error()
				overallStatus = "failed"
			} else if out, ok := res["output"]; ok {
				stepOutput = fmt.Sprintf("%v", out)
			} else {
				outJSON, _ := json.Marshal(res)
				stepOutput = string(outJSON)
			}
		}

		stepDuration := time.Since(stepStart).Milliseconds()
		stepStatus := "success"
		if stepErr != "" {
			stepStatus = "failed"
		}

		results = append(results, map[string]interface{}{
			"id": stepID, "tool": toolName, "status": stepStatus,
			"output": stepOutput, "error": stepErr,
			"duration_ms": stepDuration,
		})
	}

	// Update execution record
	execDuration := time.Since(now).Milliseconds()
	resultsJSON, _ := json.Marshal(results)
	db.Pool.Exec(r.Context(),
		`UPDATE workflow_executions SET status = $1, output = $2, duration_ms = $3, finished_at = NOW()
		 WHERE id = $4`,
		overallStatus, string(resultsJSON), execDuration, execID)

	OK(w, map[string]interface{}{
		"execution_id": execID,
		"status":       overallStatus,
		"steps":        results,
		"duration_ms":  execDuration,
	})
}

func (h *WorkflowHandler) ListExecs(w http.ResponseWriter, r *http.Request) {
	if db.Pool == nil {
		OK(w, []map[string]interface{}{})
		return
	}
	defID := r.URL.Query().Get("definition_id")

	query := `SELECT id, definition_id, status, trigger, COALESCE(output, ''), COALESCE(error, ''), duration_ms, started_at, finished_at
		 FROM workflow_executions`
	args := []interface{}{}
	if defID != "" {
		query += ` WHERE definition_id = $1`
		args = append(args, defID)
	}
	query += ` ORDER BY started_at DESC LIMIT 50`

	rows, err := db.Pool.Query(r.Context(), query, args...)
	if err != nil {
		OK(w, []map[string]interface{}{})
		return
	}
	defer rows.Close()

	execs := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, defID2, status, trigger, output, errStr string
		var durationMs int64
		var startedAt, finishedAt *time.Time
		if err := rows.Scan(&id, &defID2, &status, &trigger, &output, &errStr, &durationMs, &startedAt, &finishedAt); err != nil {
			continue
		}
		execs = append(execs, map[string]interface{}{
			"id": id, "definition_id": defID2, "status": status,
			"trigger": trigger, "output": output, "error": errStr,
			"duration_ms": durationMs,
			"started_at": startedAt, "finished_at": finishedAt,
		})
	}
	OK(w, execs)
}
