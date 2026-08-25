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

// TestAlipaySignVerify 楠岃瘉 RSA2 绛惧悕/楠岀鑷唇锛堝悓涓€瀵瑰瘑閽ワ級銆?func TestAlipaySignVerify(t *testing.T) {
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

	// 绡℃敼鍙傛暟鍚庨獙绛惧繀椤诲け璐?	params["total_amount"] = "100.00"
	if err := client.verify(params, sig); err == nil {
		t.Fatal("verify should fail after tampering")
	}
}

// TestVerifyCallback 楠岃瘉鍥炶皟楠岀锛堟垚鍔熺姸鎬?+ 绡℃敼鎷掔粷 + 闈炴垚鍔熺姸鎬佹嫆缁濓級銆?func TestVerifyCallback(t *testing.T) {
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

	// 绡℃敼閲戦 鈫?鎷掔粷
	tampered := map[string]string{}
	for k, v := range good {
		tampered[k] = v
	}
	tampered["total_amount"] = "1.00"
	if _, _, ok := client.VerifyCallback(tampered); ok {
		t.Fatal("tampered callback should be rejected")
	}

	// 闈炴垚鍔熺姸鎬?鈫?鎷掔粷
	waiting := map[string]string{}
	for k, v := range good {
		waiting[k] = v
	}
	waiting["trade_status"] = "WAIT_BUYER_PAY"
	if _, _, ok := client.VerifyCallback(waiting); ok {
		t.Fatal("non-success trade_status should be rejected")
	}
}

// TestConfirmPaymentIdempotent 楠岃瘉璁㈠崟骞傜瓑鍏ヨ处锛氶噸澶嶇‘璁ゅ彧鍏ヨ处涓€娆°€?func TestConfirmPaymentIdempotent(t *testing.T) {
	store := &orderAwareStore{}
	mgr := NewManager(store)
	defer mgr.Close()
	mgr.Subscribe(NewTransactionRecorder(store))

	p := NewPayment("u1", ChannelAlipay, 1000, 100000, "CNY")
	if err := mgr.CreatePayment(context.Background(), p); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	// 绗竴娆＄‘璁わ細鍏ヨ处
	_, credited, err := mgr.ConfirmPayment(context.Background(), p.ID, "trade-1")
	if err != nil || !credited {
		t.Fatalf("first ConfirmPayment = credited=%v err=%v", credited, err)
	}
	bal, _ := mgr.GetBalance("u1")
	if bal != 1000 {
		t.Fatalf("balance = %d, want 1000", bal)
	}

	// 绗簩娆＄‘璁わ紙閲嶅鍥炶皟锛夛細骞傜瓑锛屼笉鍐嶅叆璐?	_, credited2, err := mgr.ConfirmPayment(context.Background(), p.ID, "trade-1")
	if err != nil || credited2 {
		t.Fatalf("second ConfirmPayment = credited=%v err=%v (want false, nil)", credited2, err)
	}
	bal2, _ := mgr.GetBalance("u1")
	if bal2 != 1000 {
		t.Fatalf("balance after double-confirm = %d, want 1000", bal2)
	}
}

// orderAwareStore锛歳ecordingStore + 鍐呭瓨璁㈠崟琛紙骞傜瓑娴嬭瘯鐢級銆?type orderAwareStore struct {
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

// MarkPaymentPaid 骞傜瓑鎺ㄨ繘 pending鈫抪aid銆?func (s *orderAwareStore) MarkPaymentPaid(ctx context.Context, id, tradeNo string) (*Payment, error) {
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

// 鈹€鈹€ 娴嬭瘯瀵嗛挜瀵?鈹€鈹€

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
