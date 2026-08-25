package billing

import (
	"context"
	"log/slog"

	"github.com/athenavi/chiron/internal/id"
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
// 浣欓涓轰氦鏄撳悗鐪熷疄浣欓锛堜簨浠舵惡甯︼級锛沠ree_chat 鏍囪鐢ㄩ噺涓嶈鍏ヤ氦鏄撳巻鍙层€?func (r *TransactionRecorder) OnCreditChange(evt CreditEvent) {
	if evt.Reason == "free_chat" {
		return
	}
	tx := &CreditChange{
		ID:        "tx_" + id.NextID(),
		UserID:    evt.UserID,
		Amount:    evt.Amount,
		Balance:   evt.Balance,
		Reason:    evt.Reason,
		CreatedAt: evt.Timestamp,
	}
	if err := r.store.AddTransaction(context.Background(), tx); err != nil {
		slog.Warn("failed to record transaction", "user_id", evt.UserID, "error", err)
	}
}
