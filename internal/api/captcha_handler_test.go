package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ── fake 基础设施 ───────────────────────────────────────

// fakeCaptchaVerifier 是 auth.CaptchaVerifier 的测试替身。
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

// memCounter 是 failCounterStore 的内存实现。
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

// captchaScan 构造 ent_captcha_config 行扫描（列序与 loadConfig 一致）。
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
						return &fakeRow{} // ErrNoRows → 未配置
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

// ── PublicConfig：只下发非敏感字段 ─────────────────────

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

// ── GetConfig：secret 脱敏回显 ─────────────────────────

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

// ── UpdateConfig：启用前置校验 ─────────────────────────

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

// secret 传脱敏占位符 → 保留原值不报错。
func TestCaptchaUpdateConfig_MaskedSecretPlaceholderKept(t *testing.T) {
	origEnc, _ := auth.EncryptAESGCM(testEncKey, "orig-secret")
	// 既有配置（未启用），占位符更新后 secret 密文保持原值
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "k", origEnc, false), &fakeCaptchaVerifier{}, nil)
	w := httptest.NewRecorder()
	h.UpdateConfig(w, testCaptchaReq("PUT", "/v1/ent/captcha/config",
		`{"provider":"turnstile","site_key":"k","secret":"`+maskedSecret+`","enabled":false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// custom + verify_url + secret → 允许启用。
func TestCaptchaUpdateConfig_CustomOK(t *testing.T) {
	// 既有 custom 配置（未启用，secret 已配置），启用并补 verify_url
	origEnc, _ := auth.EncryptAESGCM(testEncKey, "orig-secret")
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaCustom, "", origEnc, false), &fakeCaptchaVerifier{}, nil)
	w := httptest.NewRecorder()
	h.UpdateConfig(w, testCaptchaReq("PUT", "/v1/ent/captcha/config",
		`{"provider":"custom","verify_url":"https://cap.example.com/verify","enabled":true}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
}

// ── Enforce 防滥用栅栏 ─────────────────────────────────

func testCaptchaIP() string { return "192.0.2.1" }

func enforceRequest() *http.Request {
	req := httptest.NewRequest("POST", "/v1/auth/login", nil)
	req.RemoteAddr = testCaptchaIP() + ":1234"
	return req
}

// 未启用且失败未达阈值 → 放行（nil）。
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

// 达硬上限 → 429。
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

// 已启用但缺 token → 428 captcha_required（前端加载验证码组件的信号）。
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

// token 校验失败 → 403。
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

// 服务商不可达 → fail-loud 502，绝不静默放行。
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

// 校验成功 → 放行。
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

// 未全局启用但同 IP 失败 ≥ 阈值且已配置 secret → 升级强制验证码（428）。
func TestCaptchaEnforce_FailEscalation_428(t *testing.T) {
	counter := newMemCounter()
	counter.counts[testCaptchaIP()] = captchaFailThreshold
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "s")
	// enabled=false 但已配置
	h := newTestCaptchaHandler(captchaScan(auth.CaptchaTurnstile, "k", secretEnc, false), &fakeCaptchaVerifier{}, counter)

	w := httptest.NewRecorder()
	if err := h.Enforce(w, enforceRequest(), nil); err != errCaptchaHandled {
		t.Fatalf("expected errCaptchaHandled, got %v", err)
	}
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected 428 after fail escalation, got %d", w.Code)
	}
}

// 启用但 secret 缺失 → fail-loud 503，绝不静默放行。
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

// ── 失败计数联动 ───────────────────────────────────────

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
