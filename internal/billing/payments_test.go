package billing

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"sync"
	"testing"
	"time"
)

// TestAlipaySignVerify 验证 RSA2 签名/验签自洽（同一对密钥）。
func TestAlipaySignVerify(t *testing.T) {
	client := newTestAlipayClient(t)

	params := map[string]string{
		"app_id":      client.appID,
		"method":      "alipay.trade.precreate",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"biz_content": `{"out_trade_no":"pay_test_1","total_amount":"10.00"}`,
	}
	sig, err := client.sign(params)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := client.verify(params, sig); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// 篡改参数后验签必须失败
	params["total_amount"] = "100.00"
	if err := client.verify(params, sig); err == nil {
		t.Fatal("verify should fail after tampering")
	}
}

// TestVerifyCallback 验证回调验签（成功状态 + 篡改拒绝 + 非成功状态拒绝）。
func TestVerifyCallback(t *testing.T) {
	client := newTestAlipayClient(t)

	good := map[string]string{
		"app_id":       client.appID,
		"out_trade_no": "pay_test_2",
		"trade_no":     "202608170000100001",
		"trade_status": "TRADE_SUCCESS",
		"total_amount": "10.00",
	}
	sig, err := client.sign(good)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	good["sign"] = sig

	outTradeNo, tradeNo, ok := client.VerifyCallback(good)
	if !ok || outTradeNo != "pay_test_2" || tradeNo != "202608170000100001" {
		t.Fatalf("VerifyCallback = %q, %q, %v", outTradeNo, tradeNo, ok)
	}

	// 篡改金额 → 拒绝
	tampered := map[string]string{}
	for k, v := range good {
		tampered[k] = v
	}
	tampered["total_amount"] = "1.00"
	if _, _, ok := client.VerifyCallback(tampered); ok {
		t.Fatal("tampered callback should be rejected")
	}

	// 非成功状态 → 拒绝
	waiting := map[string]string{}
	for k, v := range good {
		waiting[k] = v
	}
	waiting["trade_status"] = "WAIT_BUYER_PAY"
	if _, _, ok := client.VerifyCallback(waiting); ok {
		t.Fatal("non-success trade_status should be rejected")
	}
}

// TestConfirmPaymentIdempotent 验证订单幂等入账：重复确认只入账一次。
func TestConfirmPaymentIdempotent(t *testing.T) {
	store := &orderAwareStore{}
	mgr := NewManager(store)
	defer mgr.Close()
	mgr.Subscribe(NewTransactionRecorder(store))

	p := NewPayment("u1", ChannelAlipay, 1000, 100000, "CNY")
	if err := mgr.CreatePayment(context.Background(), p); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	// 第一次确认：入账
	_, credited, err := mgr.ConfirmPayment(context.Background(), p.ID, "trade-1")
	if err != nil || !credited {
		t.Fatalf("first ConfirmPayment = credited=%v err=%v", credited, err)
	}
	bal, _ := mgr.GetBalance("u1")
	if bal != 1000 {
		t.Fatalf("balance = %d, want 1000", bal)
	}

	// 第二次确认（重复回调）：幂等，不再入账
	_, credited2, err := mgr.ConfirmPayment(context.Background(), p.ID, "trade-1")
	if err != nil || credited2 {
		t.Fatalf("second ConfirmPayment = credited=%v err=%v (want false, nil)", credited2, err)
	}
	bal2, _ := mgr.GetBalance("u1")
	if bal2 != 1000 {
		t.Fatalf("balance after double-confirm = %d, want 1000", bal2)
	}
}

// orderAwareStore：recordingStore + 内存订单表（幂等测试用）。
type orderAwareStore struct {
	recordingStore
	mu   sync.Mutex
	ords map[string]*Payment
}

func (s *orderAwareStore) CreatePayment(ctx context.Context, p *Payment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ords == nil {
		s.ords = make(map[string]*Payment)
	}
	cp := *p
	s.ords[p.ID] = &cp
	return nil
}

func (s *orderAwareStore) GetPayment(ctx context.Context, id string) (*Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.ords[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, nil
}

// MarkPaymentPaid 幂等推进 pending→paid。
func (s *orderAwareStore) MarkPaymentPaid(ctx context.Context, id, tradeNo string) (*Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.ords[id]
	if !ok || p.Status != PayStatusPending {
		return nil, nil
	}
	p.Status = PayStatusPaid
	p.TradeNo = tradeNo
	now := time.Now()
	p.PaidAt = &now
	cp := *p
	return &cp, nil
}

func (s *orderAwareStore) UpdatePaymentProvider(ctx context.Context, id, qrCode, providerOrderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.ords[id]; ok {
		p.QRCode = qrCode
		p.ProviderOrderID = providerOrderID
	}
	return nil
}

// ── 测试密钥对 ──

var cachedKeyPair []string

func newTestAlipayClient(t *testing.T) *AlipayClient {
	t.Helper()
	privPEM, pubPEM := testRSAKeyPairOnce(t)
	client, err := NewAlipayClient("test-app-id", privPEM, pubPEM, "https://openapi.alipay.com/gateway.do", "")
	if err != nil {
		t.Fatalf("NewAlipayClient: %v", err)
	}
	return client
}

func testRSAKeyPairOnce(t *testing.T) (string, string) {
	t.Helper()
	if cachedKeyPair != nil {
		return cachedKeyPair[0], cachedKeyPair[1]
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	cachedKeyPair = []string{string(privPEM), string(pubPEM)}
	return cachedKeyPair[0], cachedKeyPair[1]
}
