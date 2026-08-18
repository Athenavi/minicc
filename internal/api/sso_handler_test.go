package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ── fake 基础设施 ───────────────────────────────────────

type fakeRow struct {
	scan func(dest ...any) error
}

func (f *fakeRow) Scan(dest ...any) error {
	if f.scan == nil {
		return pgx.ErrNoRows
	}
	return f.scan(dest...)
}

// fakeQuerier 按 SQL 关键字分发预设行为，供 SSO/身份管理 handler 测试复用。
type fakeQuerier struct {
	queryRow func(sql string, args ...any) pgx.Row
	exec     func(sql string, args ...any) (pgconn.CommandTag, error)
}

func (f *fakeQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, errors.New("fake: Query not implemented")
}

func (f *fakeQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if f.queryRow == nil {
		return &fakeRow{}
	}
	return f.queryRow(sql, args...)
}

func (f *fakeQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if f.exec == nil {
		return pgconn.CommandTag{}, errors.New("fake: Exec not implemented")
	}
	return f.exec(sql, args...)
}

// fakeExchanger 是 auth.OIDCExchanger 的测试替身（不触网）。
type fakeExchanger struct {
	authURL string
	authErr error
	result  *auth.IDTokenResult
	exchErr error
}

func (f *fakeExchanger) AuthURL(ctx context.Context, p *auth.OIDCProviderConfig, state, nonce string) (string, error) {
	return f.authURL, f.authErr
}

func (f *fakeExchanger) ExchangeAndVerify(ctx context.Context, p *auth.OIDCProviderConfig, code, expectedNonce string) (*auth.IDTokenResult, error) {
	return f.result, f.exchErr
}

var testEncKey = []byte("0123456789abcdef0123456789abcdef")

func newTestSSOHandler(store entQuerier, idp auth.OIDCExchanger) *SSOHandler {
	return &SSOHandler{
		auth:       auth.NewAuthenticator(strings.Repeat("s", 32), time.Hour),
		cfg:        &config.Config{JWTExpiration: time.Hour},
		db:         store,
		exchanger:  idp,
		codec:      auth.NewStateCodec(testEncKey, time.Minute),
		encKey:     testEncKey,
		successURL: "/",
	}
}

// providerScan 模拟 ent_oidc_providers 行扫描（列顺序与 ssoProviderColumns 一致）。
func providerScan(enabled, autoProvision bool) func(dest ...any) error {
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "idp-secret")
	return func(dest ...any) error {
		*dest[0].(*string) = "11111111-1111-1111-1111-111111111111"
		*dest[1].(*string) = "00000000-0000-0000-0000-000000000001"
		*dest[2].(*string) = "okta"
		*dest[3].(*string) = "https://idp.example.com"
		*dest[4].(*string) = "client-1"
		*dest[5].(*string) = secretEnc
		*dest[6].(*[]string) = []string{"openid", "email", "profile"}
		*dest[7].(*bool) = enabled
		*dest[8].(*bool) = autoProvision
		*dest[9].(*[]byte) = []byte(`{"idp-admin":"admin"}`)
		*dest[10].(*time.Time) = time.Now()
		*dest[11].(*time.Time) = time.Now()
		return nil
	}
}

// ── 脱敏逻辑 ────────────────────────────────────────────

func TestSanitizeProvider_MasksSecret(t *testing.T) {
	p := &ssoProvider{
		ID:              "11111111-1111-1111-1111-111111111111",
		Name:            "okta",
		Issuer:          "https://idp.example.com",
		ClientID:        "client-1",
		ClientSecretEnc: "encrypted-blob",
	}
	resp := sanitizeProvider(p)

	if got := resp["client_secret"]; got != maskedSecret {
		t.Fatalf("client_secret must be masked, got %v", got)
	}
	// 任何字段都不得携带密文
	if raw, _ := json.Marshal(resp); strings.Contains(string(raw), "encrypted-blob") {
		t.Fatal("response must not contain the encrypted secret either")
	}
}

// ── callback state 校验分支 ─────────────────────────────

func TestSSOCallback_MissingParams(t *testing.T) {
	h := newTestSSOHandler(&fakeQuerier{}, &fakeExchanger{})

	req := httptest.NewRequest("GET", "/v1/auth/sso/callback", nil)
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSSOCallback_MalformedState(t *testing.T) {
	h := newTestSSOHandler(&fakeQuerier{}, &fakeExchanger{})

	req := httptest.NewRequest("GET", "/v1/auth/sso/callback?code=abc&state=garbage", nil)
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed state, got %d", w.Code)
	}
}

func TestSSOCallback_TamperedState(t *testing.T) {
	h := newTestSSOHandler(&fakeQuerier{}, &fakeExchanger{})

	state, err := h.codec.Issue("11111111-1111-1111-1111-111111111111", "nonce-1")
	if err != nil {
		t.Fatal(err)
	}
	tampered := state[:len(state)-3] + "AAA"

	req := httptest.NewRequest("GET", "/v1/auth/sso/callback?code=abc&state="+tampered, nil)
	w := httptest.NewRecorder()
	h.Callback(w, req)

	// state 校验失败必须在触碰 DB 之前以 400 拒绝
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for tampered state, got %d", w.Code)
	}
}

func TestSSOCallback_ExpiredState(t *testing.T) {
	h := newTestSSOHandler(&fakeQuerier{}, &fakeExchanger{})

	// 用拨快 2 分钟的时钟签发 → 相对真实时钟已过期
	pastCodec := auth.NewStateCodecWithClock(testEncKey, time.Minute,
		func() time.Time { return time.Now().Add(2 * time.Minute) })
	state, err := pastCodec.Issue("11111111-1111-1111-1111-111111111111", "nonce-1")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/v1/auth/sso/callback?code=abc&state="+state, nil)
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for expired state, got %d", w.Code)
	}
}

func TestSSOCallback_NoBinding_NoAutoProvision_Forbidden(t *testing.T) {
	store := &fakeQuerier{
		queryRow: func(sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "ent_oidc_providers"):
				return &fakeRow{scan: providerScan(true, false)} // enabled, auto_provision=false
			case strings.Contains(sql, "ent_user_identities"):
				return &fakeRow{scan: func(dest ...any) error { return pgx.ErrNoRows }}
			default:
				return &fakeRow{}
			}
		},
	}
	idp := &fakeExchanger{result: &auth.IDTokenResult{Subject: "sub-1", Email: "u@example.com"}}
	h := newTestSSOHandler(store, idp)

	state, err := h.codec.Issue("11111111-1111-1111-1111-111111111111", "nonce-1")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/v1/auth/sso/callback?code=abc&state="+state, nil)
	w := httptest.NewRecorder()
	h.Callback(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unprovisioned subject, got %d", w.Code)
	}
}

// ── 管理端：密钥缺失 503 ────────────────────────────────

func TestCreateProvider_MissingKey_503(t *testing.T) {
	h := newTestSSOHandler(&fakeQuerier{}, &fakeExchanger{})
	h.encKey = nil

	body := `{"name":"okta","issuer":"https://idp","client_id":"c","client_secret":"s"}`
	req := httptest.NewRequest("POST", "/v1/ent/sso/providers", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateProvider(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without ENT_OIDC_SECRET_KEY, got %d", w.Code)
	}
}

// ── role_mapping 解析 ───────────────────────────────────

func TestResolveRole(t *testing.T) {
	mapping := map[string]string{"idp-admin": "admin", "idp-user": "user"}
	if got := resolveRole(mapping, []string{"idp-admin"}); got != "admin" {
		t.Fatalf("expected admin, got %q", got)
	}
	if got := resolveRole(mapping, []string{"unknown"}); got != "user" {
		t.Fatalf("expected default user, got %q", got)
	}
	if got := resolveRole(nil, nil); got != "user" {
		t.Fatalf("expected default user for nil mapping, got %q", got)
	}
}
