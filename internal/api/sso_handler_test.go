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

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// 鈹€鈹€ fake 鍩虹璁炬柦 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

type fakeRow struct {
	scan func(dest ...any) error
}

func (f *fakeRow) Scan(dest ...any) error {
	if f.scan == nil {
		return pgx.ErrNoRows
	}
	return f.scan(dest...)
}

// fakeQuerier 鎸?SQL 鍏抽敭瀛楀垎鍙戦璁捐涓猴紝渚?SSO/韬唤绠＄悊 handler 娴嬭瘯澶嶇敤銆?
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

// fakeExchanger 鏄?auth.OIDCExchanger 鐨勬祴璇曟浛韬紙涓嶈Е缃戯級銆?
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
		oauth2:     idp, // 娴嬭瘯鍚屾牱娉ㄥ叆 fake
		codec:      auth.NewStateCodec(testEncKey, time.Minute),
		encKey:     testEncKey,
		successURL: "/",
		bindURL:    "/profile?bind=ok",
	}
}

// providerScan 妯℃嫙 ent_oidc_providers 琛屾壂鎻忥紙鍒楅『搴忎笌 ssoProviderColumns 涓€鑷达紝21 鍒楋級銆?
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
		*dest[10].(*string) = "oidc"
		*dest[11].(*string) = "custom"
		*dest[12].(*string) = "" // display_name
		*dest[13].(*string) = "" // icon
		*dest[14].(*int) = 100
		*dest[15].(*string) = "" // auth_url
		*dest[16].(*string) = "" // token_url
		*dest[17].(*string) = "" // userinfo_url
		*dest[18].(*[]byte) = []byte(`{}`)
		*dest[19].(*time.Time) = time.Now()
		*dest[20].(*time.Time) = time.Now()
		return nil
	}
}

// 鈹€鈹€ 鑴辨晱閫昏緫 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

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
	// 浠讳綍瀛楁閮戒笉寰楁惡甯﹀瘑鏂?
	if raw, _ := json.Marshal(resp); strings.Contains(string(raw), "encrypted-blob") {
		t.Fatal("response must not contain the encrypted secret either")
	}
}

// 鈹€鈹€ callback state 鏍￠獙鍒嗘敮 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

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

	// state 鏍￠獙澶辫触蹇呴』鍦ㄨЕ纰?DB 涔嬪墠浠?400 鎷掔粷
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for tampered state, got %d", w.Code)
	}
}

func TestSSOCallback_ExpiredState(t *testing.T) {
	h := newTestSSOHandler(&fakeQuerier{}, &fakeExchanger{})

	// 鐢ㄦ嫧蹇?2 鍒嗛挓鐨勬椂閽熺鍙?鈫?鐩稿鐪熷疄鏃堕挓宸茶繃鏈?
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

// 鈹€鈹€ 绠＄悊绔細瀵嗛挜缂哄け 503 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

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

// 鈹€鈹€ role_mapping 瑙ｆ瀽 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

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

// 鈹€鈹€ normalizeProviderInput 鍗忚鏍￠獙涓庢ā鏉垮～鍏?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestNormalizeProviderInput(t *testing.T) {
	// 闈炴硶鍗忚
	if _, _, _, _, _, _, _, err := normalizeProviderInput("saml", "", "", "", "", "", nil); err == nil {
		t.Fatal("expected error for unknown protocol")
	}
	// 鏈煡 provider_type
	if _, _, _, _, _, _, _, err := normalizeProviderInput("oidc", "myspace", "", "", "", "", nil); err == nil {
		t.Fatal("expected error for unknown provider_type")
	}
	// github oauth2 妯℃澘绔偣鑷姩濉厖 + 缂虹渷 scopes
	issuer, protocol, ptype, authURL, tokenURL, userinfoURL, scopes, err :=
		normalizeProviderInput("", "github", "", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if protocol != auth.ProtocolOAuth2 || ptype != auth.ProviderGitHub {
		t.Fatalf("protocol/type = %q/%q", protocol, ptype)
	}
	if issuer != "" || authURL == "" || tokenURL == "" || userinfoURL == "" {
		t.Fatalf("github template endpoints not filled: %q %q %q", authURL, tokenURL, userinfoURL)
	}
	if len(scopes) == 0 {
		t.Fatal("default scopes not filled")
	}
}

// 鈹€鈹€ Login bind 妯″紡鐧诲綍鎬佹牎楠?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// bind 妯″紡鏃犵櫥褰曟€侊紙鏃?claims / 鏃?cookie锛夆啋 401锛屼笉寰楃鍙?state銆?
func TestSSOLogin_BindModeWithoutLogin_401(t *testing.T) {
	store := &fakeQuerier{queryRow: func(sql string, args ...any) pgx.Row {
		return &fakeRow{scan: providerScan(true, true)}
	}}
	h := newTestSSOHandler(store, &fakeExchanger{authURL: "https://idp.example.com/authorize"})

	req := httptest.NewRequest("GET", "/v1/auth/sso/login/11111111-1111-1111-1111-111111111111?mode=bind", nil)
	req.SetPathValue("providerID", "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bind without login, got %d", w.Code)
	}
}

// bind 妯″紡鎼哄甫鐧诲綍鎬?鈫?302 鍒?IdP锛宻tate 鎼哄甫 bind+uid銆?
func TestSSOLogin_BindMode_Redirects(t *testing.T) {
	store := &fakeQuerier{queryRow: func(sql string, args ...any) pgx.Row {
		return &fakeRow{scan: providerScan(true, true)}
	}}
	h := newTestSSOHandler(store, &fakeExchanger{authURL: "https://idp.example.com/authorize"})

	req := requestWithClaims("GET", "/v1/auth/sso/login/11111111-1111-1111-1111-111111111111?mode=bind", "", userClaims("user-1"))
	req.SetPathValue("providerID", "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://idp.example.com/authorize" {
		t.Fatalf("location = %q", loc)
	}
}

// 鈹€鈹€ bind 鍥炶皟鍒嗘敮 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// bindStore 鎸?SQL 鍒嗗彂 bind 鍥炶皟鍚勯樁娈佃涓恒€?
func bindStore(identityScan func(dest ...any) error, insertErr error) *fakeQuerier {
	return &fakeQuerier{
		queryRow: func(sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "ent_oidc_providers"):
				return &fakeRow{scan: providerScan(true, true)}
			case strings.Contains(sql, "ent_user_identities"):
				return &fakeRow{scan: identityScan}
			default:
				return &fakeRow{}
			}
		},
		exec: func(sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "INSERT INTO ent_user_identities") {
				return pgconn.NewCommandTag("INSERT 0 1"), insertErr
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
}

func bindCallbackRequest(t *testing.T, h *SSOHandler, uid string) *httptest.ResponseRecorder {
	t.Helper()
	state, err := h.codec.IssueMode("11111111-1111-1111-1111-111111111111", "nonce-1", auth.StateModeBind, uid)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/v1/auth/sso/callback?code=abc&state="+state, nil)
	w := httptest.NewRecorder()
	h.Callback(w, req)
	return w
}

func identityScanRow(userID string) func(dest ...any) error {
	return func(dest ...any) error {
		*dest[0].(*string) = userID
		*dest[1].(*string) = "bound@example.com"
		*dest[2].(*string) = "Bound User"
		*dest[3].(*string) = "user"
		return nil
	}
}

// 宸茬粦瀹氫粬浜?鈫?409 涓枃鎻愮ず銆?
func TestSSOCallback_BindConflict_409(t *testing.T) {
	store := bindStore(identityScanRow("user-2"), nil)
	h := newTestSSOHandler(store, &fakeExchanger{result: &auth.IDTokenResult{Subject: "sub-1"}})

	w := bindCallbackRequest(t, h, "user-1")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d (body=%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "宸茬粦瀹氬叾浠栫敤鎴?) {
		t.Fatalf("expected conflict message, got %s", w.Body.String())
	}
}

// 宸茬粦瀹氭湰浜?鈫?骞傜瓑 302 鍒?bindURL銆?
func TestSSOCallback_BindSelf_Idempotent(t *testing.T) {
	store := bindStore(identityScanRow("user-1"), nil)
	h := newTestSSOHandler(store, &fakeExchanger{result: &auth.IDTokenResult{Subject: "sub-1"}})

	w := bindCallbackRequest(t, h, "user-1")
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d (body=%s)", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != h.bindURL {
		t.Fatalf("location = %q", loc)
	}
}

// 鏈粦瀹?鈫?INSERT 鎴愬姛 鈫?302 鍒?bindURL銆?
func TestSSOCallback_BindSuccess(t *testing.T) {
	inserted := false
	store := bindStore(func(dest ...any) error { return pgx.ErrNoRows }, nil)
	store.exec = func(sql string, args ...any) (pgconn.CommandTag, error) {
		if strings.Contains(sql, "INSERT INTO ent_user_identities") {
			inserted = true
			if args[0] != "user-1" {
				t.Errorf("insert user_id = %v, want user-1", args[0])
			}
		}
		return pgconn.NewCommandTag("INSERT 0 1"), nil
	}
	h := newTestSSOHandler(store, &fakeExchanger{result: &auth.IDTokenResult{Subject: "sub-1"}})

	w := bindCallbackRequest(t, h, "user-1")
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d (body=%s)", w.Code, w.Body.String())
	}
	if !inserted {
		t.Fatal("expected identity INSERT to be executed")
	}
}

// INSERT 鍞竴鍐茬獊锛堝苟鍙戠粦瀹氾級鈫?409銆?
func TestSSOCallback_BindUniqueViolation_409(t *testing.T) {
	store := bindStore(func(dest ...any) error { return pgx.ErrNoRows },
		&pgconn.PgError{Code: "23505"})
	h := newTestSSOHandler(store, &fakeExchanger{result: &auth.IDTokenResult{Subject: "sub-1"}})

	w := bindCallbackRequest(t, h, "user-1")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

// 鈹€鈹€ DeleteIdentity 瑙ｇ粦瀹堝崼 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func deleteIdentityRequest(h *SSOHandler, passwordSet bool, count int) *httptest.ResponseRecorder {
	store := &fakeQuerier{
		queryRow: func(sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "u.password_set"):
				return &fakeRow{scan: func(dest ...any) error {
					*dest[0].(*bool) = passwordSet
					*dest[1].(*int) = count
					return nil
				}}
			case strings.Contains(sql, "SELECT subject FROM ent_user_identities"):
				return &fakeRow{scan: func(dest ...any) error {
					*dest[0].(*string) = "sub-1"
					return nil
				}}
			default:
				return &fakeRow{}
			}
		},
		exec: func(sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	h.db = store
	req := requestWithClaims("DELETE", "/v1/auth/sso/identities/22222222-2222-2222-2222-222222222222", "", userClaims("user-1"))
	req.SetPathValue("id", "22222222-2222-2222-2222-222222222222")
	w := httptest.NewRecorder()
	h.DeleteIdentity(w, req)
	return w
}

// 鏃犲瘑鐮佷笖鏈€鍚庝竴涓笁鏂硅韩浠?鈫?403锛堜繚搴曡嚦灏戜竴绉嶇櫥褰曟柟寮忥級銆?
func TestDeleteIdentity_LastIdentityNoPassword_403(t *testing.T) {
	h := newTestSSOHandler(&fakeQuerier{}, &fakeExchanger{})
	w := deleteIdentityRequest(h, false, 1)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// 宸茶瀵嗙爜 鈫?鍏佽瑙ｇ粦銆?
func TestDeleteIdentity_WithPassword_OK(t *testing.T) {
	h := newTestSSOHandler(&fakeQuerier{}, &fakeExchanger{})
	w := deleteIdentityRequest(h, true, 1)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// 澶氫釜涓夋柟韬唤 鈫?鍏佽瑙ｇ粦銆?
func TestDeleteIdentity_MultipleIdentities_OK(t *testing.T) {
	h := newTestSSOHandler(&fakeQuerier{}, &fakeExchanger{})
	w := deleteIdentityRequest(h, false, 2)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// 鈹€鈹€ SetPassword 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func passwordStore(t *testing.T, passwordHash string, passwordSet bool) *fakeQuerier {
	t.Helper()
	return &fakeQuerier{
		queryRow: func(sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "password_hash, password_set") {
				return &fakeRow{scan: func(dest ...any) error {
					*dest[0].(*string) = passwordHash
					*dest[1].(*bool) = passwordSet
					return nil
				}}
			}
			return &fakeRow{}
		},
		exec: func(sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
}

// SSO 寤哄彿鐢ㄦ埛棣栬瀵嗙爜锛氭棤闇€ current_password銆?
func TestSetPassword_FirstSetNoCurrentRequired(t *testing.T) {
	h := newTestSSOHandler(passwordStore(t, "", false), &fakeExchanger{})

	body := `{"new_password":"new-strong-pass"}`
	req := requestWithClaims("POST", "/v1/auth/password", body, userClaims("user-1"))
	w := httptest.NewRecorder()
	h.SetPassword(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// 宸茶瀵嗙爜浣嗘棫瀵嗙爜閿欒 鈫?401銆?
func TestSetPassword_WrongCurrent_401(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	h := newTestSSOHandler(passwordStore(t, string(hash), true), &fakeExchanger{})

	body := `{"current_password":"wrong","new_password":"new-strong-pass"}`
	req := requestWithClaims("POST", "/v1/auth/password", body, userClaims("user-1"))
	w := httptest.NewRecorder()
	h.SetPassword(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// 宸茶瀵嗙爜浣嗙己 current_password 鈫?400銆?
func TestSetPassword_MissingCurrent_400(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	h := newTestSSOHandler(passwordStore(t, string(hash), true), &fakeExchanger{})

	body := `{"new_password":"new-strong-pass"}`
	req := requestWithClaims("POST", "/v1/auth/password", body, userClaims("user-1"))
	w := httptest.NewRecorder()
	h.SetPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// 鏂板瘑鐮侀暱搴︿笉瓒?鈫?400銆?
func TestSetPassword_TooShort_400(t *testing.T) {
	h := newTestSSOHandler(passwordStore(t, "", false), &fakeExchanger{})

	body := `{"new_password":"short"}`
	req := requestWithClaims("POST", "/v1/auth/password", body, userClaims("user-1"))
	w := httptest.NewRecorder()
	h.SetPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
