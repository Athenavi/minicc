package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
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
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	return "mcc_" + hex.EncodeToString(b)
}

func ValidateAPIKeyFormat(key string) error {
	if !strings.HasPrefix(key, "mcc_") {
		return fmt.Errorf("api key must start with 'mcc_'")
	}
	hexPart := key[4:]
	if len(hexPart) != 64 {
		return fmt.Errorf("api key hex part must be 64 characters, got %d", len(hexPart))
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return fmt.Errorf("api key hex part is not valid hex: %w", err)
	}
	return nil
}
