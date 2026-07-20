package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-32-bytes-long-for-testing!")
	defer os.Unsetenv("JWT_SECRET")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("expected port 8080, got: %s", cfg.Port)
	}
	if cfg.CORSOrigins != "http://localhost:3000,http://localhost:5173" {
		t.Errorf("expected cors origins http://localhost:3000,http://localhost:5173, got: %s", cfg.CORSOrigins)
	}
	if cfg.JWTSecret != "test-secret-32-bytes-long-for-testing!" {
		t.Errorf("JWT_SECRET not read from env")
	}
}

func TestLoadFailsWithoutJWTSecret(t *testing.T) {
	os.Unsetenv("JWT_SECRET")

	// ValidateJWTSecret tests the validation logic directly
	result := ValidateJWTSecret("")
	if result {
		t.Error("expected false for empty secret")
	}

	result = ValidateJWTSecret("dev-secret-change-in-production")
	if result {
		t.Error("expected false for default secret")
	}

	result = ValidateJWTSecret("a-real-strong-secret-key-here-32-bytes!")
	if !result {
		t.Error("expected true for valid secret")
	}
}

func TestEnvOverride(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-here-32-bytes-long-for-test!")
	os.Setenv("PORT", "9090")
	defer func() {
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("PORT")
	}()

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("expected port 9090, got: %s", cfg.Port)
	}
}
