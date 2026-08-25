package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// 鈹€鈹€ fake 鍩虹璁炬柦 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// fakeCaptchaVerifier 鏄?auth.CaptchaVerifier 鐨勬祴璇曟浛韬€?
type fakeCaptchaVerifier struct {
	err      error
	lastCfg  *auth.CaptchaConfig
	lastTok  *auth.CaptchaToken
	lastIP   string
	callCount int
}

func (f *fakeCaptchaVerifier) Verify(ctx context.Context, cfg *auth.CaptchaConfig, tok *auth.CaptchaToken, remoteIP string) error {
	f.callCount++
	f.lastCfg, f.lastTok, f.lastIP = cfg, tok, remoteIP
	return f.err
}

// memCounter 鏄?failCounterStore 鐨勫唴瀛樺疄鐜般€?
type memCounter struct {
	counts    map[string]int
	incrCalls int
	clearIPs  []string
}

func newMemCounter() *memCounter { return &memCounter{counts: map[string]int{}} }

func (m *memCounter) incr(ctx context.Context, ip string, window time.Duration) {
	m.incrCalls++
	m.counts[ip]++
}

func (m *memCounter) get(ctx context.Context, ip string) int { return m.counts[ip] }

func (m *memCounter) clear(ctx context.Context, ip string) {
	m.clearIPs = append(m.clearIPs, ip)
	delete(m.counts, ip)
}

// captchaScan 鏋勯€?ent_captcha_config 琛屾壂鎻忥紙鍒楀簭涓?loadConfig 涓€鑷达級銆?
func captchaScan(provider, siteKey, secretEnc string, enabled bool) func(dest ...any) error {
	return func(dest ...any) error {
		*dest[0].(*string) = provider
		*dest[1].(*string) = siteKey
		*dest[2].(*string) = secretEnc
		verifyURL := ""
		*dest[3].(**string) = &verifyURL
		*dest[4].(*bool) = enabled
		return nil
	}
}

func newTestCaptchaHandler(rowScan func(dest ...any) error, v auth.CaptchaVerifier, counter failCounterStore) *CaptchaHandler {
	return &CaptchaHandler{
		db: &fakeQuerier{
			queryRow: func(sql string, args ...any) pgx.Row {
				if strings.Contains(sql, "ent_captcha_config") {
					if rowScan == nil {
						return &fakeRow{} // ErrNoRows 鈫?鏈厤缃?
					}
					return &fakeRow{scan: rowScan}
				}
				return &fakeRow{}
			},
			exec: func(sql string, args ...any) (pgconn.CommandTag, error) {
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			},
		},
		encKey:   testEncKey,
		verifier: v,
		counter:  counter,
	}
}

func testCaptchaReq(method, target, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, target, nil)
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// 鈹€鈹€ PublicConfig锛氬彧涓嬪彂闈炴晱鎰熷瓧娈?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestCaptchaPublicConfig_Disabled(t *testing.T) {
	h := newTestCaptchaHandler(nil, &fakeCaptchaVerifier{}, nil)
	w := httptest.NewRecorder()
	h.PublicConfig(w, testCaptchaReq("GET", "/v1/auth/captcha/config", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("expected enabled=false, got %s", w.Body.String())
	}
}

func TestCaptchaPublicConfig_EnabledNoSecretLeak(t *testing.T) {
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "top-captcha-secret")
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "site-key-1", secretEnc, true), &fakeCaptchaVerifier{}, nil)

	w := httptest.NewRecorder()
	h.PublicConfig(w, testCaptchaReq("GET", "/v1/auth/captcha/config", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{`"enabled":true`, "turnstile", "site-key-1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, secretEnc) || strings.Contains(body, "top-captcha-secret") {
		t.Fatalf("secret leaked: %s", body)
	}
}

// 鈹€鈹€ GetConfig锛歴ecret 鑴辨晱鍥炴樉 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestCaptchaGetConfig_MasksSecret(t *testing.T) {
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "top-captcha-secret")
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "site-key-1", secretEnc, true), &fakeCaptchaVerifier{}, nil)

	w := httptest.NewRecorder()
	h.GetConfig(w, testCaptchaReq("GET", "/v1/ent/captcha/config", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, maskedSecret) {
		t.Fatalf("secret must be masked, got %s", body)
	}
	if strings.Contains(body, secretEnc) || strings.Contains(body, "top-captcha-secret") {
		t.Fatalf("secret leaked: %s", body)
	}
}

// 鈹€鈹€ UpdateConfig锛氬惎鐢ㄥ墠缃牎楠?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestCaptchaUpdateConfig_Validation(t *testing.T) {
	cases := []struct {
		name string
		body string
		code int
	}{
		{"unknown provider", `{"provider":"geetest","enabled":true}`, http.StatusBadRequest},
		{"enable without secret", `{"provider":"turnstile","site_key":"k","enabled":true}`, http.StatusBadRequest},
		{"enable turnstile without site_key", `{"provider":"turnstile","enabled":true}`, http.StatusBadRequest},
		{"enable custom without verify_url", `{"provider":"custom","secret":"s","enabled":true}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestCaptchaHandler(nil, &fakeCaptchaVerifier{}, nil)
			w := httptest.NewRecorder()
			h.UpdateConfig(w, testCaptchaReq("PUT", "/v1/ent/captcha/config", tc.body))
			if w.Code != tc.code {
				t.Fatalf("expected %d, got %d (body=%s)", tc.code, w.Code, w.Body.String())
			}
		})
	}
}

// secret 浼犺劚鏁忓崰浣嶇 鈫?淇濈暀鍘熷€间笉鎶ラ敊銆?
func TestCaptchaUpdateConfig_MaskedSecretPlaceholderKept(t *testing.T) {
	origEnc, _ := auth.EncryptAESGCM(testEncKey, "orig-secret")
	// 鏃㈡湁閰嶇疆锛堟湭鍚敤锛夛紝鍗犱綅绗︽洿鏂板悗 secret 瀵嗘枃淇濇寔鍘熷€?
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "k", origEnc, false), &fakeCaptchaVerifier{}, nil)
	w := httptest.NewRecorder()
	h.UpdateConfig(w, testCaptchaReq("PUT", "/v1/ent/captcha/config",
		`{"provider":"turnstile","site_key":"k","secret":"`+maskedSecret+`","enabled":false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// custom + verify_url + secret 鈫?鍏佽鍚敤銆?
func TestCaptchaUpdateConfig_CustomOK(t *testing.T) {
	// 鏃㈡湁 custom 閰嶇疆锛堟湭鍚敤锛宻ecret 宸查厤缃級锛屽惎鐢ㄥ苟琛?verify_url
	origEnc, _ := auth.EncryptAESGCM(testEncKey, "orig-secret")
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaCustom, "", origEnc, false), &fakeCaptchaVerifier{}, nil)
	w := httptest.NewRecorder()
	h.UpdateConfig(w, testCaptchaReq("PUT", "/v1/ent/captcha/config",
		`{"provider":"custom","verify_url":"https://cap.example.com/verify","enabled":true}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// 鈹€鈹€ Enforce 闃叉互鐢ㄦ爡鏍?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func testCaptchaIP() string { return "192.0.2.1" }

func enforceRequest() *http.Request {
	req := httptest.NewRequest("POST", "/v1/auth/login", nil)
	req.RemoteAddr = testCaptchaIP() + ":1234"
	return req
}

// 鏈惎鐢ㄤ笖澶辫触鏈揪闃堝€?鈫?鏀捐锛坣il锛夈€?
func TestCaptchaEnforce_NotConfigured_Pass(t *testing.T) {
	h := newTestCaptchaHandler(nil, &fakeCaptchaVerifier{}, newMemCounter())
	w := httptest.NewRecorder()
	if err := h.Enforce(w, enforceRequest(), nil); err != nil {
		t.Fatalf("expected pass, got %v", w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("no response should be written, got %d", w.Code)
	}
}

// 杈剧‖涓婇檺 鈫?429銆?
func TestCaptchaEnforce_HardLimit_429(t *testing.T) {
	counter := newMemCounter()
	counter.counts[testCaptchaIP()] = captchaHardLimit
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "s")
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "k", secretEnc, true), &fakeCaptchaVerifier{}, counter)

	w := httptest.NewRecorder()
	if err := h.Enforce(w, enforceRequest(), nil); err != errCaptchaHandled {
		t.Fatalf("expected errCaptchaHandled, got %v", err)
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
}

// 宸插惎鐢ㄤ絾缂?token 鈫?428 captcha_required锛堝墠绔姞杞介獙璇佺爜缁勪欢鐨勪俊鍙凤級銆?
func TestCaptchaEnforce_MissingToken_428(t *testing.T) {
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "s")
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "k", secretEnc, true), &fakeCaptchaVerifier{}, newMemCounter())

	w := httptest.NewRecorder()
	if err := h.Enforce(w, enforceRequest(), nil); err != errCaptchaHandled {
		t.Fatalf("expected errCaptchaHandled, got %v", err)
	}
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected 428, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "captcha_required") {
		t.Fatalf("expected captcha_required error, got %s", w.Body.String())
	}
}

// token 鏍￠獙澶辫触 鈫?403銆?
func TestCaptchaEnforce_VerifyFailed_403(t *testing.T) {
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "s")
	v := &fakeCaptchaVerifier{err: auth.ErrCaptchaFailed}
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "k", secretEnc, true), v, newMemCounter())

	w := httptest.NewRecorder()
	tok := &auth.CaptchaToken{Token: "tok"}
	if err := h.Enforce(w, enforceRequest(), tok); err != errCaptchaHandled {
		t.Fatalf("expected errCaptchaHandled, got %v", err)
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

// 鏈嶅姟鍟嗕笉鍙揪 鈫?fail-loud 502锛岀粷涓嶉潤榛樻斁琛屻€?
func TestCaptchaEnforce_Unreachable_502(t *testing.T) {
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "s")
	v := &fakeCaptchaVerifier{err: auth.ErrCaptchaUnreachable}
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "k", secretEnc, true), v, newMemCounter())

	w := httptest.NewRecorder()
	tok := &auth.CaptchaToken{Token: "tok"}
	if err := h.Enforce(w, enforceRequest(), tok); err != errCaptchaHandled {
		t.Fatalf("expected errCaptchaHandled, got %v", err)
	}
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", w.Code)
	}
}

// 鏍￠獙鎴愬姛 鈫?鏀捐銆?
func TestCaptchaEnforce_VerifyOK_Pass(t *testing.T) {
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "cap-secret")
	v := &fakeCaptchaVerifier{}
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "k", secretEnc, true), v, newMemCounter())

	w := httptest.NewRecorder()
	tok := &auth.CaptchaToken{Token: "tok"}
	if err := h.Enforce(w, enforceRequest(), tok); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
	if v.callCount != 1 {
		t.Fatalf("verifier called %d times", v.callCount)
	}
	if v.lastIP != testCaptchaIP() {
		t.Fatalf("remoteIP = %q", v.lastIP)
	}
	if v.lastCfg.Provider != auth.CaptchaTurnstile || v.lastCfg.Secret != "cap-secret" {
		t.Fatalf("verifier cfg = %+v", v.lastCfg)
	}
}

// 鏈叏灞€鍚敤浣嗗悓 IP 澶辫触 鈮?闃堝€间笖宸查厤缃?secret 鈫?鍗囩骇寮哄埗楠岃瘉鐮侊紙428锛夈€?
func TestCaptchaEnforce_FailEscalation_428(t *testing.T) {
	counter := newMemCounter()
	counter.counts[testCaptchaIP()] = captchaFailThreshold
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "s")
	// enabled=false 浣嗗凡閰嶇疆
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "k", secretEnc, false), &fakeCaptchaVerifier{}, counter)

	w := httptest.NewRecorder()
	if err := h.Enforce(w, enforceRequest(), nil); err != errCaptchaHandled {
		t.Fatalf("expected errCaptchaHandled, got %v", err)
	}
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected 428 after fail escalation, got %d", w.Code)
	}
}

// 鍚敤浣?secret 缂哄け 鈫?fail-loud 503锛岀粷涓嶉潤榛樻斁琛屻€?
func TestCaptchaEnforce_EnabledWithoutSecret_503(t *testing.T) {
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "k", "", true), &fakeCaptchaVerifier{}, newMemCounter())

	w := httptest.NewRecorder()
	if err := h.Enforce(w, enforceRequest(), &auth.CaptchaToken{Token: "tok"}); err != errCaptchaHandled {
		t.Fatalf("expected errCaptchaHandled, got %v", err)
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// 鈹€鈹€ 澶辫触璁℃暟鑱斿姩 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestCaptchaRecordAndClearFailures(t *testing.T) {
	counter := newMemCounter()
	h := newTestCaptchaHandler(nil, &fakeCaptchaVerifier{}, counter)

	req := enforceRequest()
	h.RecordFailure(context.Background(), req)
	h.RecordFailure(context.Background(), req)
	if counter.counts[testCaptchaIP()] != 2 {
		t.Fatalf("fail count = %d", counter.counts[testCaptchaIP()])
	}

	h.ClearFailures(context.Background(), req)
	if counter.counts[testCaptchaIP()] != 0 {
		t.Fatal("failures must be cleared")
	}
	if len(counter.clearIPs) != 1 || counter.clearIPs[0] != testCaptchaIP() {
		t.Fatalf("clearIPs = %v", counter.clearIPs)
	}
}
