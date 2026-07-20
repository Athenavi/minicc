package billing

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// ── Types ──

// CreditChange records a credit transaction.
type CreditChange struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Amount    int    `json:"amount"`   // positive = credit, negative = debit
	Balance   int    `json:"balance"`  // balance after transaction
	Reason    string `json:"reason"`   // "llm_call", "image_gen", "recharge", "admin"
	CreatedAt time.Time `json:"created_at"`
}

// BillingConfig holds pricing and limits.
type BillingConfig struct {
	FreeCredits      int            `json:"free_credits"`       // credits given on registration
	LLMCostPerToken  int            `json:"llm_cost_per_token"` // cost per token (input)
	LLMCostPerOutput int            `json:"llm_cost_per_output"`
	ImageCost        int            `json:"image_cost"`         // per image generation
}

var DefaultConfig = BillingConfig{
	FreeCredits:      1000,
	LLMCostPerToken:  1,   // 1 credit per 1000 input tokens
	LLMCostPerOutput: 2,   // 2 credits per 1000 output tokens
	ImageCost:        50,  // 50 credits per image
}

// DailyFreeLimit is the number of free conversations per user per day.
const DailyFreeLimit = 5

// CreditEvent represents a credit balance change event.
type CreditEvent struct {
	UserID    string
	Amount    int       // positive = credit, negative = debit
	Reason    string
	Timestamp time.Time
}

// BillingObserver is notified asynchronously when credits change.
type BillingObserver interface {
	OnCreditChange(event CreditEvent)
}

// Manager handles credit operations with async observer notification.
// Balance is tracked in-memory via atomic operations; DB persistence is
// delegated to observers (BalanceSyncer) and runs entirely in the background.
type Manager struct {
	mu        sync.RWMutex
	config    BillingConfig
	store     Store
	observers []BillingObserver
	eventCh   chan CreditEvent
	done      chan struct{}
	closeOnce sync.Once
	balances  sync.Map // userID → *int64 (atomic balance)
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
// Entirely in-memory (atomic CAS); DB persistence is async via observers.
func (m *Manager) Deduct(userID, reason string, amount int) (int, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("invalid deduction amount: %d", amount)
	}

	ptr, err := m.getOrLoadBalance(userID)
	if err != nil {
		return 0, err
	}

	for {
		current := atomic.LoadInt64(ptr)
		if int(current) < amount {
			return int(current), fmt.Errorf("insufficient credits: have %d, need %d", current, amount)
		}
		newVal := current - int64(amount)
		if atomic.CompareAndSwapInt64(ptr, current, newVal) {
			m.publish(CreditEvent{
				UserID:    userID,
				Amount:    -amount,
				Reason:    reason,
				Timestamp: time.Now(),
			})
			return int(newVal), nil
		}
		// CAS failed (concurrent deduction), retry
	}
}

// AddCredits adds credits to a user's balance (for recharge or admin grants).
// Entirely in-memory (atomic add); DB persistence is async via observers.
func (m *Manager) AddCredits(userID, reason string, amount int) (int, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("invalid credit amount: %d", amount)
	}

	ptr, err := m.getOrLoadBalance(userID)
	if err != nil {
		return 0, err
	}

	newVal := atomic.AddInt64(ptr, int64(amount))
	m.publish(CreditEvent{
		UserID:    userID,
		Amount:    amount,
		Reason:    reason,
		Timestamp: time.Now(),
	})
	return int(newVal), nil
}

// GetHistory returns the user's credit transaction history.
func (m *Manager) GetHistory(ctx context.Context, userID string, limit int) ([]CreditChange, error) {
	return m.store.GetHistory(ctx, userID, limit)
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
