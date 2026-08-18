package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/engine"
	"github.com/athenavi/minicc/internal/id"
	"github.com/athenavi/minicc/internal/session"
)

// AgentHandler 管理自定义 Agent（DB agents 表）+ 运行会话（agent_sessions）。
// 执行链路：Run 落 session(pending) → 异步调 Python /v1/agents/dispatch
// （Python 用 SubAgent 真执行）→ 结果回写 session(completed/failed)。
type AgentHandler struct {
	authenticator *auth.Authenticator
	pythonClient  *engine.PythonClient
}

func NewAgentHandler(a *auth.Authenticator, pc *engine.PythonClient) *AgentHandler {
	h := &AgentHandler{authenticator: a, pythonClient: pc}
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

// Agent 是自定义 Agent 的 DB 表示。
type Agent struct {
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

// AgentSession 是一次 Agent 运行的持久化记录。
type AgentSession struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id,omitempty"`
	AgentName string    `json:"agent_name,omitempty"`
	Task      string    `json:"task"`
	Status    string    `json:"status"` // pending / running / completed / failed
	Result    string    `json:"result,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ── 预置 Agent 播种（DB agents 表为空时插入内置 3 类） ──

type presetAgent struct {
	name        string
	description string
	prompt      string
	tools       []map[string]any
	llm         map[string]any
	turns       int
}

func (h *AgentHandler) seedPresetAgents() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var n int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM agents`).Scan(&n); err != nil || n > 0 {
		return
	}

	presets := []presetAgent{
		{
			name:        "knowledge",
			description: "知识检索与问答代理：搜索文件、抓取网页后回答问题",
			prompt:      "你是知识检索与问答代理。使用可用工具查找信息（搜索文件、grep、读取文件、抓取网页），基于真实材料回答，不确定时说明不确定之处。",
			tools: []map[string]any{
				{"name": "search_files", "description": "按关键词搜索文件", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
				{"name": "grep_files", "description": "在文件中查找文本", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
				{"name": "read_file", "description": "读取文件内容", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
				{"name": "web_fetch", "description": "抓取网页内容", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
			},
			llm:   map[string]any{"model": "deepseek-chat", "max_tokens": 4096, "temperature": 0.4},
			turns: 6,
		},
		{
			name:        "tool",
			description: "通用工具代理：执行命令、操作文件完成具体任务",
			prompt:      "你是通用工具代理。把任务拆解为可执行的工具调用（执行命令、读写文件等），逐步完成并汇报结果。",
			tools: []map[string]any{
				{"name": "shell_exec", "description": "执行 shell 命令", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
				{"name": "write_file", "description": "写入文件", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
				{"name": "read_file", "description": "读取文件内容", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
			},
			llm:   map[string]any{"model": "deepseek-chat", "max_tokens": 4096, "temperature": 0.6},
			turns: 8,
		},
		{
			name:        "browser",
			description: "浏览器自动化代理：导航、点击、输入、截图完成网页任务",
			prompt:      "你是浏览器自动化代理。使用浏览器工具导航网页、定位元素、点击与输入、截图确认，完成网页相关任务。",
			tools: []map[string]any{
				{"name": "browser_navigate", "description": "导航到 URL", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
				{"name": "browser_click", "description": "点击页面元素", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
				{"name": "browser_type", "description": "在输入框输入文本", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
				{"name": "browser_read", "description": "读取页面文本", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
				{"name": "browser_screenshot", "description": "页面截图", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
			},
			llm:   map[string]any{"model": "deepseek-chat", "max_tokens": 4096, "temperature": 0.5},
			turns: 8,
		},
	}

	for _, p := range presets {
		agentID, err := id.UUID()
		if err != nil {
			slog.Warn("seed preset agent: generate id", "name", p.name, "error", err)
			continue
		}
		toolsJSON, _ := json.Marshal(p.tools)
		llmJSON, _ := json.Marshal(p.llm)
		if _, err := db.Pool.Exec(ctx,
			`INSERT INTO agents (id, tenant_id, name, description, system_prompt, tools, llm_config, max_turns, timeout_seconds, enabled)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true)`,
			agentID, session.DefaultTenantID, p.name, p.description, p.prompt,
			string(toolsJSON), string(llmJSON), p.turns, 120); err != nil {
			slog.Warn("seed preset agent", "name", p.name, "error", err)
		}
	}
}

// ── CRUD ──────────────────────────────────────────────────────

// List 返回全部 Agent（按创建时间倒序）。
func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Pool.Query(r.Context(),
		`SELECT id::text, name, COALESCE(description,''), COALESCE(system_prompt,''), COALESCE(tools,'[]'::jsonb), COALESCE(llm_config,'{}'::jsonb), max_turns, timeout_seconds, enabled, created_at, updated_at
		 FROM agents ORDER BY created_at DESC`)
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

// Create 新建一个 Agent。
func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
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
		`INSERT INTO agents (id, tenant_id, name, description, system_prompt, tools, llm_config, max_turns, timeout_seconds, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, session.DefaultTenantID, body.Name, body.Description, body.SystemPrompt,
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

// Get 返回单个 Agent。
func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		BadRequest(w, "id is required")
		return
	}
	a, err := h.queryAgent(r, agentID)
	if err != nil {
		NotFound(w, "agent not found")
		return
	}
	OK(w, a)
}

// Update 更新 Agent 字段（name/description/system_prompt/tools/llm_config/max_turns/timeout_seconds/enabled）。
func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		BadRequest(w, "id is required")
		return
	}
	var body Agent
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	
	// 动态 SET：非零字段才更新（避免把空值当“清除”）
	sets := []string{}
	args := []any{}
	push := func(expr string, v any) {
		sets = append(sets, expr)
		args = append(args, v)
	}
	if body.Name != "" {
		push("name = $"+itoa(len(args)+1), body.Name)
	}
	if body.Description != "" || body.SystemPrompt != "" {
		push("description = $"+itoa(len(args)+1), body.Description)
		push("system_prompt = $"+itoa(len(args)+1), body.SystemPrompt)
	}
	if len(body.Tools) > 0 && string(body.Tools) != "null" {
		push("tools = $"+itoa(len(args)+1), string(body.Tools))
	}
	if len(body.LLMConfig) > 0 && string(body.LLMConfig) != "null" {
		push("llm_config = $"+itoa(len(args)+1), string(body.LLMConfig))
	}
	if body.MaxTurns > 0 {
		push("max_turns = $"+itoa(len(args)+1), body.MaxTurns)
	}
	if body.TimeoutSeconds > 0 {
		push("timeout_seconds = $"+itoa(len(args)+1), body.TimeoutSeconds)
	}
	push("enabled = $"+itoa(len(args)+1), body.Enabled)
	args = append(args, agentID)

	if len(sets) == 0 {
		BadRequest(w, "nothing to update")
		return
	}
	if _, err := db.Pool.Exec(r.Context(),
		`UPDATE agents SET `+joinComma(sets)+`, updated_at = NOW() WHERE id = $`+itoa(len(args)), args...); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update agent failed")
		return
	}
	a, err := h.queryAgent(r, agentID)
	if err != nil {
		NotFound(w, "agent not found")
		return
	}
	OK(w, a)
}

// Delete 删除 Agent 及其运行记录（agent_sessions 级联删除）。
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		BadRequest(w, "id is required")
		return
	}
	if _, err := db.Pool.Exec(r.Context(), `DELETE FROM agents WHERE id = $1`, agentID); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "delete agent failed")
		return
	}
	OK(w, map[string]string{"status": "deleted"})
}

// ── 运行与会话 ────────────────────────────────────────────────

// Run 派发任务给 Agent：落 session(pending) 后异步执行，结果回写。
func (h *AgentHandler) Run(w http.ResponseWriter, r *http.Request) {
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

	agent, err := h.queryAgent(r, agentID)
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
		`INSERT INTO agent_sessions (id, user_id, agent_id, name, task, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, 'pending', $6, $6)`,
		sessionID, claims.UserID, agent.ID, agent.Name, body.Task, now); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create session failed")
		return
	}

	timeout := time.Duration(agent.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	go h.executeAgent(agent, body.Task, sessionID, claims.UserID, timeout)

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

func (h *AgentHandler) executeAgent(agent *Agent, task, sessionID, userID string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, _ = db.Pool.Exec(ctx, `UPDATE agent_sessions SET status = 'running', updated_at = NOW() WHERE id = $1`, sessionID)

	// tools/llm_config 转 map 传给 Python（tools 保持 []map 结构）
	var tools []map[string]any
	if len(agent.Tools) > 0 && string(agent.Tools) != "[]" {
		_ = json.Unmarshal(agent.Tools, &tools)
	}
	var llm map[string]any
	if len(agent.LLMConfig) > 0 && string(agent.LLMConfig) != "{}" {
		_ = json.Unmarshal(agent.LLMConfig, &llm)
	}

	body := map[string]any{
		"task":         task,
		"name":         agent.Name,
		"description":  agent.Description,
		"system_prompt": agent.SystemPrompt,
		"tools":        tools,
		"model":        llmString(llm, "model", "deepseek-chat"),
		"max_turns":    agent.MaxTurns,
		"max_tokens":   llmInt(llm, "max_tokens", 4096),
		"temperature":  llmFloat(llm, "temperature", 0.6),
		"tenant_id":    userID,
		"session_id":   sessionID,
	}

	var result any
	if h.pythonClient == nil || !h.pythonClient.IsConnected() {
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

// ListSessions 返回当前用户的运行记录（倒序）。
func (h *AgentHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	rows, err := db.Pool.Query(r.Context(),
		`SELECT s.id, COALESCE(s.agent_id::text,''), COALESCE(a.name,''), s.task, s.status, COALESCE(s.result,''), s.created_at, s.updated_at
		 FROM agent_sessions s LEFT JOIN agents a ON a.id = s.agent_id
		 WHERE s.user_id = $1 ORDER BY s.created_at DESC LIMIT 100`, claims.UserID)
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

// GetSession 返回单个运行记录（归属校验）。
func (h *AgentHandler) GetSession(w http.ResponseWriter, r *http.Request) {
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
		 WHERE s.id = $1 AND s.user_id = $2`, sessionID, claims.UserID).
		Scan(&s.ID, &s.AgentID, &s.AgentName, &s.Task, &s.Status, &s.Result, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		NotFound(w, "session not found")
		return
	}
	OK(w, s)
}

// ── helpers ───────────────────────────────────────────────────

func (h *AgentHandler) queryAgent(r *http.Request, agentID string) (*Agent, error) {
	var a Agent
	err := db.Pool.QueryRow(r.Context(),
		`SELECT id::text, name, COALESCE(description,''), COALESCE(system_prompt,''), COALESCE(tools,'[]'::jsonb), COALESCE(llm_config,'{}'::jsonb), max_turns, timeout_seconds, enabled, created_at, updated_at
		 FROM agents WHERE id = $1`, agentID).
		Scan(&a.ID, &a.Name, &a.Description, &a.SystemPrompt, &a.Tools, &a.LLMConfig,
			&a.MaxTurns, &a.TimeoutSeconds, &a.Enabled, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ── 通用小工具（字符串/数值拼接与 llm_config 取值） ──

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
