package billing

import (
	"context"
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
