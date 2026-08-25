package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// EnvOIDCSecretKey 鏄紒涓?SSO 鍔犲瘑瀵嗛挜鐨勭幆澧冨彉閲忓悕銆?
// 鍘熷鍊煎繀椤?鈮?32 瀛楄妭锛涘唴閮ㄧ粡 SHA-256 褰掍竴鍖栦负 32 瀛楄妭 AES-256 瀵嗛挜銆?
// 鏈厤缃椂 SSO 绠＄悊鍐欐帴鍙ｈ繑鍥?503锛岃/鍙戠幇鎺ュ彛涓嶅彈褰卞搷銆?
const EnvOIDCSecretKey = "ENT_OIDC_SECRET_KEY"

// LoadOIDCEncryptionKey 浠庣幆澧冨彉閲忓姞杞?SSO 鍔犲瘑瀵嗛挜銆?
// 鏈厤缃垨闀垮害涓嶈冻 32 瀛楄妭鏃惰繑鍥?nil锛堣皟鐢ㄦ柟鎹杩斿洖 503 鎻愮ず閰嶇疆缂哄け锛夈€?
func LoadOIDCEncryptionKey() []byte {
	raw := os.Getenv(EnvOIDCSecretKey)
	if len(raw) < 8 {
		slog.Warn("ENT_OIDC_SECRET_KEY too short (< 8 chars), OIDC config write disabled",
			"length", len(raw))
		return nil
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// EncryptAESGCM 浣跨敤 AES-256-GCM 鍔犲瘑鏄庢枃锛岃繑鍥?base64(nonce || ciphertext || tag)銆?
func EncryptAESGCM(key []byte, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("aes-gcm nonce: %w", err)
	}
	// Seal(nonce, ...) 灏?nonce 浣滀负鍓嶇紑鎷兼帴閫佸嚭锛岃В瀵嗘椂鎸夊墠缂€鍒囧垎
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptAESGCM 瑙ｅ瘑 EncryptAESGCM 鐨勮緭鍑恒€傚瘑鏂囪绡℃敼鎴栧瘑閽ラ敊璇椂杩斿洖閿欒銆?
func DecryptAESGCM(key []byte, encoded string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("aes-gcm decode: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("aes-gcm: ciphertext too short")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("aes-gcm open: %w", err)
	}
	return string(plaintext), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("aes-gcm: key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
