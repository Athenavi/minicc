package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/session"
)

// ── 共享测试 helper（api 包内各测试文件通用） ──

// requestWithClaims 构造带认证上下文（authMW 已注入 claims）的请求，
// 用于 handler 级直测。body 非空时自动设置 JSON Content-Type。
func requestWithClaims(method, target, body string, claims *auth.Claims) *http.Request {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, target, reader)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	if claims != nil {
		req = req.WithContext(auth.WithClaims(req.Context(), claims))
	}
	return req
}

// decodeAPIResponse 解析统一响应体，便于断言业务数据含义。
func decodeAPIResponse(t *testing.T, w *httptest.ResponseRecorder) APIResponse {
	t.Helper()
	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
	return resp
}

// newTestConversationHandler 构造无 DB（pool=nil, redis=nil）的会话 handler。
func newTestConversationHandler() *ConversationHandler {
	return NewConversationHandler(nil, session.NewManager(nil, nil))
}

func userClaims(userID string) *auth.Claims {
	// 默认 tenant-id-1，保证多租户隔离 helper（claimsOf）通过校验。
	// 跨租户隔离测试可调用 tenantClaims 显式指定不同 tenant_id。
	return &auth.Claims{UserID: userID, Role: "user", TenantID: "tenant-id-1"}
}

// tenantClaims 构造指定租户的 claims，用于跨租户隔离测试。
func tenantClaims(userID, tenantID string) *auth.Claims {
	return &auth.Claims{UserID: userID, Role: "user", TenantID: tenantID}
}

// ── List：DB 不可用时的降级行为 ──

// TestConversationList_DegradesToEmpty 意图：DB 不可用时列表必须降级为
// 空列表（200），绝不能向用户返回 500 —— 前端依赖该接口渲染侧边栏。
func TestConversationList_DegradesToEmpty(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("GET", "/v1/conversations", "", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (degraded empty list), got %d", w.Code)
	}
	resp := decodeAPIResponse(t, w)
	if !resp.Success {
		t.Fatalf("expected success=true, body=%s", w.Body.String())
	}
	data, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected data to be an array, got %T", resp.Data)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty conversation list, got %v", data)
	}
}

// ── Get ──

func TestConversationGet_MissingID(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("GET", "/v1/conversations/", "", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", w.Code)
	}
}

// TestConversationGet_UnknownID_NoDB 意图：会话查询不到（含 DB 不可用）
// 必须回答 404 而不是 500 —— 对客户端而言"不可读"与"不存在"语义一致。
func TestConversationGet_UnknownID_NoDB(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("GET", "/v1/conversations/some-id", "", userClaims("user-1"))
	req.SetPathValue("id", "some-id")
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unresolvable conversation, got %d", w.Code)
	}
}

// ── Create ──

func TestConversationCreate_InvalidJSON(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("POST", "/v1/conversations", "{bad", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

// TestConversationCreate_DefaultTitle 意图：未提供标题时必须使用默认标题
// "新对话"（产品约定），且会话 id 必须由服务端生成返回给前端。
func TestConversationCreate_DefaultTitle(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("POST", "/v1/conversations", `{}`, userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	resp := decodeAPIResponse(t, w)
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", resp.Data)
	}
	if data["title"] != "新对话" {
		t.Fatalf("expected default title 新对话, got %v", data["title"])
	}
	if id, _ := data["id"].(string); id == "" {
		t.Fatal("expected server-generated non-empty conversation id")
	}
}

// ── Update：参数校验（校验必须先于会话查询，避免拿非法输入触达存储） ──

func TestConversationUpdate_MissingID(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("PUT", "/v1/conversations/", `{"pinned":true}`, userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", w.Code)
	}
}

func TestConversationUpdate_InvalidJSON(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("PUT", "/v1/conversations/c-1", "{bad", userClaims("user-1"))
	req.SetPathValue("id", "c-1")
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

// TestConversationUpdate_NoFields 意图：title 与 pinned 都缺省时更新无意义，
// 必须拒绝（防止误把空请求当作"清除字段"）。
func TestConversationUpdate_NoFields(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("PUT", "/v1/conversations/c-1", `{}`, userClaims("user-1"))
	req.SetPathValue("id", "c-1")
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when neither title nor pinned given, got %d", w.Code)
	}
}

// TestConversationUpdate_BlankTitle 意图：显式传空白标题必须拒绝，
// 空白标题会让侧边栏出现无名会话。
func TestConversationUpdate_BlankTitle(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("PUT", "/v1/conversations/c-1", `{"title":"   "}`, userClaims("user-1"))
	req.SetPathValue("id", "c-1")
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for blank title, got %d", w.Code)
	}
}

// TestConversationUpdate_UnknownSession_NoDB 意图：通过全部参数校验后，
// 会话不存在（含 DB 不可用）回答 404。
func TestConversationUpdate_UnknownSession_NoDB(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("PUT", "/v1/conversations/c-1", `{"title":"新标题"}`, userClaims("user-1"))
	req.SetPathValue("id", "c-1")
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown session, got %d", w.Code)
	}
}

// ── Delete ──

func TestConversationDelete_MissingID(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("DELETE", "/v1/conversations/", "", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", w.Code)
	}
}

// TestConversationDelete_UnknownSession_NoDB 意图：删除不存在的会话回答 404
// （而不是 500），与 Get 保持一致的"查不到即不存在"语义。
func TestConversationDelete_UnknownSession_NoDB(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("DELETE", "/v1/conversations/c-1", "", userClaims("user-1"))
	req.SetPathValue("id", "c-1")
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown session, got %d", w.Code)
	}
}
