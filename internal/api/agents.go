package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/engine"
	"github.com/athenavi/chiron/internal/id"
)

// AgentHandler 绠＄悊鑷畾涔?Agent锛圖B agents 琛級+ 杩愯浼氳瘽锛坅gent_sessions锛夈€?// 鎵ц閾捐矾锛歊un 钀?session(pending) 鈫?寮傛璋?Python /v1/agents/dispatch
// 锛圥ython 鐢?SubAgent 鐪熸墽琛岋級鈫?缁撴灉鍥炲啓 session(completed/failed)銆?type AgentHandler struct {
	authenticator *auth.Authenticator
	pythonClient  *engine.PythonClient
	sem           chan struct{} // 骞跺彂鎵ц涓婇檺锛堜笌 /submit 鐨?agentSem 鍚屾簮锛?}

func NewAgentHandler(a *auth.Authenticator, pc *engine.PythonClient, sem chan struct{}) *AgentHandler {
	h := &AgentHandler{authenticator: a, pythonClient: pc, sem: sem}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("seed preset agents failed", "panic", r)
			}
		}()
		h.seedPresetAgents()
	}()
	return h
}

// Agent 鏄嚜瀹氫箟 Agent 鐨?DB 琛ㄧず銆?type Agent struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	SystemPrompt   string          `json:"system_prompt,omitempty"`
	Tools          json.RawMessage `json:"tools,omitempty"`
	LLMConfig      json.RawMessage `json:"llm_config,omitempty"`
	MaxTurns       int             `json:"max_turns"`
	TimeoutSeconds int             `json:"timeout_seconds"`
	Enabled        bool            `json:"enabled"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// AgentSession 鏄竴娆?Agent 杩愯鐨勬寔涔呭寲璁板綍銆?type AgentSession struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id,omitempty"`
	AgentName string    `json:"agent_name,omitempty"`
	Task      string    `json:"task"`
	Status    string    `json:"status"` // pending / running / completed / failed
	Result    string    `json:"result,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// 鈹€鈹€ 棰勭疆 Agent 鎾锛圖B agents 琛ㄤ负绌烘椂鎻掑叆鍐呯疆 3 绫伙級 鈹€鈹€

type presetAgent struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Prompt      string         `json:"prompt"`
	Tools       []map[string]any `json:"tools"`
	LLM         map[string]any `json:"llm"`
	Turns       int            `json:"turns"`
}

// loadPresetAgents 浠?configs/preset_agents.json 鍔犺浇棰勭疆 Agent 瀹氫箟銆?// 鏂囦欢涓嶅瓨鍦ㄦ椂杩斿洖绌哄垪琛紙涓嶆挱绉嶄换浣曢缃?Agent锛夈€?func loadPresetAgents() []presetAgent {
	candidates := []string{
		"configs/preset_agents.json",
		"/etc/chiron/preset_agents.json",
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var presets []presetAgent
		if err := json.Unmarshal(data, &presets); err != nil {
			slog.Warn("parse preset agents config failed", "path", path, "error", err)
			return nil
		}
		slog.Info("loaded preset agents from config", "path", path, "count", len(presets))
		return presets
	}
	slog.Warn("preset agents config not found 鈥?no preset agents will be seeded")
	return nil
}

func (h *AgentHandler) seedPresetAgents() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ownerTenantID, err := h.resolveOwnerTenantID(ctx)
	if err != nil {
		slog.Warn("seed preset agents: resolve owner tenant failed", "error", err)
		return
	}

	var n int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM agents WHERE tenant_id = $1`, ownerTenantID).Scan(&n); err != nil || n > 0 {
		return
	}
	var ownerUserID string
	if err := db.Pool.QueryRow(ctx, `SELECT id::text FROM users WHERE tenant_id = $1 AND role = 'owner' ORDER BY created_at LIMIT 1`, ownerTenantID).Scan(&ownerUserID); err != nil || ownerUserID == "" {
		slog.Warn("seed preset agents: no owner user", "tenant", ownerTenantID)
		return
	}

	presets := loadPresetAgents()
	if len(presets) == 0 {
		return
	}

	for _, p := range presets {
		agentID, err := id.UUID()
		if err != nil {
			slog.Warn("seed preset agent: generate id", "name", p.Name, "error", err)
			continue
		}
		toolsJSON, _ := json.Marshal(p.Tools)
		llmJSON, _ := json.Marshal(p.LLM)
		if _, err := db.Pool.Exec(ctx,
			`INSERT INTO agents (id, tenant_id, user_id, name, description, system_prompt, tools, llm_config, max_turns, timeout_seconds, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true)`,
			agentID, ownerTenantID, ownerUserID, p.Name, p.Description, p.Prompt,
			string(toolsJSON), string(llmJSON), p.Turns, 120); err != nil {
			slog.Warn("seed preset agent", "name", p.Name, "error", err)
		}
	}
}

// 鈹€鈹€ CRUD 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// List 杩斿洖褰撳墠绉熸埛鐨勫叏閮?Agent锛堟寜鍒涘缓鏃堕棿鍊掑簭锛夈€?func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	rows, err := db.Pool.Query(r.Context(),
		`SELECT id::text, name, COALESCE(description,''), COALESCE(system_prompt,''), COALESCE(tools,'[]'::jsonb), COALESCE(llm_config,'{}'::jsonb), max_turns, timeout_seconds, enabled, created_at, updated_at
		 FROM agents WHERE tenant_id = $1 AND (user_id = $2 OR (visibility = 'tenant' AND tenant_id = $1)) ORDER BY created_at DESC`, claims.TenantID, claims.UserID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list agents failed")
		return
	}
	defer rows.Close()

	agents := make([]Agent, 0, 8)
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.SystemPrompt, &a.Tools, &a.LLMConfig,
			&a.MaxTurns, &a.TimeoutSeconds, &a.Enabled, &a.CreatedAt, &a.UpdatedAt); err != nil {
			slog.Warn("scan agent", "error", err)
			continue
		}
		agents = append(agents, a)
	}
	OK(w, agents)
}

// Create 鏂板缓涓€涓?Agent锛堢粦瀹氬綋鍓嶇鎴凤級銆?func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	var body Agent
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	body.Name = trimSpace(body.Name)
	if body.Name == "" {
		BadRequest(w, "name is required")
		return
	}

	toolsJSON := body.Tools
	if len(toolsJSON) == 0 || string(toolsJSON) == "null" {
		toolsJSON = json.RawMessage("[]")
	}
	llmJSON := body.LLMConfig
	if len(llmJSON) == 0 || string(llmJSON) == "null" {
		llmJSON = json.RawMessage("{}")
	}
	if body.MaxTurns <= 0 {
		body.MaxTurns = 5
	}
	if body.TimeoutSeconds <= 0 {
		body.TimeoutSeconds = 120
	}

	id, err := id.UUID()
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "generate id failed")
		return
	}
	_, err = db.Pool.Exec(r.Context(),
		`INSERT INTO agents (id, tenant_id, user_id, name, description, system_prompt, tools, llm_config, max_turns, timeout_seconds, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		id, claims.TenantID, claims.UserID, body.Name, body.Description, body.SystemPrompt,
		string(toolsJSON), string(llmJSON), body.MaxTurns, body.TimeoutSeconds, body.Enabled)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create agent failed")
		return
	}
	body.ID = id
	body.CreatedAt = time.Now()
	body.UpdatedAt = time.Now()
	OK(w, body)
}

// Get 杩斿洖鍗曚釜 Agent锛堝繀椤诲綊灞炲綋鍓嶇鎴凤級銆?func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	agentID := r.PathValue("id")
	if agentID == "" {
		BadRequest(w, "id is required")
		return
	}
	a, err := h.queryAgent(r.Context(), claims.TenantID, claims.UserID, agentID)
	if err != nil {
		NotFound(w, "agent not found")
		return
	}
	OK(w, a)
}

// Update 鏇存柊 Agent 瀛楁锛坣ame/description/system_prompt/tools/llm_config/max_turns/timeout_seconds/enabled锛夈€?func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	agentID := r.PathValue("id")
	if agentID == "" {
		BadRequest(w, "id is required")
		return
	}
	// P1 淇锛氭敼鐢ㄦ寚閽堝瓧娈垫寜闇€鏇存柊鈥斺€斿師瀹炵幇 description/system_prompt 鎴愬瑕嗙洊
	// 锛堝彧浼犲叾涓€娓呯┖鍙︿竴涓級锛屼笖 enabled 鏃犳潯浠跺啓鍏ワ紙涓嶄紶鍗宠閲嶇疆涓?false锛夈€?	var body struct {
		Name           *string         `json:"name"`
		Description    *string         `json:"description"`
		SystemPrompt   *string         `json:"system_prompt"`
		Tools          json.RawMessage `json:"tools"`
		LLMConfig      json.RawMessage `json:"llm_config"`
		MaxTurns       *int            `json:"max_turns"`
		TimeoutSeconds *int            `json:"timeout_seconds"`
		Enabled        *bool           `json:"enabled"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}

	// 鍔ㄦ€?SET锛氶潪闆跺瓧娈垫墠鏇存柊锛堥伩鍏嶆妸绌哄€煎綋鈥滄竻闄も€濓級
	sets := []string{}
	args := []any{}
	push := func(expr string, v any) {
		sets = append(sets, expr)
		args = append(args, v)
	}
	if body.Name != nil {
		push("name = $"+itoa(len(args)+1), *body.Name)
	}
	if body.Description != nil {
		push("description = $"+itoa(len(args)+1), *body.Description)
	}
	if body.SystemPrompt != nil {
		push("system_prompt = $"+itoa(len(args)+1), *body.SystemPrompt)
	}
	if len(body.Tools) > 0 && string(body.Tools) != "null" {
		push("tools = $"+itoa(len(args)+1), string(body.Tools))
	}
	if len(body.LLMConfig) > 0 && string(body.LLMConfig) != "null" {
		push("llm_config = $"+itoa(len(args)+1), string(body.LLMConfig))
	}
	if body.MaxTurns != nil {
		push("max_turns = $"+itoa(len(args)+1), *body.MaxTurns)
	}
	if body.TimeoutSeconds != nil {
		push("timeout_seconds = $"+itoa(len(args)+1), *body.TimeoutSeconds)
	}
	if body.Enabled != nil {
		push("enabled = $"+itoa(len(args)+1), *body.Enabled)
	}
	// WHERE tenant_id = $N+1 AND id = $N+2 鈥斺€?鍙岄噸鏍￠獙闃茶法绉熸埛
	args = append(args, claims.TenantID, agentID)

	if len(sets) == 0 {
		BadRequest(w, "nothing to update")
		return
	}
	if _, err := db.Pool.Exec(r.Context(),
		`UPDATE agents SET `+joinComma(sets)+`, updated_at = NOW() WHERE tenant_id = $`+itoa(len(args)-1)+` AND id = $`+itoa(len(args))+" AND user_id = $"+itoa(len(args)+1), append(args, claims.UserID)...); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update agent failed")
		return
	}
	a, err := h.queryAgent(r.Context(), claims.TenantID, claims.UserID, agentID)
	if err != nil {
		NotFound(w, "agent not found")
		return
	}
	OK(w, a)
}

// Delete 鍒犻櫎 Agent 鍙婂叾杩愯璁板綍锛堜粎褰撳綊灞炲綋鍓嶇鎴凤級銆?func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	agentID := r.PathValue("id")
	if agentID == "" {
		BadRequest(w, "id is required")
		return
	}
	if _, err := db.Pool.Exec(r.Context(), `DELETE FROM agents WHERE tenant_id = $1 AND id = $2 AND user_id = $3`, claims.TenantID, agentID, claims.UserID); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "delete agent failed")
		return
	}
	OK(w, map[string]string{"status": "deleted"})
}

// 鈹€鈹€ 杩愯涓庝細璇?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// Run 娲惧彂浠诲姟缁?Agent锛氳惤 session(pending) 鍚庡紓姝ユ墽琛岋紝缁撴灉鍥炲啓銆?func (h *AgentHandler) Run(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		BadRequest(w, "id is required")
		return
	}
	claims := auth.GetClaims(r.Context())
	var body struct {
		Task string `json:"task"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	body.Task = trimSpace(body.Task)
	if body.Task == "" {
		BadRequest(w, "task is required")
		return
	}

	agent, err := h.queryAgent(r.Context(), claims.TenantID, claims.UserID, agentID)
	if err != nil {
		NotFound(w, "agent not found")
		return
	}
	if !agent.Enabled {
		BadRequest(w, "agent is disabled")
		return
	}

	sessionID, err := id.UUID()
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "generate id failed")
		return
	}
	now := time.Now()
	if _, err := db.Pool.Exec(r.Context(),
		`INSERT INTO agent_sessions (id, tenant_id, user_id, agent_id, name, task, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7, $7)`,
		sessionID, claims.TenantID, claims.UserID, agent.ID, agent.Name, body.Task, now); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create session failed")
		return
	}

	timeout := time.Duration(agent.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	// P1 淇锛氭墽琛屽墠鑾峰彇骞跺彂淇″彿閲忥紝闃叉鏃犱笂闄愬苟鍙戞墦鐖嗗紩鎿?	if h.sem != nil {
		h.sem <- struct{}{}
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("agent execution panic", "session", sessionID, "agent", agentID, "panic", r)
				// Mark session as failed on panic
				updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer updateCancel()
				_, _ = db.Pool.Exec(updateCtx,
					`UPDATE agent_sessions SET status = 'failed', result = $1, updated_at = NOW() WHERE id = $2`,
					fmt.Sprintf(`{"error":"agent execution panicked: %v"}`, r), sessionID)
			}
			if h.sem != nil {
				<-h.sem
			}
		}()
		h.executeAgent(agent, body.Task, sessionID, claims.UserID, claims.TenantID, timeout)
	}()

	OK(w, AgentSession{
		ID:        sessionID,
		AgentID:   agent.ID,
		AgentName: agent.Name,
		Task:      body.Task,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (h *AgentHandler) executeAgent(agent *Agent, task, sessionID, userID, tenantID string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, _ = db.Pool.Exec(ctx, `UPDATE agent_sessions SET status = 'running', updated_at = NOW() WHERE id = $1`, sessionID)

	// tools/llm_config 杞?map 浼犵粰 Python锛坱ools 淇濇寔 []map 缁撴瀯锛?	var tools []map[string]any
	if len(agent.Tools) > 0 && string(agent.Tools) != "[]" {
		_ = json.Unmarshal(agent.Tools, &tools)
	}
	var llm map[string]any
	if len(agent.LLMConfig) > 0 && string(agent.LLMConfig) != "{}" {
		_ = json.Unmarshal(agent.LLMConfig, &llm)
	}

	body := map[string]any{
		"task":          task,
		"name":          agent.Name,
		"description":   agent.Description,
		"system_prompt": agent.SystemPrompt,
		"tools":         tools,
		"model":         llmString(llm, "model", "deepseek-chat"),
		"max_turns":     agent.MaxTurns,
		"max_tokens":    llmInt(llm, "max_tokens", 4096),
		"temperature":   llmFloat(llm, "temperature", 0.6),
		"tenant_id":     tenantID, // S 澶氱鎴烽殧绂?鐢?JWT claims 鐨?TenantID,涓嶈兘鐢?userID
		"user_id":       userID,
		"session_id":    sessionID,
	}

	var result any
	if h.pythonClient == nil {
		result = map[string]any{"success": false, "error": "python engine not available", "output": ""}
	} else if err := h.pythonClient.PostJSON(ctx, "/v1/agents/dispatch", body, &result); err != nil {
		result = map[string]any{"success": false, "error": err.Error(), "output": ""}
	}

	resultJSON, _ := json.Marshal(result)
	status := "completed"
	if m, ok := result.(map[string]any); ok && !boolOf(m["success"]) {
		status = "failed"
	}
	_, _ = db.Pool.Exec(ctx,
		`UPDATE agent_sessions SET status = $1, result = $2, updated_at = NOW() WHERE id = $3`,
		status, string(resultJSON), sessionID)
}

// ListSessions 杩斿洖褰撳墠鐢ㄦ埛鍦ㄥ綋鍓嶇鎴蜂笅鐨勮繍琛岃褰曪紙鍊掑簭锛夈€?// SetVisibility 璁剧疆 Agent 鍏变韩鍙鎬э紙浠?owner锛夛細PUT /v1/agents/{id}/visibility
func (h *AgentHandler) SetVisibility(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
	}
	var body struct {
		Visibility string `json:"visibility"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.Visibility != "private" && body.Visibility != "tenant" {
		BadRequest(w, "visibility must be private or tenant")
		return
	}
	// owner-only锛氭洿鏂板繀椤诲懡涓?user_id
	tag, err := db.Pool.Exec(r.Context(),
		`UPDATE agents SET visibility = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND user_id = $4`,
		body.Visibility, r.PathValue("id"), claims.TenantID, claims.UserID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update visibility failed")
		return
	}
	if tag.RowsAffected() == 0 {
		Forbidden(w, "agent not found or not owned by you")
		return
	}
	OK(w, map[string]interface{}{"id": r.PathValue("id"), "visibility": body.Visibility})
}

func (h *AgentHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	rows, err := db.Pool.Query(r.Context(),
		`SELECT s.id, COALESCE(s.agent_id::text,''), COALESCE(a.name,''), s.task, s.status, COALESCE(s.result,''), s.created_at, s.updated_at
		 FROM agent_sessions s LEFT JOIN agents a ON a.id = s.agent_id
		 WHERE s.user_id = $1 AND s.tenant_id = $2 ORDER BY s.created_at DESC LIMIT 100`, claims.UserID, claims.TenantID)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list sessions failed")
		return
	}
	defer rows.Close()

	sessions := make([]AgentSession, 0, 16)
	for rows.Next() {
		var s AgentSession
		if err := rows.Scan(&s.ID, &s.AgentID, &s.AgentName, &s.Task, &s.Status, &s.Result, &s.CreatedAt, &s.UpdatedAt); err != nil {
			slog.Warn("scan agent session", "error", err)
			continue
		}
		sessions = append(sessions, s)
	}
	OK(w, sessions)
}

// GetSession 杩斿洖鍗曚釜杩愯璁板綍锛堝綊灞炴牎楠岋級銆?func (h *AgentHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		BadRequest(w, "id is required")
		return
	}
	claims := auth.GetClaims(r.Context())
	var s AgentSession
	err := db.Pool.QueryRow(r.Context(),
		`SELECT s.id, COALESCE(s.agent_id::text,''), COALESCE(a.name,''), s.task, s.status, COALESCE(s.result,''), s.created_at, s.updated_at
		 FROM agent_sessions s LEFT JOIN agents a ON a.id = s.agent_id
		 WHERE s.id = $1 AND s.user_id = $2 AND s.tenant_id = $3`, sessionID, claims.UserID, claims.TenantID).
		Scan(&s.ID, &s.AgentID, &s.AgentName, &s.Task, &s.Status, &s.Result, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		NotFound(w, "session not found")
		return
	}
	OK(w, s)
}

// 鈹€鈹€ helpers 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func (h *AgentHandler) queryAgent(ctx context.Context, tenantID, userID, agentID string) (*Agent, error) {
	var a Agent
	err := db.Pool.QueryRow(ctx,
		`SELECT id::text, name, COALESCE(description,''), COALESCE(system_prompt,''), COALESCE(tools,'[]'::jsonb), COALESCE(llm_config,'{}'::jsonb), max_turns, timeout_seconds, enabled, created_at, updated_at
		 FROM agents WHERE tenant_id = $1 AND id = $2 AND (user_id = $3 OR (visibility = 'tenant' AND tenant_id = $1))`, tenantID, agentID, userID).
		Scan(&a.ID, &a.Name, &a.Description, &a.SystemPrompt, &a.Tools, &a.LLMConfig,
			&a.MaxTurns, &a.TimeoutSeconds, &a.Enabled, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// resolveOwnerTenantID 鏌ヨ绯荤粺棣栦釜 owner 瑙掕壊鐢ㄦ埛鐨?tenant_id锛岀敤浣滈缃?Agent 鐨勫綊灞炵鎴枫€?// 澶氱鎴峰満鏅笅棰勭疆 Agent 浠呭湪 owner 绉熸埛鎾涓€娆★紙鍏跺畠绉熸埛闇€鑷閫氳繃 API 鍒涘缓锛夈€?func (h *AgentHandler) resolveOwnerTenantID(ctx context.Context) (string, error) {
	var tenantID string
	err := db.Pool.QueryRow(ctx,
		`SELECT tenant_id FROM users WHERE role = 'owner' ORDER BY created_at LIMIT 1`).Scan(&tenantID)
	if err != nil {
		return "", fmt.Errorf("no owner found: %w", err)
	}
	return tenantID, nil
}

// 鈹€鈹€ 閫氱敤灏忓伐鍏凤紙瀛楃涓?鏁板€兼嫾鎺ヤ笌 llm_config 鍙栧€硷級 鈹€鈹€

func trimSpace(s string) string { return strings.TrimSpace(s) }

func itoa(n int) string { return strconv.Itoa(n) }

func joinComma(items []string) string { return strings.Join(items, ", ") }

func llmString(m map[string]any, key, fallback string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func llmInt(m map[string]any, key string, fallback int) int {
	if v, ok := m[key].(float64); ok && v > 0 {
		return int(v)
	}
	return fallback
}

func llmFloat(m map[string]any, key string, fallback float64) float64 {
	if v, ok := m[key].(float64); ok && v > 0 {
		return v
	}
	return fallback
}

func boolOf(v any) bool {
	b, _ := v.(bool)
	return b
}
