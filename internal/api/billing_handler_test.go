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

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/billing"
)

// 鈹€鈹€ 鍐呭瓨鐗?billing.Store 鈹€鈹€
// 璁╄璐?handler 鐨勮鍗?浣欓閫昏緫鍙互鑴辩 PostgreSQL 娴嬭瘯
// 锛堢湡瀹?PGStore 渚濊禆鍏ㄥ眬 db.Pool锛夈€?

type fakeBillingStore struct {
	mu        sync.Mutex
	balances  map[string]int
	balErr    error // 娉ㄥ叆 GetBalance 閿欒
	histErr   error // 娉ㄥ叆 GetHistory 閿欒
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

// MarkPaymentPaid 骞傜瓑鎺ㄨ繘 pending鈫抪aid锛堜笌 PG 瀹炵幇鐨勮涔変竴鑷达級銆?
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

// 鈹€鈹€ 娴嬭瘯鏋勯€?鈹€鈹€

// newTestBillingHandler 鏋勯€犳棤浠讳綍鏀粯娓犻亾閰嶇疆锛坅lipay/wechat/paypal 鍧囨湭鍚敤锛?
// 鐨?BillingHandler锛岄厤鍚堝唴瀛?store銆?
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

// 鈹€鈹€ resolveUserID 鈹€鈹€

// TestBillingResolveUserID 鎰忓浘锛氳璐规帴鍙ｅ繀椤讳互 JWT claims 浣滀负鍞竴鐢ㄦ埛韬唤鏉ユ簮锛?
// 鏃?claims 鏃惰繑鍥炵┖涓诧紝椹卞姩 401 鈥斺€?缁濅笉鍏佽鍖垮悕韬唤钀藉埌璁¤垂閫昏緫銆?
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

// 鈹€鈹€ GetBalance 鈹€鈹€

func TestBillingGetBalance_NoClaims(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	req := httptest.NewRequest("GET", "/v1/billing/balance", nil)
	w := httptest.NewRecorder()
	h.GetBalance(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims, got %d", w.Code)
	}
}

// TestBillingGetBalance_NewUser 鎰忓浘锛氫綑棰濇煡璇㈠け璐ワ紙鏂扮敤鎴锋棤璁板綍锛夊繀椤婚檷绾т负
// 200 + 闆朵綑棰濓紝鑰屼笉鏄姤閿?鈥斺€?鏂扮敤鎴烽娆℃墦寮€璁¤垂椤典笉鑳界湅鍒伴敊璇€?
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

// TestBillingGetBalance_WithFreeQuotaDiag 鎰忓浘锛氫綑棰濆搷搴旈渶鍚屾椂缁欏嚭鍏嶈垂棰濆害
// 璇婃柇锛堟瘡鏃ュ厤璐规鏁帮級锛屽墠绔嵁姝ゅ垽鏂槸鍚﹁繕鑳藉厤璐瑰璇濄€?
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

// 鈹€鈹€ GetHistory 鈹€鈹€

func TestBillingGetHistory_NoClaims(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	req := httptest.NewRequest("GET", "/v1/billing/history", nil)
	w := httptest.NewRecorder()
	h.GetHistory(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims, got %d", w.Code)
	}
}

// TestBillingGetHistory_StoreErrorDegrades 鎰忓浘锛氬巻鍙叉煡璇㈠け璐ラ檷绾т负绌哄巻鍙诧紙200锛夛紝
// 涓庝綑棰濇帴鍙ｄ竴鑷寸殑"璁¤垂椤垫案杩滃彲鎵撳紑"绾﹀畾銆?
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

// 鈹€鈹€ GetUsage 鈹€鈹€

func TestBillingGetUsage_NoClaims(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	req := httptest.NewRequest("GET", "/v1/billing/usage", nil)
	w := httptest.NewRecorder()
	h.GetUsage(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims (before any DB access), got %d", w.Code)
	}
}

// 鈹€鈹€ Recharge 鈹€鈹€

// TestBillingRecharge_RequiresAdmin 鎰忓浘锛氫汉宸ュ姞甯佹槸绠＄悊鍛樼壒鏉冩搷浣溾€斺€?
// 鏅€氱敤鎴凤紙鍚棤 claims锛夊繀椤昏 403 鎷掔粷锛岀粷涓嶈兘杩涘叆鍔犲竵閫昏緫銆?
func TestBillingRecharge_RequiresAdmin(t *testing.T) {
	h, _ := newTestBillingHandler(t)

	req := requestWithClaims("POST", "/v1/billing/recharge", `{"amount":100}`, userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Recharge(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for regular user, got %d", w.Code)
	}

	// 鏃?claims 鍚屾牱鎷掔粷锛坔andler 琚粫杩?authMW 鐩磋皟鏃剁殑闃插尽锛?
	req = httptest.NewRequest("POST", "/v1/billing/recharge", strings.NewReader(`{"amount":100}`))
	w = httptest.NewRecorder()
	h.Recharge(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without claims, got %d", w.Code)
	}
}

// TestBillingRecharge_InvalidAmount 鎰忓浘锛氶潪姝ｆ暟閲戦蹇呴』鎷掔粷鍦ㄥ叆璐︿箣鍓嶃€?
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

// TestBillingRecharge_AddsCredits 鎰忓浘锛氱鐞嗗憳鍏呭€煎繀椤诲疄闄呭鍔犵敤鎴蜂綑棰濆苟鍥炴姤鏂颁綑棰濄€?
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

// 鈹€鈹€ CreatePayment 鈹€鈹€

// TestBillingCreatePayment_Validation 鎰忓浘锛氶潪娉曢噾棰?鏈煡鏀粯娓犻亾蹇呴』 400锛?
// 鍦ㄥ垱寤鸿鍗曚笌璋冪敤浠讳綍娓犻亾涔嬪墠鎷掔粷銆?
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

// TestBillingCreatePayment_ChannelNotConfigured 鎰忓浘锛氭笭閬撴湭閰嶇疆鏃跺繀椤绘樉寮?
// 杩斿洖 501 Not Implemented 鈥斺€?鐢ㄦ埛闇€瑕佹槑纭煡閬?璇ユ敮浠樻柟寮忎笉鍙敤"锛?
// 涓嶅厑璁搁潤榛樻垚鍔熸垨 500銆?
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

// 鈹€鈹€ GetOrder 鈹€鈹€

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

// TestBillingGetOrder_OwnerIsolation 鎰忓浘锛氳鍗曞睘浜庝笅鍗曠敤鎴风鏈夆€斺€?
// 鍏朵粬鐢ㄦ埛鏌ヨ蹇呴』 403锛岄槻姝㈡硠闇蹭粬浜哄厖鍊艰褰曘€?
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

// TestBillingGetOrder_PaidStatus 鎰忓浘锛氬凡鏀粯璁㈠崟杞杩斿洖 paid 鐘舵€侊紙鍓嶇鎹鏀捐锛夈€?
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

// TestBillingGetOrder_PendingNoChannel 鎰忓浘锛歱ending 璁㈠崟杞鍦ㄦ笭閬撳鎴风
// 鏈厤缃椂蹇呴』瀹夊叏闄嶇骇鈥斺€斾粛杩斿洖 pending锛岃€屼笉鏄?panic 鎴?500銆?
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

// 鈹€鈹€ 鏀粯鍥炶皟锛氭笭閬撴湭閰嶇疆 鈹€鈹€

// TestBillingCallbacks_NotConfigured 鎰忓浘锛氭湭閰嶇疆娓犻亾鐨勫洖璋冪鐐瑰繀椤?501 鎷掔粷锛?
// 闃叉浼€犲洖璋冩帰娴嬪叆璐﹂€昏緫銆?
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

// 鈹€鈹€ PayPalCapture 鈹€鈹€

func TestBillingPayPalCapture_NoClaims(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	req := httptest.NewRequest("POST", "/v1/billing/paypal-capture", strings.NewReader(`{"order_id":"x"}`))
	w := httptest.NewRecorder()
	h.PayPalCapture(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims, got %d", w.Code)
	}
}

// TestBillingPayPalCapture_MissingOrderID 鎰忓浘锛氱己 order_id 蹇呴』 400锛?
// 涓斿繀椤诲湪璋冪敤 PayPal API锛堝閮ㄧ綉缁滐級涔嬪墠鎷掔粷銆?
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

// 鈹€鈹€ 绾緟鍔╅€昏緫 鈹€鈹€

// TestBillingNotifyURLs 鎰忓浘锛氬洖璋冨湴鍧€蹇呴』鐢?PUBLIC_BASE_URL 鎺ㄥ涓斿幓灏鹃儴鏂滄潬锛?
// 鏈厤缃椂杩斿洖绌轰覆锛堝井淇￠摼璺嵁姝ゆ樉寮?501锛岃€屼笉鏄妸鍥炶皟鍙戝埌閿欒鍦板潃锛夈€?
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

// TestBillingFirstOrigin 鎰忓浘锛歅ayPal 鍥炶烦鍦板潃鍙彇 CORS 鐧藉悕鍗曠涓€涓簮锛?
// 骞舵竻鐞嗗熬閮ㄧ┖鏍?鏂滄潬锛堥槻姝㈡嫾鍑洪潪娉?redirect URL锛夈€?
func TestBillingFirstOrigin(t *testing.T) {
	h := &BillingHandler{cfg: &config.Config{CORSOrigins: "http://a.com/ , http://b.com"}}
	if got := h.firstOrigin(); got != "http://a.com" {
		t.Fatalf("firstOrigin = %q, want http://a.com", got)
	}
}

// TestBillingPayPalBaseURL 鎰忓浘锛氭矙绠卞紑鍏冲繀椤诲垏鎹?PayPal API 鍩熷悕锛?
// 闃叉娴嬭瘯娴侀噺鎵撳埌鐢熶骇缃戝叧銆?
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

// TestBillingPaymentResponse 鎰忓浘锛氳鍗曞簭鍒楀寲鍙毚闇插绾﹀瓧娈碉紱
// qr_code 浠呭湪瀛樺湪鏃跺嚭鐜帮紙閬垮厤鍓嶇鎷垮埌绌轰簩缁寸爜涓诧級銆?
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
