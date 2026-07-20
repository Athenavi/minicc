package billing

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// BalanceSyncer is a BillingObserver that periodically flushes in-memory
// balance changes to the database.  It batches per-user deltas and writes
// them in a single goroutine, so the hot path (Deduct/AddCredits) never
// touches the database.
type BalanceSyncer struct {
	store    Store
	interval time.Duration

	mu        sync.Mutex
	pending   map[string]int // userID → accumulated delta (signed)
	stop      chan struct{}
	closeOnce sync.Once
}

// NewBalanceSyncer creates a syncer that flushes to the database every interval.
func NewBalanceSyncer(store Store, interval time.Duration) *BalanceSyncer {
	s := &BalanceSyncer{
		store:    store,
		interval: interval,
		pending:  make(map[string]int),
		stop:     make(chan struct{}),
	}
	go s.flushLoop()
	return s
}

// OnCreditChange accumulates the delta for later flushing.
// Called by the Manager's background dispatcher goroutine.
func (s *BalanceSyncer) OnCreditChange(evt CreditEvent) {
	s.mu.Lock()
	s.pending[evt.UserID] += evt.Amount
	s.mu.Unlock()
}

// Close stops the flush loop and performs a final flush.
func (s *BalanceSyncer) Close() {
	s.closeOnce.Do(func() {
		close(s.stop)
		s.flush()
	})
}

func (s *BalanceSyncer) flushLoop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.flush()
		case <-s.stop:
			return
		}
	}
}

// flush writes all accumulated deltas to the database in one pass.
func (s *BalanceSyncer) flush() {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return
	}
	batch := s.pending
	s.pending = make(map[string]int)
	s.mu.Unlock()

	ctx := context.Background()
	for userID, delta := range batch {
		if delta == 0 {
			continue
		}
		var err error
		if delta < 0 {
			_, err = s.store.AtomicDeductBalance(ctx, userID, -delta)
		} else {
			_, err = s.store.AtomicAddBalance(ctx, userID, delta)
		}
		if err != nil {
			slog.Warn("balance sync failed", "user_id", userID, "delta", delta, "error", err)
			// Re-queue failed delta so it's retried on next flush
			s.mu.Lock()
			s.pending[userID] += delta
			s.mu.Unlock()
		}
	}
}
