package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newCaptchaVerifier() *HTTPCaptchaVerifier {
	return NewHTTPCaptchaVerifier()
}

// formCapture 鎹曡幏 form 璇锋眰浣撲笌鏌ヨ鍙傛暟銆?
func formCapture(resp string) (*httptest.Server, *map[string]string) {
	captured := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		for k, v := range r.PostForm {
			captured[k] = v[0]
		}
		w.Write([]byte(resp))
	}))
	return srv, &captured
}

// TestCaptchaVerify_FormProviders turnstile/recaptcha/hcaptcha 鍏辩敤 form + success 濂戠害銆?
func TestCaptchaVerify_FormProviders(t *testing.T) {
	for _, provider := range []string{CaptchaTurnstile, CaptchaRecaptcha, CaptchaHCaptcha} {
		t.Run(provider, func(t *testing.T) {
			srv, captured := formCapture(`{"success": true}`)
			defer srv.Close()

			v := newCaptchaVerifier()
			cfg := &CaptchaConfig{Provider: provider, Secret: "s3cret", VerifyURL: srv.URL}
			err := v.Verify(context.Background(), cfg, &CaptchaToken{Token: "tok-1"}, "1.2.3.4")
			if err != nil {
				t.Fatalf("expected pass, got %v", err)
			}
			if (*captured)["secret"] != "s3cret" || (*captured)["response"] != "tok-1" {
				t.Fatalf("unexpected form payload: %v", *captured)
			}
			if (*captured)["remoteip"] != "1.2.3.4" {
				t.Fatalf("expected remoteip, got %v", *captured)
			}
		})
	}
}

func TestCaptchaVerify_FormProviderFailure(t *testing.T) {
	srv, _ := formCapture(`{"success": false, "error-codes": ["invalid-input-response"]}`)
	defer srv.Close()

	v := newCaptchaVerifier()
	cfg := &CaptchaConfig{Provider: CaptchaTurnstile, Secret: "s", VerifyURL: srv.URL}
	err := v.Verify(context.Background(), cfg, &CaptchaToken{Token: "bad"}, "")
	if err == nil || !strings.Contains(err.Error(), "captcha verification failed") {
		t.Fatalf("expected ErrCaptchaFailed, got %v", err)
	}
}

func TestCaptchaVerify_Tencent(t *testing.T) {
	srv, captured := formCapture(`{"response": "1"}`)
	defer srv.Close()

	v := newCaptchaVerifier()
	cfg := &CaptchaConfig{Provider: CaptchaTencent, SiteKey: "aid-1", Secret: "appsecret", VerifyURL: srv.URL}

	// Ticket + Randstr 榻愬 鈫?閫氳繃
	if err := v.Verify(context.Background(), cfg, &CaptchaToken{Token: "ticket-1", Randstr: "rand-1"}, "9.9.9.9"); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
	if (*captured)["aid"] != "aid-1" || (*captured)["Ticket"] != "ticket-1" || (*captured)["Randstr"] != "rand-1" {
		t.Fatalf("unexpected tencent payload: %v", *captured)
	}

	// 缂?Randstr 鈫?鐩存帴澶辫触锛堜笉瑙︾綉锛?
	if err := v.Verify(context.Background(), cfg, &CaptchaToken{Token: "ticket-1"}, ""); err == nil {
		t.Fatal("expected failure without randstr")
	}

	// response != "1" 鈫?澶辫触
	srv2, _ := formCapture(`{"response": "7", "err_msg": "verify fail"}`)
	defer srv2.Close()
	cfg2 := &CaptchaConfig{Provider: CaptchaTencent, SiteKey: "aid-1", Secret: "s", VerifyURL: srv2.URL}
	if err := v.Verify(context.Background(), cfg2, &CaptchaToken{Token: "t", Randstr: "r"}, ""); err == nil {
		t.Fatal("expected failure for response != 1")
	}
}

// TestCaptchaVerify_CustomContract 鑷畾涔夌鐐瑰绾︼細POST JSON 鈫?{"success": true}銆?
func TestCaptchaVerify_CustomContract(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected json content-type, got %q", r.Header.Get("Content-Type"))
		}
		body := make([]byte, 4096)
		n, _ := r.Body.Read(body)
		if err := json.Unmarshal(body[:n], &gotBody); err != nil {
			t.Errorf("malformed json body: %v", err)
		}
		w.Write([]byte(`{"success": true}`))
	}))
	defer srv.Close()

	v := newCaptchaVerifier()
	cfg := &CaptchaConfig{Provider: CaptchaCustom, Secret: "my-secret", VerifyURL: srv.URL}
	if err := v.Verify(context.Background(), cfg, &CaptchaToken{Token: "tok"}, "3.3.3.3"); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
	if gotBody["secret"] != "my-secret" || gotBody["token"] != "tok" || gotBody["remote_ip"] != "3.3.3.3" {
		t.Fatalf("unexpected custom payload: %v", gotBody)
	}

	// custom 鏈厤 verify_url 鈫?閰嶇疆閿欒
	if err := v.Verify(context.Background(), &CaptchaConfig{Provider: CaptchaCustom}, &CaptchaToken{Token: "t"}, ""); err == nil {
		t.Fatal("expected error when verify_url missing")
	}
}

// TestCaptchaVerify_CustomFailureStatus 鑷畾涔夌鐐归潪 200 鈫?鎷掔粷銆?
func TestCaptchaVerify_CustomFailureStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"success": false}`))
	}))
	defer srv.Close()

	v := newCaptchaVerifier()
	cfg := &CaptchaConfig{Provider: CaptchaCustom, Secret: "s", VerifyURL: srv.URL}
	if err := v.Verify(context.Background(), cfg, &CaptchaToken{Token: "t"}, ""); err == nil {
		t.Fatal("expected failure on non-200 status")
	}
}

// TestCaptchaVerify_Unreachable 鏈嶅姟鍟嗕笉鍙揪 鈫?ErrCaptchaUnreachable锛坒ail-loud锛夈€?
func TestCaptchaVerify_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 绔嬪嵆鍏抽棴 鈫?杩炴帴鎷掔粷

	v := newCaptchaVerifier()
	cfg := &CaptchaConfig{Provider: CaptchaHCaptcha, Secret: "s", VerifyURL: srv.URL}
	err := v.Verify(context.Background(), cfg, &CaptchaToken{Token: "t"}, "")
	if err == nil || !strings.Contains(err.Error(), "captcha provider unreachable") {
		t.Fatalf("expected ErrCaptchaUnreachable, got %v", err)
	}
}

func TestCaptchaVerify_Validation(t *testing.T) {
	v := newCaptchaVerifier()
	ctx := context.Background()

	if err := v.Verify(ctx, nil, &CaptchaToken{Token: "t"}, ""); err == nil {
		t.Fatal("expected error for nil config")
	}
	if err := v.Verify(ctx, &CaptchaConfig{Provider: "nope"}, &CaptchaToken{Token: "t"}, ""); err == nil {
		t.Fatal("expected error for unknown provider")
	}
	// 绌?token 鐩存帴澶辫触
	if err := v.Verify(ctx, &CaptchaConfig{Provider: CaptchaTurnstile, Secret: "s"}, &CaptchaToken{}, ""); err != ErrCaptchaFailed {
		t.Fatalf("expected ErrCaptchaFailed, got %v", err)
	}
}
