package billing

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockStore struct{}

func (m *mockStore) GetBalance(ctx context.Context, userID string) (int, error) {
	return 0, nil
}
func (m *mockStore) SetBalance(ctx context.Context, userID string, balance int) error {
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
	return 0, nil
}
func (m *mockStore) AtomicAddBalance(ctx context.Context, userID string, amount int) (int, error) {
	return 0, nil
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

func TestBalanceSyncer_CloseDoubleCall(t *testing.T) {
	syncer := NewBalanceSyncer(&mockStore{}, 1*time.Second)
	// First close should succeed
	syncer.Close()
	// Second close should NOT panic
	syncer.Close()
}

// TestTransactionRecorder_RecordsRealBalance 验证交易记录携带交易后真实余额，
// 且金额符号正确（加为正、扣为负）。
func TestTransactionRecorder_RecordsRealBalance(t *testing.T) {
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

// TestTransactionRecorder_SkipsFreeChat 验证 free_chat 用量标记不写入交易历史。
func TestTransactionRecorder_SkipsFreeChat(t *testing.T) {
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
