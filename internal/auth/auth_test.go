package auth

import (
	"context"
	"testing"
	"time"
)

func TestNewAuthenticator(t *testing.T) {
	a := NewAuthenticator("test-secret-at-least-16-chars", time.Hour)
	if a == nil {
		t.Fatal("expected non-nil authenticator")
	}
}

func TestGenerateAndValidateToken(t *testing.T) {
	a := NewAuthenticator("test-secret-at-least-16-chars", time.Hour)

	token, err := a.GenerateToken("user-1", "user@test.com", "admin", []string{"chat:write", "admin:read"})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	parsed, err := a.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if parsed.UserID != "user-1" {
		t.Fatalf("expected user-1, got %q", parsed.UserID)
	}
	if parsed.Role != "admin" {
		t.Fatalf("expected admin role, got %q", parsed.Role)
	}
	if parsed.Email != "user@test.com" {
		t.Fatalf("expected user@test.com, got %q", parsed.Email)
	}
}

func TestGenerateToken_WithPermissions(t *testing.T) {
	a := NewAuthenticator("test-secret-at-least-16-chars", time.Hour)
	perms := []string{"chat:write", "admin:read"}
	token, _ := a.GenerateToken("user-1", "", "user", perms)

	parsed, _ := a.ValidateToken(token)
	if len(parsed.Perms) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(parsed.Perms))
	}
}

func TestValidateToken_Invalid(t *testing.T) {
	a := NewAuthenticator("test-secret-at-least-16-chars", time.Hour)
	_, err := a.ValidateToken("invalid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	a1 := NewAuthenticator("secret-one-at-least-16-chars", time.Hour)
	a2 := NewAuthenticator("secret-two-at-least-16-chars", time.Hour)

	token, _ := a1.GenerateToken("user-1", "", "user", nil)

	_, err := a2.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error when validating with wrong secret")
	}
}

func TestRefreshToken(t *testing.T) {
	a := NewAuthenticator("test-secret-at-least-16-chars", time.Hour)
	token, _ := a.GenerateToken("user-1", "", "user", nil)

	refreshed, err := a.RefreshToken(token)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if refreshed == token {
		t.Fatal("expected different token after refresh")
	}

	parsed, _ := a.ValidateToken(refreshed)
	if parsed.UserID != "user-1" {
		t.Fatalf("expected user-1, got %q", parsed.UserID)
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key := GenerateAPIKey()
	if key == "" {
		t.Fatal("expected non-empty key")
	}
	if len(key) < 10 {
		t.Fatalf("expected key length >= 10, got %d", len(key))
	}
	// Should start with mcc_
	if len(key) < 4 || key[:4] != "mcc_" {
		t.Fatalf("expected 'mcc_' prefix, got %q", key[:4])
	}
}

func TestGenerateAPIKey_Unique(t *testing.T) {
	k1 := GenerateAPIKey()
	k2 := GenerateAPIKey()
	if k1 == k2 {
		t.Fatal("expected unique keys")
	}
}

func TestValidateAPIKeyFormat(t *testing.T) {
	if err := ValidateAPIKeyFormat(GenerateAPIKey()); err != nil {
		t.Fatalf("expected valid key, got %v", err)
	}
}

func TestValidateAPIKeyFormat_TooShort(t *testing.T) {
	if err := ValidateAPIKeyFormat("short"); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestPermissions(t *testing.T) {
	if PermChatWrite != "chat:write" {
		t.Fatalf("expected 'chat:write', got %q", PermChatWrite)
	}
	if PermAdminRead != "admin:read" {
		t.Fatalf("expected 'admin:read', got %q", PermAdminRead)
	}
	if PermToolsExec != "tools:execute" {
		t.Fatalf("expected 'tools:execute', got %q", PermToolsExec)
	}
}

func TestHasPermission(t *testing.T) {
	c := &Claims{
		UserID: "user-1",
		Role:   "user",
		Perms:  []string{"chat:write"},
	}
	if !HasPermission(c, "chat:write") {
		t.Fatal("expected to have chat:write permission")
	}
	if HasPermission(c, "admin:read") {
		t.Fatal("expected NOT to have admin:read permission")
	}
}

func TestHasPermission_RoleBased(t *testing.T) {
	admin := &Claims{UserID: "admin-1", Role: "admin"}
	if !HasPermission(admin, "chat:write") {
		t.Fatal("expected admin to have chat:write via role")
	}
	if !HasPermission(admin, "admin:read") {
		t.Fatal("expected admin to have admin:read via role")
	}
}

func TestHasPermission_Nil(t *testing.T) {
	if HasPermission(nil, "chat:write") {
		t.Fatal("expected false for nil claims")
	}
}

func TestRolePermissions(t *testing.T) {
	if _, ok := RolePermissions["owner"]; !ok {
		t.Fatal("expected owner role")
	}
	if _, ok := RolePermissions["admin"]; !ok {
		t.Fatal("expected admin role")
	}
	if _, ok := RolePermissions["user"]; !ok {
		t.Fatal("expected user role")
	}
	if len(RolePermissions["owner"]) < len(RolePermissions["user"]) {
		t.Fatal("expected owner to have more permissions than user")
	}
}

func TestWithClaimsAndGetClaims(t *testing.T) {
	claims := &Claims{UserID: "user-1", Role: "user"}
	ctx := WithClaims(context.Background(), claims)

	got := GetClaims(ctx)
	if got == nil {
		t.Fatal("expected non-nil claims")
	}
	if got.UserID != "user-1" {
		t.Fatalf("expected user-1, got %q", got.UserID)
	}
}

func TestGetClaims_NoContext(t *testing.T) {
	claims := GetClaims(nil)
	if claims != nil {
		t.Fatal("expected nil claims for nil context")
	}
}

func TestGetClaims_EmptyContext(t *testing.T) {
	claims := GetClaims(context.Background())
	if claims != nil {
		t.Fatal("expected nil claims for empty context")
	}
}
