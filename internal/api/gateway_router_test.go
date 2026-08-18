package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRouter_AuthMiddlewareCoverage 意图（authMW 覆盖完整性）：
// 所有涉及用户私有数据的业务路由（agents / uploads / billing 支付与用量 /
// 会话更新与分享）都必须挂在 authMW 之后——未携带凭证的请求一律 401，
// 不得以 404/405/500 等形式"放行"到业务 handler。
// 这是对 register*Routes 拆分后路由注册的回归保护。
func TestRouter_AuthMiddlewareCoverage(t *testing.T) {
	router := testRouter(t)

	protectedPaths := []string{
		// Agents CRUD + 运行会话
		"GET /v1/agents",
		"POST /v1/agents",
		"GET /v1/agents/agent-1",
		"PUT /v1/agents/agent-1",
		"DELETE /v1/agents/agent-1",
		"POST /v1/agents/agent-1/run",
		"GET /v1/agents/sessions",
		"GET /v1/agents/sessions/sess-1",
		// 分片上传全链路
		"POST /v1/uploads",
		"PUT /v1/uploads/up-1/chunks/0",
		"GET /v1/uploads/up-1",
		"POST /v1/uploads/up-1/complete",
		// Billing：支付下单 / 订单查询 / PayPal 捕获 / 用量
		"POST /v1/billing/pay",
		"GET /v1/billing/orders/pay_1",
		"POST /v1/billing/paypal-capture",
		"GET /v1/billing/usage",
		// Conversations：更新与分享
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

// TestRouter_PaymentCallbacksArePublic 意图：支付宝/微信异步回调按协议
// 不可能携带用户凭证（靠签名验签保证安全），因此这两个端点必须保持公开
// （未配置渠道时回答 501，而不是 401）——若回归中被误挂 authMW，支付
// 回调将全部失败且难以排查。
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
