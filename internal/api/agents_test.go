package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestAgentHandler 鐩存帴鏋勯€?AgentHandler锛堜笉缁?NewAgentHandler锛?
// 閬垮厤瑙﹀彂棰勭疆 Agent 鎾 goroutine 鈥斺€?閭ｉ渶瑕佺湡瀹?DB锛夈€?
// agents handler 鐨勮璇佺敱璺敱灞?authMW 璐熻矗锛宧andler 绾ф祴璇曡仛鐒?
// authMW 涔嬪悗鐨勫弬鏁版牎楠屼笌绾€昏緫銆?
func newTestAgentHandler() *AgentHandler {
	return &AgentHandler{}
}

// 鈹€鈹€ Create锛氬悕绉版牎楠?鈹€鈹€

// TestAgentCreate_InvalidJSON 鎰忓浘锛氶潪娉?JSON 蹇呴』琚嫆缁濆湪涓氬姟閫昏緫涔嬪墠锛?
// 涓嶅緱瑙﹁揪 DB锛堟棤 DB 鐜涓嬫柇瑷€ 400 鍗宠瘉鏄庢湭瑙﹁揪锛夈€?
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

// TestAgentCreate_EmptyName 鎰忓浘锛欰gent 蹇呴』鏈夊彲杈ㄨ瘑鍚嶇О鈥斺€旂┖鐧藉悕绉?
// trim 鍚庝负绌哄悓鏍锋嫆缁濓紝闃叉浜х敓鏃犲悕 Agent銆?
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

// 鈹€鈹€ Get/Update/Delete/Run锛氳矾寰勫弬鏁颁笌璇锋眰浣撴牎楠?鈹€鈹€

// TestAgentGet_MissingID 鎰忓浘锛氱己璺緞鍙傛暟蹇呴』 400锛屼笉寰楁嬁绌?id 鏌ュ簱銆?
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

// TestAgentUpdate_InvalidJSON 鎰忓浘锛氬甫鍚堟硶 id 浣嗚姹備綋闈炴硶鏃讹紝鍚屾牱鎷掔粷鍦ㄦ牎楠屽眰銆?
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

// TestAgentRun_EmptyTask 鎰忓浘锛氭淳鍙戠┖浠诲姟鏃犳剰涔夛紝蹇呴』鍦ㄦ煡璇?Agent /
// 鍒涘缓 session锛堣Е杈?DB锛変箣鍓嶆嫆缁濄€?
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

// 鈹€鈹€ llm_config 鍙栧€肩函閫昏緫 鈹€鈹€
// 杩欎簺 helper 鍐冲畾娲惧彂缁?Python 寮曟搸鐨勬ā鍨嬪弬鏁帮紱
// 鎰忓浘锛氫换浣曠己澶?闈炴硶/绫诲瀷涓嶇鐨勯厤缃」閮藉繀椤昏惤鍒板畨鍏ㄩ粯璁ゅ€硷紝
// 缁濅笉鑳芥妸绌哄€兼垨閿欒绫诲瀷浼犵粰鎵ц寮曟搸銆?

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

	// 缂哄け閿?鈫?fallback
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

	// 绌哄瓧绗︿覆 / 闈炴鏁?/ 绫诲瀷涓嶇 鈫?涓€寰?fallback锛堜笉寰楅€忎紶闈炴硶鍊硷級
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

	// nil map锛坙lm_config 涓虹┖鏃讹級涔熶笉寰?panic
	if got := llmString(nil, "model", "fb"); got != "fb" {
		t.Fatalf("llmString nil map = %q, want fallback", got)
	}
	if got := llmInt(nil, "max_tokens", 100); got != 100 {
		t.Fatalf("llmInt nil map = %d, want fallback", got)
	}
}

// TestAgentBoolOf 鎰忓浘锛歅ython 杩斿洖缁撴灉涓?success 缂哄け鎴栭潪甯冨皵鏃讹紝
// 蹇呴』鎸夊け璐ュ鐞嗭紙boolOf=false 鈫?session 鐘舵€佺疆 failed锛夛紝瀹佸彲璇姤澶辫触涓嶅彲璇姤鎴愬姛銆?
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
