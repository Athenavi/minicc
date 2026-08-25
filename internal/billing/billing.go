package billing

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// 鈹€鈹€ Types 鈹€鈹€

// CreditChange records a credit transaction.
type CreditChange struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Amount    int       `json:"amount"`  // positive = credit, negative = debit
	Balance   int       `json:"balance"` // balance after transaction
	Reason    string    `json:"reason"`  // "llm_call", "image_gen", "recharge", "admin"
	CreatedAt time.Time `json:"created_at"`
}

// BillingConfig holds pricing and limits.
type BillingConfig struct {
	FreeCredits      int `json:"free_credits"`       // credits given on registration
	LLMCostPerToken  int `json:"llm_cost_per_token"` // cost per token (input)
	LLMCostPerOutput int `json:"llm_cost_per_output"`
	ImageCost        int `json:"image_cost"` // per image generation
}

var DefaultConfig = BillingConfig{
	FreeCredits:      1000,
	LLMCostPerToken:  1,  // 1 credit per 1000 input tokens
	LLMCostPerOutput: 2,  // 2 credits per 1000 output tokens
	ImageCost:        50, // 50 credits per image
}

// DailyFreeLimit is the number of free conversations per user per day.
const DailyFreeLimit = 5

// CreditEvent represents a credit balance change event.
type CreditEvent struct {
	UserID    string
	Amount    int // positive = credit, negative = debit
	Balance   int // balance after this change
	Reason    string
	Timestamp time.Time
}

// BillingObserver is notified asynchronously when credits change.
type BillingObserver interface {
	OnCreditChange(event CreditEvent)
}

// Manager handles credit operations with async observer notification.
// Balance is tracked in-memory via atomic operations; DB persistence is
// delegated to observers and runs in the background.
type Manager struct {
	mu        sync.RWMutex
	config    BillingConfig
	store     Store
	observers []BillingObserver
	eventCh   chan CreditEvent
	done      chan struct{}
	closeOnce sync.Once
	balances  sync.Map // userID 鈫?*int64 (atomic balance)
}

// Store is the interface for persisting credit data.
type Store interface {
	GetBalance(ctx context.Context, userID string) (int, error)
	SetBalance(ctx context.Context, userID string, balance int) error
	AddTransaction(ctx context.Context, tx *CreditChange) error
	GetHistory(ctx context.Context, userID string, limit int) ([]CreditChange, error)
	DailyFreeCount(ctx context.Context, userID string) (int, error)
	MarkFreeUsage(ctx context.Context, userID string) error
	AtomicDeductBalance(ctx context.Context, userID string, amount int) (int, error)
	AtomicAddBalance(ctx context.Context, userID string, amount int) (int, error)
	PaymentStore
}

// NewManager creates a billing manager with the given store.
// It starts a background goroutine to dispatch events to observers.
func NewManager(store Store) *Manager {
	m := &Manager{
		store:   store,
		config:  DefaultConfig,
		eventCh: make(chan CreditEvent, 1024),
		done:    make(chan struct{}),
	}
	go m.dispatch()
	return m
}

// Subscribe registers a BillingObserver to receive credit change events.
func (m *Manager) Subscribe(obs BillingObserver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observers = append(m.observers, obs)
}

// Close stops the background event dispatcher and drains remaining events.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		close(m.done)
	})
}

// dispatch runs in a background goroutine, forwarding events to all observers.
func (m *Manager) dispatch() {
	for {
		select {
		case evt := <-m.eventCh:
			m.mu.RLock()
			observers := m.observers
			m.mu.RUnlock()
			for _, obs := range observers {
				obs.OnCreditChange(evt)
			}
		case <-m.done:
			// Drain remaining events before exiting
			for {
				select {
				case evt := <-m.eventCh:
					m.mu.RLock()
					observers := m.observers
					m.mu.RUnlock()
					for _, obs := range observers {
						obs.OnCreditChange(evt)
					}
				default:
					return
				}
			}
		}
	}
}

// publish sends a CreditEvent to the async channel. Non-blocking.
func (m *Manager) publish(evt CreditEvent) {
	select {
	case m.eventCh <- evt:
	default:
		slog.Warn("billing event channel full, dropping event", "user_id", evt.UserID, "reason", evt.Reason)
	}
}

// getOrLoadBalance returns the cached balance pointer for a user.
// On first access the balance is loaded from the database.
func (m *Manager) getOrLoadBalance(userID string) (*int64, error) {
	if v, ok := m.balances.Load(userID); ok {
		return v.(*int64), nil
	}
	balance, err := m.store.GetBalance(context.Background(), userID)
	if err != nil {
		return nil, fmt.Errorf("load balance: %w", err)
	}
	ptr := new(int64)
	*ptr = int64(balance)
	actual, _ := m.balances.LoadOrStore(userID, ptr)
	return actual.(*int64), nil
}

// GetBalance returns the user's current credit balance from the in-memory cache.
// Loads from DB on first access for a given user.
func (m *Manager) GetBalance(userID string) (int, error) {
	ptr, err := m.getOrLoadBalance(userID)
	if err != nil {
		return 0, err
	}
	return int(atomic.LoadInt64(ptr)), nil
}

// Deduct deducts credits from a user's balance. Returns the new balance.
// Returns an error if insufficient credits.
// P0-P1 淇锛氭敼涓?PG 鍗曡鍙ュ師瀛愭墸璐癸紙UPDATE ... RETURNING锛夛紝鏁版嵁搴撲负鍞竴
// 浜嬪疄婧愶紝澶氬壇鏈儴缃蹭笅涓嶄細瓒呮墸/閲嶅鎵ｈ垂锛涘唴瀛樹粎浣滆缂撳瓨銆?func (m *Manager) Deduct(userID, reason string, amount int) (int, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("invalid deduction amount: %d", amount)
	}

	newBalance, err := m.store.AtomicDeductBalance(context.Background(), userID, amount)
	if err != nil {
		return 0, fmt.Errorf("insufficient credits or user not found: %w", err)
	}
	m.setBalanceCache(userID, newBalance)
	m.publish(CreditEvent{
		UserID:    userID,
		Amount:    -amount,
		Balance:   newBalance,
		Reason:    reason,
		Timestamp: time.Now(),
	})
	return newBalance, nil
}

// AddCredits adds credits to a user's balance (for recharge or admin grants).
// P0-P1 淇锛氭敼涓?PG 鍗曡鍙ュ師瀛愬厖鍊硷紝鏁版嵁搴撲负鍞竴浜嬪疄婧愩€?func (m *Manager) AddCredits(userID, reason string, amount int) (int, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("invalid credit amount: %d", amount)
	}

	newBalance, err := m.store.AtomicAddBalance(context.Background(), userID, amount)
	if err != nil {
		return 0, fmt.Errorf("add credits failed: %w", err)
	}
	m.setBalanceCache(userID, newBalance)
	m.publish(CreditEvent{
		UserID:    userID,
		Amount:    amount,
		Balance:   newBalance,
		Reason:    reason,
		Timestamp: time.Now(),
	})
	return newBalance, nil
}

// setBalanceCache 鏇存柊鍐呭瓨璇荤紦瀛橈紙涓嶆敼鍙?DB 浜嬪疄婧愶級銆?func (m *Manager) setBalanceCache(userID string, balance int) {
	ptr := new(int64)
	*ptr = int64(balance)
	m.balances.Store(userID, ptr)
}

// GetHistory returns the user's credit transaction history.
func (m *Manager) GetHistory(ctx context.Context, userID string, limit int) ([]CreditChange, error) {
	return m.store.GetHistory(ctx, userID, limit)
}

// 鈹€鈹€ 鏀粯璁㈠崟锛坉elegate 鍒?PaymentStore锛?鈹€鈹€

// CreatePayment 鍒涘缓涓€绗?pending 鏀粯璁㈠崟銆?func (m *Manager) CreatePayment(ctx context.Context, p *Payment) error {
	return m.store.CreatePayment(ctx, p)
}

// GetPayment 鎸夊唴閮ㄨ鍗曞彿鏌ヨ璁㈠崟銆?func (m *Manager) GetPayment(ctx context.Context, id string) (*Payment, error) {
	return m.store.GetPayment(ctx, id)
}

// UpdatePaymentProvider 棰勪笅鍗曟垚鍔熷悗鍥炲～浜岀淮鐮佷笌娓犻亾璁㈠崟鍙枫€?func (m *Manager) UpdatePaymentProvider(ctx context.Context, id, qrCode, providerOrderID string) error {
	return m.store.UpdatePaymentProvider(ctx, id, qrCode, providerOrderID)
}

// MarkPaymentFailed 鏍囪璁㈠崟鏀粯澶辫触銆?func (m *Manager) MarkPaymentFailed(ctx context.Context, id string) error {
	return m.store.MarkPaymentFailed(ctx, id)
}

// Config returns the current billing config.
func (m *Manager) Config() BillingConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// DailyFreeCount returns how many free conversations the user has used today.
func (m *Manager) DailyFreeCount(ctx context.Context, userID string) (int, error) {
	return m.store.DailyFreeCount(ctx, userID)
}

// MarkFreeUsage records one free conversation for today.
func (m *Manager) MarkFreeUsage(ctx context.Context, userID string) error {
	return m.store.MarkFreeUsage(ctx, userID)
}

// DeductTokens deducts credits based on token usage.
func (m *Manager) DeductTokens(userID string, inputTokens, outputTokens int) (int, error) {
	cfg := m.Config()
	cost := int((int64(inputTokens)*int64(cfg.LLMCostPerToken) + int64(outputTokens)*int64(cfg.LLMCostPerOutput)) / 1000)
	if cost < 1 {
		cost = 1
	}
	return m.Deduct(userID, "llm_token", cost)
}
