package engine

import "context"

// Biller interface for credit management during LLM calls.
type Biller interface {
	Deduct(userID, reason string, amount int) (int, error)
	GetBalance(userID string) (int, error)
	DailyFreeCount(ctx context.Context, userID string) (int, error)
	MarkFreeUsage(ctx context.Context, userID string) error
	DeductTokens(userID string, inputTokens, outputTokens int) (int, error)
}
