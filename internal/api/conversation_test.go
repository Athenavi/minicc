package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/session"
)

// 鈹€鈹€ 鍏变韩娴嬭瘯 helper锛坅pi 鍖呭唴鍚勬祴璇曟枃浠堕€氱敤锛?鈹€鈹€

// requestWithClaims 鏋勯€犲甫璁よ瘉涓婁笅鏂囷紙authMW 宸叉敞鍏?claims锛夌殑璇锋眰锛?
// 鐢ㄤ簬 handler 绾х洿娴嬨€俠ody 闈炵┖鏃惰嚜鍔ㄨ缃?JSON Content-Type銆?
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

// decodeAPIResponse 瑙ｆ瀽缁熶竴鍝嶅簲浣擄紝渚夸簬鏂█涓氬姟鏁版嵁鍚箟銆?
func decodeAPIResponse(t *testing.T, w *httptest.ResponseRecorder) APIResponse {
	t.Helper()
	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
	return resp
}

// newTestConversationHandler 鏋勯€犳棤 DB锛坧ool=nil, redis=nil锛夌殑浼氳瘽 handler銆?
func newTestConversationHandler() *ConversationHandler {
	return NewConversationHandler(nil, session.NewManager(nil, nil))
}

func userClaims(userID string) *auth.Claims {
	// 榛樿 tenant-id-1锛屼繚璇佸绉熸埛闅旂 helper锛坈laimsOf锛夐€氳繃鏍￠獙銆?
	// 璺ㄧ鎴烽殧绂绘祴璇曞彲璋冪敤 tenantClaims 鏄惧紡鎸囧畾涓嶅悓 tenant_id銆?
	return &auth.Claims{UserID: userID, Role: "user", TenantID: "tenant-id-1"}
}

// tenantClaims 鏋勯€犳寚瀹氱鎴风殑 claims锛岀敤浜庤法绉熸埛闅旂娴嬭瘯銆?
func tenantClaims(userID, tenantID string) *auth.Claims {
	return &auth.Claims{UserID: userID, Role: "user", TenantID: tenantID}
}

// 鈹€鈹€ List锛欴B 涓嶅彲鐢ㄦ椂鐨勯檷绾ц涓?鈹€鈹€

// TestConversationList_DegradesToEmpty 鎰忓浘锛欴B 涓嶅彲鐢ㄦ椂鍒楄〃蹇呴』闄嶇骇涓?
// 绌哄垪琛紙200锛夛紝缁濅笉鑳藉悜鐢ㄦ埛杩斿洖 500 鈥斺€?鍓嶇渚濊禆璇ユ帴鍙ｆ覆鏌撲晶杈规爮銆?
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

// 鈹€鈹€ Get 鈹€鈹€

func TestConversationGet_MissingID(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("GET", "/v1/conversations/", "", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Get(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", w.Code)
	}
}

// TestConversationGet_UnknownID_NoDB 鎰忓浘锛氫細璇濇煡璇笉鍒帮紙鍚?DB 涓嶅彲鐢級
// 蹇呴』鍥炵瓟 404 鑰屼笉鏄?500 鈥斺€?瀵瑰鎴风鑰岃█"涓嶅彲璇?涓?涓嶅瓨鍦?璇箟涓€鑷淬€?
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

// 鈹€鈹€ Create 鈹€鈹€

func TestConversationCreate_InvalidJSON(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("POST", "/v1/conversations", "{bad", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

// TestConversationCreate_DefaultTitle 鎰忓浘锛氭湭鎻愪緵鏍囬鏃跺繀椤讳娇鐢ㄩ粯璁ゆ爣棰?
// "鏂板璇?锛堜骇鍝佺害瀹氾級锛屼笖浼氳瘽 id 蹇呴』鐢辨湇鍔＄鐢熸垚杩斿洖缁欏墠绔€?
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
	if data["title"] != "鏂板璇? {
		t.Fatalf("expected default title 鏂板璇? got %v", data["title"])
	}
	if id, _ := data["id"].(string); id == "" {
		t.Fatal("expected server-generated non-empty conversation id")
	}
}

// 鈹€鈹€ Update锛氬弬鏁版牎楠岋紙鏍￠獙蹇呴』鍏堜簬浼氳瘽鏌ヨ锛岄伩鍏嶆嬁闈炴硶杈撳叆瑙﹁揪瀛樺偍锛?鈹€鈹€

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

// TestConversationUpdate_NoFields 鎰忓浘锛歵itle 涓?pinned 閮界己鐪佹椂鏇存柊鏃犳剰涔夛紝
// 蹇呴』鎷掔粷锛堥槻姝㈣鎶婄┖璇锋眰褰撲綔"娓呴櫎瀛楁"锛夈€?
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

// TestConversationUpdate_BlankTitle 鎰忓浘锛氭樉寮忎紶绌虹櫧鏍囬蹇呴』鎷掔粷锛?
// 绌虹櫧鏍囬浼氳渚ц竟鏍忓嚭鐜版棤鍚嶄細璇濄€?
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

// TestConversationUpdate_UnknownSession_NoDB 鎰忓浘锛氶€氳繃鍏ㄩ儴鍙傛暟鏍￠獙鍚庯紝
// 浼氳瘽涓嶅瓨鍦紙鍚?DB 涓嶅彲鐢級鍥炵瓟 404銆?
func TestConversationUpdate_UnknownSession_NoDB(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("PUT", "/v1/conversations/c-1", `{"title":"鏂版爣棰?}`, userClaims("user-1"))
	req.SetPathValue("id", "c-1")
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown session, got %d", w.Code)
	}
}

// 鈹€鈹€ Delete 鈹€鈹€

func TestConversationDelete_MissingID(t *testing.T) {
	h := newTestConversationHandler()
	req := requestWithClaims("DELETE", "/v1/conversations/", "", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", w.Code)
	}
}

// TestConversationDelete_UnknownSession_NoDB 鎰忓浘锛氬垹闄や笉瀛樺湪鐨勪細璇濆洖绛?404
// 锛堣€屼笉鏄?500锛夛紝涓?Get 淇濇寔涓€鑷寸殑"鏌ヤ笉鍒板嵆涓嶅瓨鍦?璇箟銆?
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
