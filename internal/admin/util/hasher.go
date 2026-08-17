package util

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashKey hashes an API key using SHA-256.
func HashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// VerifyKey verifies if the provided key matches the hash.
func VerifyKey(key, hash string) bool {
	return HashKey(key) == hash
}
