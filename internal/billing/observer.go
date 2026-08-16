package billing

import (
	"context"
	"log/slog"

	"github.com/athenavi/minicc/internal/id"
)

// TransactionRecorder is a BillingObserver that persists credit transactions
// asynchronously via the event channel, decoupling transaction logging from
// the hot path of Deduct/AddCredits.
type TransactionRecorder struct {
	store Store
}

// NewTransactionRecorder creates a TransactionRecorder backed by the given store.
func NewTransactionRecorder(store Store) *TransactionRecorder {
	return &TransactionRecorder{store: store}
}

// OnCreditChange persists a credit_transactions row for the event.
// Called by the Manager's background dispatcher goroutine.
func (r *TransactionRecorder) OnCreditChange(evt CreditEvent) {
	tx := &CreditChange{
		ID:        "tx_" + id.NextID(),
		UserID:    evt.UserID,
		Amount:    evt.Amount,
		Balance:   0, // balance recorded at event time; not critical for history
		Reason:    evt.Reason,
		CreatedAt: evt.Timestamp,
	}
	if err := r.store.AddTransaction(context.Background(), tx); err != nil {
		slog.Warn("failed to record transaction", "user_id", evt.UserID, "error", err)
	}
}
