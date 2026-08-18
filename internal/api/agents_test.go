package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestAgentHandler 直接构造 AgentHandler（不经 NewAgentHandler，
// 避免触发预置 Agent 播种 goroutine —— 那需要真实 DB）。
// agents handler 的认证由路由层 authMW 负责，handler 级测试聚焦
// authMW 之后的参数校验与纯逻辑。
func newTestAgentHandler() *AgentHandler {
	return &AgentHandler{}
}

// ── Create：名称校验 ──

// TestAgentCreate_InvalidJSON 意图：非法 JSON 必须被拒绝在业务逻辑之前，
// 不得触达 DB（无 DB 环境下断言 400 即证明未触达）。
func TestAgentCreate_InvalidJSON(t *testing.T) {
	h := newTestAgentHandler()
	req := httptest.NewRequest("POST", "/v1/agents", strings.NewReader("{not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

// TestAgentCreate_EmptyName 意图：Agent 必须有可辨识名称——空白名称
// trim 后为空同样拒绝，防止产生无名 Agent。
func TestAgentCreate_EmptyName(t *testing.T) {
	h := newTestAgentHandler()
	for _, body := range []string{`{}`, `{"name":""}`, `{"name":"   "}`} {
		req := httptest.NewRequest("POST", "/v1/agents", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.Create(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: expected 400 (name required), got %d", body, w.Code)
		}
	}
}

// ── Get/Update/Delete/Run：路径参数与请求体校验 ──

// TestAgentGet_MissingID 意图：缺路径参数必须 400，不得拿空 id 查库。
func TestAgentGet_MissingID(t *testing.T) {
	h := newTestAgentHandler()
	req := httptest.NewRequest("GET", "/v1/agents/", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", w.Code)
	}
}

func TestAgentUpdate_MissingID(t *testing.T) {
	h := newTestAgentHandler()
	req := httptest.NewRequest("PUT", "/v1/agents/", strings.NewReader(`{"name":"x"}`))
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", w.Code)
	}
}

// TestAgentUpdate_InvalidJSON 意图：带合法 id 但请求体非法时，同样拒绝在校验层。
func TestAgentUpdate_InvalidJSON(t *testing.T) {
	h := newTestAgentHandler()
	req := httptest.NewRequest("PUT", "/v1/agents/agent-1", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "agent-1")
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestAgentDelete_MissingID(t *testing.T) {
	h := newTestAgentHandler()
	req := httptest.NewRequest("DELETE", "/v1/agents/", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", w.Code)
	}
}

func TestAgentRun_MissingID(t *testing.T) {
	h := newTestAgentHandler()
	req := httptest.NewRequest("POST", "/v1/agents//run", strings.NewReader(`{"task":"t"}`))
	w := httptest.NewRecorder()
	h.Run(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", w.Code)
	}
}

// TestAgentRun_EmptyTask 意图：派发空任务无意义，必须在查询 Agent /
// 创建 session（触达 DB）之前拒绝。
func TestAgentRun_EmptyTask(t *testing.T) {
	h := newTestAgentHandler()
	for _, body := range []string{`{}`, `{"task":""}`, `{"task":"  "}`} {
		req := httptest.NewRequest("POST", "/v1/agents/agent-1/run", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetPathValue("id", "agent-1")
		w := httptest.NewRecorder()
		h.Run(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: expected 400 (task required), got %d", body, w.Code)
		}
	}
}

func TestAgentGetSession_MissingID(t *testing.T) {
	h := newTestAgentHandler()
	req := httptest.NewRequest("GET", "/v1/agents/sessions/", nil)
	w := httptest.NewRecorder()
	h.GetSession(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", w.Code)
	}
}

// ── llm_config 取值纯逻辑 ──
// 这些 helper 决定派发给 Python 引擎的模型参数；
// 意图：任何缺失/非法/类型不符的配置项都必须落到安全默认值，
// 绝不能把空值或错误类型传给执行引擎。

func TestAgentLLMConfigDefaults(t *testing.T) {
	full := map[string]any{"model": "gpt-x", "max_tokens": float64(2048), "temperature": 0.2}
	if got := llmString(full, "model", "deepseek-chat"); got != "gpt-x" {
		t.Fatalf("llmString present = %q, want gpt-x", got)
	}
	if got := llmInt(full, "max_tokens", 4096); got != 2048 {
		t.Fatalf("llmInt present = %d, want 2048", got)
	}
	if got := llmFloat(full, "temperature", 0.6); got != 0.2 {
		t.Fatalf("llmFloat present = %v, want 0.2", got)
	}

	// 缺失键 → fallback
	empty := map[string]any{}
	if got := llmString(empty, "model", "deepseek-chat"); got != "deepseek-chat" {
		t.Fatalf("llmString missing = %q, want fallback", got)
	}
	if got := llmInt(empty, "max_tokens", 4096); got != 4096 {
		t.Fatalf("llmInt missing = %d, want fallback", got)
	}
	if got := llmFloat(empty, "temperature", 0.6); got != 0.6 {
		t.Fatalf("llmFloat missing = %v, want fallback", got)
	}

	// 空字符串 / 非正数 / 类型不符 → 一律 fallback（不得透传非法值）
	bad := map[string]any{"model": "", "max_tokens": float64(-1), "temperature": "0.5"}
	if got := llmString(bad, "model", "fb"); got != "fb" {
		t.Fatalf("llmString empty string = %q, want fallback", got)
	}
	if got := llmInt(bad, "max_tokens", 4096); got != 4096 {
		t.Fatalf("llmInt negative = %d, want fallback", got)
	}
	if got := llmFloat(bad, "temperature", 0.6); got != 0.6 {
		t.Fatalf("llmFloat wrong type = %v, want fallback", got)
	}

	// nil map（llm_config 为空时）也不得 panic
	if got := llmString(nil, "model", "fb"); got != "fb" {
		t.Fatalf("llmString nil map = %q, want fallback", got)
	}
	if got := llmInt(nil, "max_tokens", 100); got != 100 {
		t.Fatalf("llmInt nil map = %d, want fallback", got)
	}
}

// TestAgentBoolOf 意图：Python 返回结果中 success 缺失或非布尔时，
// 必须按失败处理（boolOf=false → session 状态置 failed），宁可误报失败不可误报成功。
func TestAgentBoolOf(t *testing.T) {
	if !boolOf(true) {
		t.Fatal("boolOf(true) = false, want true")
	}
	for _, v := range []any{false, nil, "true", float64(1), map[string]any{}} {
		if boolOf(v) {
			t.Fatalf("boolOf(%v) = true, want false (non-bool must not count as success)", v)
		}
	}
}
