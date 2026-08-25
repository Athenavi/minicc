package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/athenavi/chiron/internal/db"
	"github.com/jackc/pgx/v5"
)

// PGStore implements Store using the chiron PostgreSQL database.
// It uses the existing users table for balance and adds a new billing table.

type PGStore struct{}

func NewPGStore() *PGStore {
	return &PGStore{}
}

// EnsureTables creates the billing tables if they don't exist.
func (s *PGStore) EnsureTables(ctx context.Context) error {
	if db.Pool == nil {
		return nil // no database available, skip table initialization
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

	// Create payments table锛堟敮浠樺疂/寰俊/PayPal 閫氱敤鍏呭€艰鍗曪級
	_, err = db.Pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS payments (
			id VARCHAR(64) PRIMARY KEY,
			user_id VARCHAR(32) NOT NULL,
			channel VARCHAR(16) NOT NULL,
			credits INTEGER NOT NULL,
			amount_cents BIGINT NOT NULL DEFAULT 0,
			currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
			status VARCHAR(16) NOT NULL DEFAULT 'pending',
			qr_code TEXT,
			provider_order_id VARCHAR(64) NOT NULL DEFAULT '',
			trade_no VARCHAR(64) NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			paid_at TIMESTAMPTZ,
			expired_at TIMESTAMPTZ
		)`)
	if err != nil {
		return fmt.Errorf("create payments: %w", err)
	}
	_, err = db.Pool.Exec(ctx,
		`CREATE INDEX IF NOT EXISTS idx_payments_user ON payments(user_id, created_at DESC)`)
	if err != nil {
		return fmt.Errorf("create payments user index: %w", err)
	}
	_, err = db.Pool.Exec(ctx,
		`CREATE INDEX IF NOT EXISTS idx_payments_provider ON payments(provider_order_id) WHERE provider_order_id <> ''`)
	if err != nil {
		return fmt.Errorf("create payments provider index: %w", err)
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
	var balance int
	err := db.ReadPool().QueryRow(ctx,
		`SELECT COALESCE(credits, 0) FROM users WHERE id = $1`, userID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("get user credits: %w", err)
	}
	return balance, nil
}

func (s *PGStore) SetBalance(ctx context.Context, userID string, balance int) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET credits = $1 WHERE id = $2`, balance, userID)
	return err
}

func (s *PGStore) AddTransaction(ctx context.Context, tx *CreditChange) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO credit_transactions (id, user_id, amount, balance, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tx.ID, tx.UserID, tx.Amount, tx.Balance, tx.Reason, tx.CreatedAt)
	return err
}

func (s *PGStore) GetHistory(ctx context.Context, userID string, limit int) ([]CreditChange, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.ReadPool().Query(ctx,
		`SELECT id, user_id, amount, balance, reason, created_at
		 FROM credit_transactions WHERE user_id = $1 AND reason <> 'free_chat'
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

// 鈹€鈹€ PaymentStore 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

const _paymentColumns = `id, user_id, channel, credits, amount_cents, currency, status,
	COALESCE(qr_code, ''), provider_order_id, trade_no, created_at, paid_at, expired_at`

func scanPayment(row interface{ Scan(...any) error }) (*Payment, error) {
	var p Payment
	var qr string
	var paidAt, expiredAt *time.Time
	err := row.Scan(&p.ID, &p.UserID, &p.Channel, &p.Credits, &p.AmountCents, &p.Currency,
		&p.Status, &qr, &p.ProviderOrderID, &p.TradeNo, &p.CreatedAt, &paidAt, &expiredAt)
	if err != nil {
		return nil, err
	}
	p.QRCode = qr
	p.PaidAt = paidAt
	p.ExpiredAt = expiredAt
	return &p, nil
}

func (s *PGStore) CreatePayment(ctx context.Context, p *Payment) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO payments (id, user_id, channel, credits, amount_cents, currency, status,
			qr_code, provider_order_id, trade_no, created_at, paid_at, expired_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		p.ID, p.UserID, p.Channel, p.Credits, p.AmountCents, p.Currency, p.Status,
		p.QRCode, p.ProviderOrderID, p.TradeNo, p.CreatedAt, p.PaidAt, p.ExpiredAt)
	return err
}

func (s *PGStore) GetPayment(ctx context.Context, id string) (*Payment, error) {
	row := db.ReadPool().QueryRow(ctx,
		`SELECT `+_paymentColumns+` FROM payments WHERE id = $1`, id)
	return scanPayment(row)
}

func (s *PGStore) GetPaymentByProviderOrderID(ctx context.Context, providerOrderID string) (*Payment, error) {
	row := db.ReadPool().QueryRow(ctx,
		`SELECT `+_paymentColumns+` FROM payments WHERE provider_order_id = $1`, providerOrderID)
	return scanPayment(row)
}

// MarkPaymentPaid 骞傜瓑鎺ㄨ繘 pending鈫抪aid銆傝繑鍥?nil 琛ㄧず璁㈠崟闈?pending锛堝凡澶勭悊/涓嶅瓨鍦級銆?
func (s *PGStore) MarkPaymentPaid(ctx context.Context, id, tradeNo string) (*Payment, error) {
	row := db.Pool.QueryRow(ctx,
		`UPDATE payments SET status = 'paid', trade_no = $2, paid_at = NOW()
		 WHERE id = $1 AND status = 'pending'
		 RETURNING `+_paymentColumns,
		id, tradeNo)
	p, err := scanPayment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // 宸插鐞嗘垨涓嶅瓨鍦?
		}
		return nil, err
	}
	return p, nil
}

func (s *PGStore) MarkPaymentFailed(ctx context.Context, id string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE payments SET status = 'failed' WHERE id = $1 AND status = 'pending'`, id)
	return err
}

func (s *PGStore) UpdatePaymentProvider(ctx context.Context, id, qrCode, providerOrderID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE payments SET qr_code = $2, provider_order_id = $3 WHERE id = $1 AND status = 'pending'`,
		id, qrCode, providerOrderID)
	return err
}
