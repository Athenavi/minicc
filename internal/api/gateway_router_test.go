package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRouter_AuthMiddlewareCoverage 鎰忓浘锛坅uthMW 瑕嗙洊瀹屾暣鎬э級锛?
// 鎵€鏈夋秹鍙婄敤鎴风鏈夋暟鎹殑涓氬姟璺敱锛坅gents / uploads / billing 鏀粯涓庣敤閲?/
// 浼氳瘽鏇存柊涓庡垎浜級閮藉繀椤绘寕鍦?authMW 涔嬪悗鈥斺€旀湭鎼哄甫鍑瘉鐨勮姹備竴寰?401锛?
// 涓嶅緱浠?404/405/500 绛夊舰寮?鏀捐"鍒颁笟鍔?handler銆?
// 杩欐槸瀵?register*Routes 鎷嗗垎鍚庤矾鐢辨敞鍐岀殑鍥炲綊淇濇姢銆?
func TestRouter_AuthMiddlewareCoverage(t *testing.T) {
	router := testRouter(t)

	protectedPaths := []string{
		// Agents CRUD + 杩愯浼氳瘽
		"GET /v1/agents",
		"POST /v1/agents",
		"GET /v1/agents/agent-1",
		"PUT /v1/agents/agent-1",
		"DELETE /v1/agents/agent-1",
		"POST /v1/agents/agent-1/run",
		"GET /v1/agents/sessions",
		"GET /v1/agents/sessions/sess-1",
		// 鍒嗙墖涓婁紶鍏ㄩ摼璺?
		"POST /v1/uploads",
		"PUT /v1/uploads/up-1/chunks/0",
		"GET /v1/uploads/up-1",
		"POST /v1/uploads/up-1/complete",
		// Billing锛氭敮浠樹笅鍗?/ 璁㈠崟鏌ヨ / PayPal 鎹曡幏 / 鐢ㄩ噺
		"POST /v1/billing/pay",
		"GET /v1/billing/orders/pay_1",
		"POST /v1/billing/paypal-capture",
		"GET /v1/billing/usage",
		// Conversations锛氭洿鏂颁笌鍒嗕韩
		"PUT /v1/conversations/conv-1",
		"POST /v1/conversations/conv-1/share",
		"GET /v1/conversations/conv-1/share",
		"DELETE /v1/conversations/conv-1/share",
	}

	for _, path := range protectedPaths {
		method, url, _ := strings.Cut(path, " ")
		req := httptest.NewRequest(method, url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: unauthenticated request must be rejected with 401 by authMW, got %d", path, w.Code)
		}
	}
}

// TestRouter_PaymentCallbacksArePublic 鎰忓浘锛氭敮浠樺疂/寰俊寮傛鍥炶皟鎸夊崗璁?
// 涓嶅彲鑳芥惡甯︾敤鎴峰嚟璇侊紙闈犵鍚嶉獙绛句繚璇佸畨鍏級锛屽洜姝よ繖涓や釜绔偣蹇呴』淇濇寔鍏紑
// 锛堟湭閰嶇疆娓犻亾鏃跺洖绛?501锛岃€屼笉鏄?401锛夆€斺€旇嫢鍥炲綊涓璇寕 authMW锛屾敮浠?
// 鍥炶皟灏嗗叏閮ㄥけ璐ヤ笖闅句互鎺掓煡銆?
func TestRouter_PaymentCallbacksArePublic(t *testing.T) {
	router := testRouter(t)

	for _, path := range []string{
		"/v1/billing/callback/alipay",
		"/v1/billing/callback/wechat",
	} {
		req := httptest.NewRequest("POST", path, strings.NewReader(""))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code == http.StatusUnauthorized {
			t.Errorf("%s: payment callback must not be gated by authMW (got 401)", path)
		}
	}
}
