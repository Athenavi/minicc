package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// 鈹€鈹€ AES-GCM 鍔犺В瀵?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func testKey(t *testing.T) []byte {
	t.Helper()
	t.Setenv(EnvOIDCSecretKey, strings.Repeat("k", 32))
	key := LoadOIDCEncryptionKey()
	if key == nil {
		t.Fatal("expected key to load")
	}
	return key
}

func TestEncryptDecryptAESGCM_RoundTrip(t *testing.T) {
	key := testKey(t)
	plaintext := "super-secret-client-secret-123"

	encoded, err := EncryptAESGCM(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if encoded == plaintext {
		t.Fatal("ciphertext must not equal plaintext")
	}

	decoded, err := DecryptAESGCM(key, encoded)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decoded != plaintext {
		t.Fatalf("round trip mismatch: got %q", decoded)
	}
}

func TestEncryptAESGCM_NoncesDiffer(t *testing.T) {
	key := testKey(t)
	a, err := EncryptAESGCM(key, "same")
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncryptAESGCM(key, "same")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two encryptions of the same plaintext must differ (random nonce)")
	}
}

func TestDecryptAESGCM_WrongKey(t *testing.T) {
	key := testKey(t)
	encoded, err := EncryptAESGCM(key, "payload")
	if err != nil {
		t.Fatal(err)
	}

	otherKey := make([]byte, 32)
	if _, err := DecryptAESGCM(otherKey, encoded); err == nil {
		t.Fatal("expected decrypt with wrong key to fail")
	}
}

func TestDecryptAESGCM_Tampered(t *testing.T) {
	key := testKey(t)
	encoded, err := EncryptAESGCM(key, "payload")
	if err != nil {
		t.Fatal(err)
	}
	// 缈昏浆鏈瓧绗?鈫?base64 瑙ｇ爜鍚庣殑瀵嗘枃/tag 琚鏀?
	tampered := encoded[:len(encoded)-2] + "AA"
	if _, err := DecryptAESGCM(key, tampered); err == nil {
		t.Fatal("expected tampered ciphertext to fail")
	}
}

func TestEncryptAESGCM_BadKeyLength(t *testing.T) {
	if _, err := EncryptAESGCM([]byte("short"), "x"); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestLoadOIDCEncryptionKey(t *testing.T) {
	t.Setenv(EnvOIDCSecretKey, "")
	if key := LoadOIDCEncryptionKey(); key != nil {
		t.Fatal("expected nil key when env unset")
	}

	t.Setenv(EnvOIDCSecretKey, "too-short")
	if key := LoadOIDCEncryptionKey(); key != nil {
		t.Fatal("expected nil key when env < 32 bytes")
	}

	t.Setenv(EnvOIDCSecretKey, strings.Repeat("x", 48))
	key := LoadOIDCEncryptionKey()
	if key == nil || len(key) != 32 {
		t.Fatalf("expected 32-byte normalized key, got %v", key)
	}
}

// 鈹€鈹€ state 绛惧彂/鏍￠獙 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestStateCodec_IssueVerifyRoundTrip(t *testing.T) {
	codec := NewStateCodec([]byte("test-key"), time.Minute)
	state, err := codec.Issue("provider-1", "nonce-1")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	payload, err := codec.Verify(state)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if payload.ProviderID != "provider-1" || payload.Nonce != "nonce-1" {
		t.Fatalf("payload mismatch: %+v", payload)
	}
}

func TestStateCodec_Expired(t *testing.T) {
	codec := NewStateCodec([]byte("test-key"), time.Minute)
	base := time.Now()
	codec.now = func() time.Time { return base }

	state, err := codec.Issue("p", "n")
	if err != nil {
		t.Fatal(err)
	}

	// 鏃堕棿鎺ㄨ繘鍒?TTL 涔嬪悗 鈫?杩囨湡鎷掔粷
	codec.now = func() time.Time { return base.Add(2 * time.Minute) }
	if _, err := codec.Verify(state); !errors.Is(err, ErrStateExpired) {
		t.Fatalf("expected ErrStateExpired, got %v", err)
	}
}

func TestStateCodec_TamperedRejected(t *testing.T) {
	codec := NewStateCodec([]byte("test-key"), time.Minute)
	state, err := codec.Issue("provider-1", "nonce-1")
	if err != nil {
		t.Fatal(err)
	}

	// 绡℃敼 payload 娈碉紙浼€?provider锛?
	body, sig, found := cutState(state)
	if !found {
		t.Fatal("state missing separator")
	}
	tampered := body + "x." + sig
	if _, err := codec.Verify(tampered); !errors.Is(err, ErrStateInvalid) {
		t.Fatalf("expected ErrStateInvalid for tampered payload, got %v", err)
	}

	// 绡℃敼绛惧悕娈?
	tamperedSig := body + "." + sig[:len(sig)-2] + "AA"
	if _, err := codec.Verify(tamperedSig); !errors.Is(err, ErrStateInvalid) {
		t.Fatalf("expected ErrStateInvalid for tampered signature, got %v", err)
	}
}

func TestStateCodec_WrongKeyRejected(t *testing.T) {
	issuer := NewStateCodec([]byte("key-a"), time.Minute)
	verifier := NewStateCodec([]byte("key-b"), time.Minute)

	state, err := issuer.Issue("p", "n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(state); !errors.Is(err, ErrStateInvalid) {
		t.Fatalf("expected ErrStateInvalid with different key, got %v", err)
	}
}

func TestStateCodec_MalformedRejected(t *testing.T) {
	codec := NewStateCodec([]byte("test-key"), time.Minute)
	for _, state := range []string{"", "no-separator", "...", "a.b.c"} {
		if _, err := codec.Verify(state); !errors.Is(err, ErrStateInvalid) {
			t.Fatalf("state %q: expected ErrStateInvalid, got %v", state, err)
		}
	}
}

func TestRandomNonce(t *testing.T) {
	a, err := RandomNonce()
	if err != nil || len(a) != 32 {
		t.Fatalf("nonce: %q err=%v", a, err)
	}
	b, _ := RandomNonce()
	if a == b {
		t.Fatal("two nonces must differ")
	}
}
