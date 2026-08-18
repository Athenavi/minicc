package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/athenavi/minicc/internal/auth"
)

// newTestPolicyHandler 构造注入 fake 白名单解析的策略 handler。
func newTestPolicyHandler(fn func(ctx context.Context, tenantID, userID string) ([]string, bool, error)) *EntPolicyHandler {
	return &EntPolicyHandler{resolveAllowed: fn}
}

// TestEnforceEnterprisePolicy_Denied 白名单外模型 → 403 语义错误。
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

// TestEnforceEnterprisePolicy_NoPolicy 无任何策略 → 放行。
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

// TestEnforceEnterprisePolicy_FailOpen 查询失败 → fail-open 放行。
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

// TestEnforceEnterprisePolicy_EdgeCases nil claims / 空模型名均放行（防御性）。
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

// TestEnforceEnterprisePolicy_FailOpenWithoutDB 包级入口在无 PG 环境下
// （ReadPool 为 nil）必须 fail-open，不得阻断请求。
func TestEnforceEnterprisePolicy_FailOpenWithoutDB(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/submit", nil)
	claims := &auth.Claims{UserID: "u1", TenantID: "t1"}
	if err := EnforceEnterprisePolicy(r, claims, "gpt-4"); err != nil {
		t.Fatalf("expected fail-open without DB, got %v", err)
	}
}
