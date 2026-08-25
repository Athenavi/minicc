package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/athenavi/chiron/internal/auth"
)

// newTestPolicyHandler 鏋勯€犳敞鍏?fake 鐧藉悕鍗曡В鏋愮殑绛栫暐 handler銆?
func newTestPolicyHandler(fn func(ctx context.Context, tenantID, userID string) ([]string, bool, error)) *EntPolicyHandler {
	return &EntPolicyHandler{resolveAllowed: fn}
}

// TestEnforceEnterprisePolicy_Denied 鐧藉悕鍗曞妯″瀷 鈫?403 璇箟閿欒銆?
func TestEnforceEnterprisePolicy_Denied(t *testing.T) {
	h := newTestPolicyHandler(func(ctx context.Context, tenantID, userID string) ([]string, bool, error) {
		return []string{"gpt-4", "claude-3"}, true, nil
	})
	r := httptest.NewRequest(http.MethodPost, "/submit", nil)
	claims := &auth.Claims{UserID: "u1", TenantID: "t1"}

	if err := h.EnforceEnterprisePolicy(r, claims, "evil-model"); !errors.Is(err, ErrModelNotAllowed) {
		t.Fatalf("expected ErrModelNotAllowed, got %v", err)
	}
	if err := h.EnforceEnterprisePolicy(r, claims, "gpt-4"); err != nil {
		t.Fatalf("expected allowed model to pass, got %v", err)
	}
}

// TestEnforceEnterprisePolicy_NoPolicy 鏃犱换浣曠瓥鐣?鈫?鏀捐銆?
func TestEnforceEnterprisePolicy_NoPolicy(t *testing.T) {
	h := newTestPolicyHandler(func(ctx context.Context, tenantID, userID string) ([]string, bool, error) {
		return nil, false, nil
	})
	r := httptest.NewRequest(http.MethodPost, "/submit", nil)
	claims := &auth.Claims{UserID: "u1", TenantID: "t1"}
	if err := h.EnforceEnterprisePolicy(r, claims, "any-model"); err != nil {
		t.Fatalf("expected pass-through without policy, got %v", err)
	}
}

// TestEnforceEnterprisePolicy_FailOpen 鏌ヨ澶辫触 鈫?fail-open 鏀捐銆?
func TestEnforceEnterprisePolicy_FailOpen(t *testing.T) {
	h := newTestPolicyHandler(func(ctx context.Context, tenantID, userID string) ([]string, bool, error) {
		return nil, false, errors.New("pg down")
	})
	r := httptest.NewRequest(http.MethodPost, "/submit", nil)
	claims := &auth.Claims{UserID: "u1", TenantID: "t1"}
	if err := h.EnforceEnterprisePolicy(r, claims, "any-model"); err != nil {
		t.Fatalf("expected fail-open on query error, got %v", err)
	}
}

// TestEnforceEnterprisePolicy_EdgeCases nil claims / 绌烘ā鍨嬪悕鍧囨斁琛岋紙闃插尽鎬э級銆?
func TestEnforceEnterprisePolicy_EdgeCases(t *testing.T) {
	called := false
	h := newTestPolicyHandler(func(ctx context.Context, tenantID, userID string) ([]string, bool, error) {
		called = true
		return []string{}, true, nil
	})
	r := httptest.NewRequest(http.MethodPost, "/submit", nil)
	if err := h.EnforceEnterprisePolicy(r, nil, "gpt-4"); err != nil {
		t.Fatalf("expected fail-open with nil claims, got %v", err)
	}
	if err := h.EnforceEnterprisePolicy(r, &auth.Claims{UserID: "u1"}, ""); err != nil {
		t.Fatalf("expected pass-through with empty model, got %v", err)
	}
	if called {
		t.Fatal("resolver should not be called for nil claims / empty model")
	}
}

// TestEnforceEnterprisePolicy_FailOpenWithoutDB 鍖呯骇鍏ュ彛鍦ㄦ棤 PG 鐜涓?
// 锛圧eadPool 涓?nil锛夊繀椤?fail-open锛屼笉寰楅樆鏂姹傘€?
func TestEnforceEnterprisePolicy_FailOpenWithoutDB(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/submit", nil)
	claims := &auth.Claims{UserID: "u1", TenantID: "t1"}
	if err := EnforceEnterprisePolicy(r, claims, "gpt-4"); err != nil {
		t.Fatalf("expected fail-open without DB, got %v", err)
	}
}
