package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type APIKey struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Key       string    `json:"key,omitempty"` // only shown on creation
	LastUsed  time.Time `json:"last_used"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type APIKeyStore interface {
	Create(ctx *APIKey) error
	GetByKey(key string) (*APIKey, error)
	ListByUser(userID string) ([]*APIKey, error)
	Delete(id string) error
	UpdateLastUsed(id string) error
}

func GenerateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "mcc_" + hex.EncodeToString(b)
}

func ValidateAPIKeyFormat(key string) error {
	if len(key) < 10 || len(key) > 100 {
		return fmt.Errorf("invalid api key length")
	}
	return nil
}
