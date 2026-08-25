package settings

import "testing"

func TestEncryptRoundTrip(t *testing.T) {
	s := New(nil, "test-app-secret-that-is-at-least-32-bytes-long!")
	if !s.EncryptEnabled() {
		t.Fatal("expected encryption enabled")
	}
	enc, err := s.EncryptString("sup3r-secret-value")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == "sup3r-secret-value" {
		t.Fatal("ciphertext equals plaintext")
	}
	dec, err := s.DecryptString(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != "sup3r-secret-value" {
		t.Fatalf("round-trip mismatch: got %q", dec)
	}
}

func TestEncryptNoSecret(t *testing.T) {
	s := New(nil, "")
	if s.EncryptEnabled() {
		t.Fatal("expected disabled when no app secret")
	}
	if _, err := s.EncryptString("x"); err != ErrEncryptedKeyNotFound {
		t.Fatalf("expected ErrEncryptedKeyNotFound, got %v", err)
	}
}

func TestIsSensitive(t *testing.T) {
	cases := map[string]bool{
		"password":           true,
		"api_key":            true,
		"secret_key":         true,
		"dsn":                true,
		"token":              true,
		"private_key":        true,
		"addr":               false,
		"origins":            false,
		"max_turns":          false,
		"public_base_url":    false,
	}
	for k, want := range cases {
		if got := IsSensitive(k); got != want {
			t.Errorf("IsSensitive(%q)=%v, want %v", k, got, want)
		}
	}
}
