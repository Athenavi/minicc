package util

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GenerateAPIKey generates a new API key with the given prefix.
// Format: sk-live-xxxxxxxxxxxxxxxxxxxxxx (random 32 bytes base64 encoded)
func GenerateAPIKey(prefix string) (string, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	
	key := fmt.Sprintf("%s-%s", prefix, base64.URLEncoding.EncodeToString(bytes))
	return key, nil
}

// GenerateTestAPIKey generates an API key for testing purposes.
func GenerateTestAPIKey() (string, error) {
	return GenerateAPIKey("sk-test")
}

// GenerateLiveAPIKey generates a live API key for production use.
func GenerateLiveAPIKey() (string, error) {
	return GenerateAPIKey("sk-live")
}
