package billing

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockStore 妯℃嫙 PG 鍘熷瓙浣欓璇箟锛欰tomicDeduct/Add 鐩存帴淇敼鍐呭瓨浣欓锛?// 涓庣敓浜?PGStore 鐨?UPDATE ... RETURNING 琛屼负涓€鑷淬€?type mockStore struct {
	mu      sync.Mutex
	balance int
}

func (m *mockStore) GetBalance(ctx context.Context, userID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.balance, nil
}
func (m *mockStore) SetBalance(ctx context.Context, userID string, balance int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balance = balance
	return nil
}
func (m *mockStore) AddTransaction(ctx context.Context, tx *CreditChange) error { return nil }
func (m *mockStore) GetHistory(ctx context.Context, userID string, limit int) ([]CreditChange, error) {
	return nil, nil
}
func (m *mockStore) DailyFreeCount(ctx context.Context, userID string) (int, error) {
	return 0, nil
}
func (m *mockStore) MarkFreeUsage(ctx context.Context, userID string) error { return nil }
func (m *mockStore) AtomicDeductBalance(ctx context.Context, userID string, amount int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.balance < amount {
		return 0, fmt.Errorf("insufficient credits")
	}
	m.balance -= amount
	return m.balance, nil
}
func (m *mockStore) AtomicAddBalance(ctx context.Context, userID string, amount int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balance += amount
	return m.balance, nil
}

// 鈹€鈹€ PaymentStore锛堟祴璇曠┖瀹炵幇锛?鈹€鈹€
func (m *mockStore) CreatePayment(ctx context.Context, p *Payment) error { return nil }
func (m *mockStore) GetPayment(ctx context.Context, id string) (*Payment, error) {
	return nil, nil
}
func (m *mockStore) GetPaymentByProviderOrderID(ctx context.Context, providerOrderID string) (*Payment, error) {
	return nil, nil
}
func (m *mockStore) MarkPaymentPaid(ctx context.Context, id, tradeNo string) (*Payment, error) {
	return nil, nil
}
func (m *mockStore) MarkPaymentFailed(ctx context.Context, id string) error { return nil }
func (m *mockStore) UpdatePaymentProvider(ctx context.Context, id, qrCode, providerOrderID string) error {
	return nil
}

// recordingStore captures every AddTransaction call for assertions.
type recordingStore struct {
	mockStore
	mu  sync.Mutex
	txs []CreditChange
}

func (m *recordingStore) AddTransaction(ctx context.Context, tx *CreditChange) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.txs = append(m.txs, *tx)
	return nil
}

func (m *recordingStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.txs)
}

func (m *recordingStore) tx(i int) CreditChange {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.txs[i]
}

// waitFor polls cond until true or a 2s timeout.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func TestManager_CloseDoubleCall(t *testing.T) {
	mgr := NewManager(&mockStore{})
	// First close should succeed
	mgr.Close()
	// Second close should NOT panic
	mgr.Close()
}


// TestTransactionRecorder_RecordsRealBalance 楠岃瘉浜ゆ槗璁板綍鎼哄甫浜ゆ槗鍚庣湡瀹炰綑棰濓紝
// 涓旈噾棰濈鍙锋纭紙鍔犱负姝ｃ€佹墸涓鸿礋锛夈€?func TestTransactionRecorder_RecordsRealBalance(t *testing.T) {
	store := &recordingStore{}
	mgr := NewManager(store)
	defer mgr.Close()
	mgr.Subscribe(NewTransactionRecorder(store))

	bal, err := mgr.AddCredits("u1", "recharge", 100)
	if err != nil || bal != 100 {
		t.Fatalf("AddCredits = %d, %v; want 100, nil", bal, err)
	}
	waitFor(t, func() bool { return store.count() == 1 })
	tx := store.tx(0)
	if tx.Balance != 100 || tx.Amount != 100 || tx.Reason != "recharge" {
		t.Fatalf("recharge tx = %+v; want balance=100 amount=100 reason=recharge", tx)
	}

	bal, err = mgr.Deduct("u1", "llm_token", 30)
	if err != nil || bal != 70 {
		t.Fatalf("Deduct = %d, %v; want 70, nil", bal, err)
	}
	waitFor(t, func() bool { return store.count() == 2 })
	tx = store.tx(1)
	if tx.Balance != 70 || tx.Amount != -30 || tx.Reason != "llm_token" {
		t.Fatalf("deduct tx = %+v; want balance=70 amount=-30 reason=llm_token", tx)
	}
}

// TestTransactionRecorder_SkipsFreeChat 楠岃瘉 free_chat 鐢ㄩ噺鏍囪涓嶅啓鍏ヤ氦鏄撳巻鍙层€?func TestTransactionRecorder_SkipsFreeChat(t *testing.T) {
	store := &recordingStore{}
	recorder := NewTransactionRecorder(store)

	recorder.OnCreditChange(CreditEvent{UserID: "u1", Amount: 0, Balance: 5, Reason: "free_chat"})
	recorder.OnCreditChange(CreditEvent{UserID: "u1", Amount: -2, Balance: 3, Reason: "llm_token"})

	if store.count() != 1 {
		t.Fatalf("expected 1 recorded tx (free_chat skipped), got %d", store.count())
	}
	if tx := store.tx(0); tx.Reason != "llm_token" || tx.Balance != 3 {
		t.Fatalf("recorded tx = %+v; want reason=llm_token balance=3", tx)
	}
}
