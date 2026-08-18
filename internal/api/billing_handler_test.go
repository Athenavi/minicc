package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/billing"
)

// ── 内存版 billing.Store ──
// 让计费 handler 的订单/余额逻辑可以脱离 PostgreSQL 测试
// （真实 PGStore 依赖全局 db.Pool）。

type fakeBillingStore struct {
	mu        sync.Mutex
	balances  map[string]int
	balErr    error // 注入 GetBalance 错误
	histErr   error // 注入 GetHistory 错误
	freeCount int
	payments  map[string]*billing.Payment
}

func newFakeBillingStore() *fakeBillingStore {
	return &fakeBillingStore{
		balances: make(map[string]int),
		payments: make(map[string]*billing.Payment),
	}
}

func (s *fakeBillingStore) GetBalance(ctx context.Context, userID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.balErr != nil {
		return 0, s.balErr
	}
	b, ok := s.balances[userID]
	if !ok {
		return 0, fmt.Errorf("no balance for %s", userID)
	}
	return b, nil
}

func (s *fakeBillingStore) SetBalance(ctx context.Context, userID string, balance int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.balances[userID] = balance
	return nil
}

func (s *fakeBillingStore) AddTransaction(ctx context.Context, tx *billing.CreditChange) error {
	return nil
}

func (s *fakeBillingStore) GetHistory(ctx context.Context, userID string, limit int) ([]billing.CreditChange, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.histErr != nil {
		return nil, s.histErr
	}
	return nil, nil
}

func (s *fakeBillingStore) DailyFreeCount(ctx context.Context, userID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.freeCount, nil
}

func (s *fakeBillingStore) MarkFreeUsage(ctx context.Context, userID string) error { return nil }

func (s *fakeBillingStore) AtomicDeductBalance(ctx context.Context, userID string, amount int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.balances[userID] -= amount
	return s.balances[userID], nil
}

func (s *fakeBillingStore) AtomicAddBalance(ctx context.Context, userID string, amount int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.balances[userID] += amount
	return s.balances[userID], nil
}

func (s *fakeBillingStore) CreatePayment(ctx context.Context, p *billing.Payment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *p
	s.payments[p.ID] = &cp
	return nil
}

func (s *fakeBillingStore) GetPayment(ctx context.Context, id string) (*billing.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.payments[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, nil
}

func (s *fakeBillingStore) GetPaymentByProviderOrderID(ctx context.Context, providerOrderID string) (*billing.Payment, error) {
	return nil, nil
}

// MarkPaymentPaid 幂等推进 pending→paid（与 PG 实现的语义一致）。
func (s *fakeBillingStore) MarkPaymentPaid(ctx context.Context, id, tradeNo string) (*billing.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.payments[id]
	if !ok || p.Status != billing.PayStatusPending {
		return nil, nil
	}
	p.Status = billing.PayStatusPaid
	p.TradeNo = tradeNo
	now := time.Now()
	p.PaidAt = &now
	cp := *p
	return &cp, nil
}

func (s *fakeBillingStore) MarkPaymentFailed(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.payments[id]; ok {
		p.Status = billing.PayStatusFailed
	}
	return nil
}

func (s *fakeBillingStore) UpdatePaymentProvider(ctx context.Context, id, qrCode, providerOrderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.payments[id]; ok {
		p.QRCode = qrCode
		p.ProviderOrderID = providerOrderID
	}
	return nil
}

// ── 测试构造 ──

// newTestBillingHandler 构造无任何支付渠道配置（alipay/wechat/paypal 均未启用）
// 的 BillingHandler，配合内存 store。
func newTestBillingHandler(t *testing.T) (*BillingHandler, *fakeBillingStore) {
	t.Helper()
	store := newFakeBillingStore()
	mgr := billing.NewManager(store)
	t.Cleanup(mgr.Close)
	h := NewBillingHandler(mgr, nil, &config.Config{})
	return h, store
}

func adminClaims(userID string) *auth.Claims {
	return &auth.Claims{UserID: userID, Role: "admin"}
}

// ── resolveUserID ──

// TestBillingResolveUserID 意图：计费接口必须以 JWT claims 作为唯一用户身份来源；
// 无 claims 时返回空串，驱动 401 —— 绝不允许匿名身份落到计费逻辑。
func TestBillingResolveUserID(t *testing.T) {
	h, _ := newTestBillingHandler(t)

	req := httptest.NewRequest("GET", "/v1/billing/balance", nil)
	if got := h.resolveUserID(req); got != "" {
		t.Fatalf("resolveUserID without claims = %q, want empty", got)
	}

	req = requestWithClaims("GET", "/v1/billing/balance", "", userClaims("user-9"))
	if got := h.resolveUserID(req); got != "user-9" {
		t.Fatalf("resolveUserID with claims = %q, want user-9", got)
	}
}

// ── GetBalance ──

func TestBillingGetBalance_NoClaims(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	req := httptest.NewRequest("GET", "/v1/billing/balance", nil)
	w := httptest.NewRecorder()
	h.GetBalance(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims, got %d", w.Code)
	}
}

// TestBillingGetBalance_NewUser 意图：余额查询失败（新用户无记录）必须降级为
// 200 + 零余额，而不是报错 —— 新用户首次打开计费页不能看到错误。
func TestBillingGetBalance_NewUser(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	req := requestWithClaims("GET", "/v1/billing/balance", "", userClaims("ghost-user"))
	w := httptest.NewRecorder()
	h.GetBalance(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 degraded, got %d", w.Code)
	}
	resp := decodeAPIResponse(t, w)
	data := resp.Data.(map[string]interface{})
	if data["balance"] != float64(0) {
		t.Fatalf("expected zero balance for new user, got %v", data["balance"])
	}
	if data["note"] != "new user" {
		t.Fatalf("expected note=new user, got %v", data["note"])
	}
}

// TestBillingGetBalance_WithFreeQuotaDiag 意图：余额响应需同时给出免费额度
// 诊断（每日免费次数），前端据此判断是否还能免费对话。
func TestBillingGetBalance_WithFreeQuotaDiag(t *testing.T) {
	h, store := newTestBillingHandler(t)
	store.balances["user-1"] = 250
	store.freeCount = 2

	req := requestWithClaims("GET", "/v1/billing/balance", "", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.GetBalance(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	data := decodeAPIResponse(t, w).Data.(map[string]interface{})
	if data["balance"] != float64(250) {
		t.Fatalf("balance = %v, want 250", data["balance"])
	}
	if data["daily_free_used"] != float64(2) || data["daily_free_remaining"] != float64(billing.DailyFreeLimit-2) {
		t.Fatalf("free quota diag wrong: used=%v remaining=%v", data["daily_free_used"], data["daily_free_remaining"])
	}
	if data["within_free_quota"] != true {
		t.Fatalf("within_free_quota = %v, want true (2 < %d)", data["within_free_quota"], billing.DailyFreeLimit)
	}
}

// ── GetHistory ──

func TestBillingGetHistory_NoClaims(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	req := httptest.NewRequest("GET", "/v1/billing/history", nil)
	w := httptest.NewRecorder()
	h.GetHistory(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims, got %d", w.Code)
	}
}

// TestBillingGetHistory_StoreErrorDegrades 意图：历史查询失败降级为空历史（200），
// 与余额接口一致的"计费页永远可打开"约定。
func TestBillingGetHistory_StoreErrorDegrades(t *testing.T) {
	h, store := newTestBillingHandler(t)
	store.histErr = fmt.Errorf("pg down")

	req := requestWithClaims("GET", "/v1/billing/history", "", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.GetHistory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 degraded, got %d", w.Code)
	}
	data := decodeAPIResponse(t, w).Data.(map[string]interface{})
	hist, ok := data["history"].([]interface{})
	if !ok || len(hist) != 0 {
		t.Fatalf("expected empty history array, got %v", data["history"])
	}
}

// ── GetUsage ──

func TestBillingGetUsage_NoClaims(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	req := httptest.NewRequest("GET", "/v1/billing/usage", nil)
	w := httptest.NewRecorder()
	h.GetUsage(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims (before any DB access), got %d", w.Code)
	}
}

// ── Recharge ──

// TestBillingRecharge_RequiresAdmin 意图：人工加币是管理员特权操作——
// 普通用户（含无 claims）必须被 403 拒绝，绝不能进入加币逻辑。
func TestBillingRecharge_RequiresAdmin(t *testing.T) {
	h, _ := newTestBillingHandler(t)

	req := requestWithClaims("POST", "/v1/billing/recharge", `{"amount":100}`, userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Recharge(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for regular user, got %d", w.Code)
	}

	// 无 claims 同样拒绝（handler 被绕过 authMW 直调时的防御）
	req = httptest.NewRequest("POST", "/v1/billing/recharge", strings.NewReader(`{"amount":100}`))
	w = httptest.NewRecorder()
	h.Recharge(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without claims, got %d", w.Code)
	}
}

// TestBillingRecharge_InvalidAmount 意图：非正数金额必须拒绝在入账之前。
func TestBillingRecharge_InvalidAmount(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	for _, body := range []string{`{}`, `{"amount":0}`, `{"amount":-5}`} {
		req := requestWithClaims("POST", "/v1/billing/recharge", body, adminClaims("admin-1"))
		w := httptest.NewRecorder()
		h.Recharge(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: expected 400, got %d", body, w.Code)
		}
	}
}

// TestBillingRecharge_AddsCredits 意图：管理员充值必须实际增加用户余额并回报新余额。
func TestBillingRecharge_AddsCredits(t *testing.T) {
	h, store := newTestBillingHandler(t)
	store.balances["admin-1"] = 0

	req := requestWithClaims("POST", "/v1/billing/recharge", `{"amount":100}`, adminClaims("admin-1"))
	w := httptest.NewRecorder()
	h.Recharge(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	data := decodeAPIResponse(t, w).Data.(map[string]interface{})
	if data["balance"] != float64(100) {
		t.Fatalf("balance after recharge = %v, want 100", data["balance"])
	}
}

// ── CreatePayment ──

// TestBillingCreatePayment_Validation 意图：非法金额/未知支付渠道必须 400，
// 在创建订单与调用任何渠道之前拒绝。
func TestBillingCreatePayment_Validation(t *testing.T) {
	h, _ := newTestBillingHandler(t)

	req := requestWithClaims("POST", "/v1/billing/pay", `{"credits":0,"channel":"alipay"}`, userClaims("user-1"))
	w := httptest.NewRecorder()
	h.CreatePayment(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for credits<=0, got %d", w.Code)
	}

	req = requestWithClaims("POST", "/v1/billing/pay", `{"credits":100,"channel":"bitcoin"}`, userClaims("user-1"))
	w = httptest.NewRecorder()
	h.CreatePayment(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown channel, got %d", w.Code)
	}
}

// TestBillingCreatePayment_ChannelNotConfigured 意图：渠道未配置时必须显式
// 返回 501 Not Implemented —— 用户需要明确知道"该支付方式不可用"，
// 不允许静默成功或 500。
func TestBillingCreatePayment_ChannelNotConfigured(t *testing.T) {
	for _, channel := range []string{billing.ChannelAlipay, billing.ChannelWechat, billing.ChannelPayPal} {
		t.Run(channel, func(t *testing.T) {
			h, _ := newTestBillingHandler(t)
			body := fmt.Sprintf(`{"credits":100,"channel":%q}`, channel)
			req := requestWithClaims("POST", "/v1/billing/pay", body, userClaims("user-1"))
			w := httptest.NewRecorder()
			h.CreatePayment(w, req)
			if w.Code != http.StatusNotImplemented {
				t.Fatalf("expected 501 for unconfigured channel %s, got %d (body=%s)", channel, w.Code, w.Body.String())
			}
		})
	}
}

// ── GetOrder ──

func seedOrder(t *testing.T, store *fakeBillingStore, userID, channel, status string) *billing.Payment {
	t.Helper()
	p := billing.NewPayment(userID, channel, 100, 100, "CNY")
	p.Status = status
	if err := store.CreatePayment(context.Background(), p); err != nil {
		t.Fatalf("seed payment: %v", err)
	}
	return p
}

func TestBillingGetOrder_MissingID(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	req := requestWithClaims("GET", "/v1/billing/orders/", "", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.GetOrder(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing order id, got %d", w.Code)
	}
}

func TestBillingGetOrder_Unknown(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	req := requestWithClaims("GET", "/v1/billing/orders/pay_missing", "", userClaims("user-1"))
	req.SetPathValue("id", "pay_missing")
	w := httptest.NewRecorder()
	h.GetOrder(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown order, got %d", w.Code)
	}
}

// TestBillingGetOrder_OwnerIsolation 意图：订单属于下单用户私有——
// 其他用户查询必须 403，防止泄露他人充值记录。
func TestBillingGetOrder_OwnerIsolation(t *testing.T) {
	h, store := newTestBillingHandler(t)
	p := seedOrder(t, store, "user-a", billing.ChannelAlipay, billing.PayStatusPending)

	req := requestWithClaims("GET", "/v1/billing/orders/"+p.ID, "", userClaims("user-b"))
	req.SetPathValue("id", p.ID)
	w := httptest.NewRecorder()
	h.GetOrder(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-owner, got %d", w.Code)
	}
}

// TestBillingGetOrder_PaidStatus 意图：已支付订单轮询返回 paid 状态（前端据此放行）。
func TestBillingGetOrder_PaidStatus(t *testing.T) {
	h, store := newTestBillingHandler(t)
	p := seedOrder(t, store, "user-a", billing.ChannelAlipay, billing.PayStatusPaid)

	req := requestWithClaims("GET", "/v1/billing/orders/"+p.ID, "", userClaims("user-a"))
	req.SetPathValue("id", p.ID)
	w := httptest.NewRecorder()
	h.GetOrder(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	data := decodeAPIResponse(t, w).Data.(map[string]interface{})
	if data["status"] != billing.PayStatusPaid {
		t.Fatalf("status = %v, want paid", data["status"])
	}
	if data["id"] != p.ID {
		t.Fatalf("id = %v, want %s", data["id"], p.ID)
	}
}

// TestBillingGetOrder_PendingNoChannel 意图：pending 订单轮询在渠道客户端
// 未配置时必须安全降级——仍返回 pending，而不是 panic 或 500。
func TestBillingGetOrder_PendingNoChannel(t *testing.T) {
	h, store := newTestBillingHandler(t)
	p := seedOrder(t, store, "user-a", billing.ChannelAlipay, billing.PayStatusPending)

	req := requestWithClaims("GET", "/v1/billing/orders/"+p.ID, "", userClaims("user-a"))
	req.SetPathValue("id", p.ID)
	w := httptest.NewRecorder()
	h.GetOrder(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	data := decodeAPIResponse(t, w).Data.(map[string]interface{})
	if data["status"] != billing.PayStatusPending {
		t.Fatalf("status = %v, want pending", data["status"])
	}
}

// ── 支付回调：渠道未配置 ──

// TestBillingCallbacks_NotConfigured 意图：未配置渠道的回调端点必须 501 拒绝，
// 防止伪造回调探测入账逻辑。
func TestBillingCallbacks_NotConfigured(t *testing.T) {
	h, _ := newTestBillingHandler(t)

	req := httptest.NewRequest("POST", "/v1/billing/callback/alipay", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.AlipayCallback(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("alipay callback: expected 501, got %d", w.Code)
	}

	req = httptest.NewRequest("POST", "/v1/billing/callback/wechat", strings.NewReader(""))
	w = httptest.NewRecorder()
	h.WechatCallback(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("wechat callback: expected 501, got %d", w.Code)
	}
}

// ── PayPalCapture ──

func TestBillingPayPalCapture_NoClaims(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	req := httptest.NewRequest("POST", "/v1/billing/paypal-capture", strings.NewReader(`{"order_id":"x"}`))
	w := httptest.NewRecorder()
	h.PayPalCapture(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims, got %d", w.Code)
	}
}

// TestBillingPayPalCapture_MissingOrderID 意图：缺 order_id 必须 400，
// 且必须在调用 PayPal API（外部网络）之前拒绝。
func TestBillingPayPalCapture_MissingOrderID(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	for _, body := range []string{`{}`, `{"order_id":""}`} {
		req := requestWithClaims("POST", "/v1/billing/paypal-capture", body, userClaims("user-1"))
		w := httptest.NewRecorder()
		h.PayPalCapture(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: expected 400, got %d", body, w.Code)
		}
	}
}

// ── 纯辅助逻辑 ──

// TestBillingNotifyURLs 意图：回调地址必须由 PUBLIC_BASE_URL 推导且去尾部斜杠；
// 未配置时返回空串（微信链路据此显式 501，而不是把回调发到错误地址）。
func TestBillingNotifyURLs(t *testing.T) {
	h := &BillingHandler{cfg: &config.Config{}}
	if got := h.alipayNotifyURL(); got != "" {
		t.Fatalf("alipayNotifyURL without base = %q, want empty", got)
	}
	if got := h.wechatNotifyURL(); got != "" {
		t.Fatalf("wechatNotifyURL without base = %q, want empty", got)
	}

	h.cfg.PublicBaseURL = "https://app.example.com/"
	if got := h.alipayNotifyURL(); got != "https://app.example.com/v1/billing/callback/alipay" {
		t.Fatalf("alipayNotifyURL = %q", got)
	}
	if got := h.wechatNotifyURL(); got != "https://app.example.com/v1/billing/callback/wechat" {
		t.Fatalf("wechatNotifyURL = %q", got)
	}
}

// TestBillingFirstOrigin 意图：PayPal 回跳地址只取 CORS 白名单第一个源，
// 并清理尾部空格/斜杠（防止拼出非法 redirect URL）。
func TestBillingFirstOrigin(t *testing.T) {
	h := &BillingHandler{cfg: &config.Config{CORSOrigins: "http://a.com/ , http://b.com"}}
	if got := h.firstOrigin(); got != "http://a.com" {
		t.Fatalf("firstOrigin = %q, want http://a.com", got)
	}
}

// TestBillingPayPalBaseURL 意图：沙箱开关必须切换 PayPal API 域名，
// 防止测试流量打到生产网关。
func TestBillingPayPalBaseURL(t *testing.T) {
	h := &BillingHandler{cfg: &config.Config{PayPalSandbox: true}}
	if got := h.payPalBaseURL(); got != "https://api-m.sandbox.paypal.com" {
		t.Fatalf("sandbox base url = %q", got)
	}
	h.cfg.PayPalSandbox = false
	if got := h.payPalBaseURL(); got != "https://api-m.paypal.com" {
		t.Fatalf("prod base url = %q", got)
	}
}

// TestBillingPaymentResponse 意图：订单序列化只暴露契约字段；
// qr_code 仅在存在时出现（避免前端拿到空二维码串）。
func TestBillingPaymentResponse(t *testing.T) {
	h := &BillingHandler{cfg: &config.Config{}}
	p := billing.NewPayment("user-1", billing.ChannelAlipay, 100, 100, "CNY")

	resp := h.paymentResponse(p)
	if resp["id"] != p.ID || resp["status"] != billing.PayStatusPending || resp["currency"] != "CNY" {
		t.Fatalf("unexpected response fields: %v", resp)
	}
	if _, ok := resp["qr_code"]; ok {
		t.Fatal("qr_code must be omitted when empty")
	}

	p.QRCode = "https://qr.alipay.com/xxx"
	resp = h.paymentResponse(p)
	if resp["qr_code"] != "https://qr.alipay.com/xxx" {
		t.Fatalf("qr_code = %v, want set value", resp["qr_code"])
	}
}
