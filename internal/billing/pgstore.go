package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/athenavi/minicc/internal/db"
)

// PGStore implements Store using the minicc PostgreSQL database.
// It uses the existing users table for balance and adds a new billing table.

type PGStore struct{}

func NewPGStore() *PGStore {
	return &PGStore{}
}

// EnsureTables creates the billing tables if they don't exist.
func (s *PGStore) EnsureTables(ctx context.Context) error {
	if db.Pool == nil {
		return fmt.Errorf("database not available")
	}

	// Add balance column to users table if not exists
	_, err := db.Pool.Exec(ctx,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS credits INTEGER NOT NULL DEFAULT 1000`)
	if err != nil {
		return fmt.Errorf("add credits column: %w", err)
	}

	// Create credit_transactions table
	_, err = db.Pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS credit_transactions (
			id VARCHAR(32) PRIMARY KEY,
			user_id VARCHAR(32) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			amount INTEGER NOT NULL,
			balance INTEGER NOT NULL,
			reason VARCHAR(64) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	if err != nil {
		return fmt.Errorf("create credit_transactions: %w", err)
	}

	// Index for fast history lookups
	_, err = db.Pool.Exec(ctx,
		`CREATE INDEX IF NOT EXISTS idx_credit_tx_user ON credit_transactions(user_id, created_at DESC)`)
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	return nil
}

func (s *PGStore) GetBalance(ctx context.Context, userID string) (int, error) {
	if db.Pool == nil {
		return 0, fmt.Errorf("database not available")
	}
	var balance int
	err := db.ReadPool().QueryRow(ctx,
		`SELECT COALESCE(credits, 0) FROM users WHERE id = $1`, userID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("get user credits: %w", err)
	}
	return balance, nil
}

func (s *PGStore) SetBalance(ctx context.Context, userID string, balance int) error {
	if db.Pool == nil {
		return fmt.Errorf("database not available")
	}
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET credits = $1 WHERE id = $2`, balance, userID)
	return err
}

func (s *PGStore) AddTransaction(ctx context.Context, tx *CreditChange) error {
	if db.Pool == nil {
		return fmt.Errorf("database not available")
	}
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO credit_transactions (id, user_id, amount, balance, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tx.ID, tx.UserID, tx.Amount, tx.Balance, tx.Reason, tx.CreatedAt)
	return err
}

func (s *PGStore) GetHistory(ctx context.Context, userID string, limit int) ([]CreditChange, error) {
	if db.Pool == nil {
		return nil, fmt.Errorf("database not available")
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.ReadPool().Query(ctx,
		`SELECT id, user_id, amount, balance, reason, created_at
		 FROM credit_transactions WHERE user_id = $1
		 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CreditChange
	for rows.Next() {
		var tx CreditChange
		if err := rows.Scan(&tx.ID, &tx.UserID, &tx.Amount, &tx.Balance, &tx.Reason, &tx.CreatedAt); err != nil {
			slog.Warn("scan transaction row skipped", "error", err)
			continue
		}
		result = append(result, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}
	return result, nil
}

// DailyFreeCount returns the number of free conversations used today (UTC).
func (s *PGStore) DailyFreeCount(ctx context.Context, userID string) (int, error) {
	if db.Pool == nil {
		return 0, fmt.Errorf("database not available")
	}
	var count int
	todayUTC := time.Now().UTC().Truncate(24 * time.Hour)
	err := db.ReadPool().QueryRow(ctx,
		`SELECT COUNT(*) FROM credit_transactions
		 WHERE user_id = $1 AND reason = 'free_chat' AND created_at >= $2`, userID, todayUTC).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// MarkFreeUsage records a free conversation usage for today.
func (s *PGStore) MarkFreeUsage(ctx context.Context, userID string) error {
	if db.Pool == nil {
		return fmt.Errorf("database not available")
	}
	tx := &CreditChange{
		ID:        fmt.Sprintf("free_%d", time.Now().UnixNano()),
		UserID:    userID,
		Amount:    0,
		Balance:   0,
		Reason:    "free_chat",
		CreatedAt: time.Now(),
	}
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO credit_transactions (id, user_id, amount, balance, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tx.ID, tx.UserID, tx.Amount, tx.Balance, tx.Reason, tx.CreatedAt)
	return err
}

// AtomicDeductBalance atomically deducts credits using a single SQL statement.
// Returns the new balance, or an error if insufficient credits or user not found.
func (s *PGStore) AtomicDeductBalance(ctx context.Context, userID string, amount int) (int, error) {
	if db.Pool == nil {
		return 0, fmt.Errorf("database not available")
	}
	var newBalance int
	err := db.Pool.QueryRow(ctx,
		`UPDATE users SET credits = credits - $1
		 WHERE id = $2 AND credits >= $1
		 RETURNING credits`,
		amount, userID).Scan(&newBalance)
	if err != nil {
		return 0, fmt.Errorf("atomic deduct failed (insufficient credits or user not found): %w", err)
	}
	return newBalance, nil
}

// AtomicAddBalance atomically adds credits using a single SQL statement.
// Returns the new balance, or an error if user not found.
func (s *PGStore) AtomicAddBalance(ctx context.Context, userID string, amount int) (int, error) {
	if db.Pool == nil {
		return 0, fmt.Errorf("database not available")
	}
	var newBalance int
	err := db.Pool.QueryRow(ctx,
		`UPDATE users SET credits = credits + $1
		 WHERE id = $2
		 RETURNING credits`,
		amount, userID).Scan(&newBalance)
	if err != nil {
		return 0, fmt.Errorf("atomic add failed: %w", err)
	}
	return newBalance, nil
}

// JSON serialization helpers for API responses
type BalanceResponse struct {
	UserID  string `json:"user_id"`
	Balance int    `json:"balance"`
}

func FormatBalance(userID string, balance int) string {
	data, _ := json.Marshal(BalanceResponse{UserID: userID, Balance: balance})
	return string(data)
}
