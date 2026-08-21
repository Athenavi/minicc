package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
)

func TestSmsProviderRegistry(t *testing.T) {
	known := SmsKnownProviders()
	if len(known) != 3 {
		t.Fatalf("expected 3 sms providers, got %d", len(known))
	}
	for _, p := range []string{SmsAliyun, SmsTencent, SmsCustom} {
		if !IsKnownSmsProvider(p) {
			t.Errorf("expected %q to be known", p)
		}
	}
	if IsKnownSmsProvider("twilio") {
		t.Error("twilio should not be known")
	}
}

func TestValidateSmsPhone(t *testing.T) {
	valid := []string{"13800138000", "+8613800138000", "+85212345678", "8613800138000", "12345"}
	for _, p := range valid {
		if !ValidateSmsPhone(p) {
			t.Errorf("expected %q valid", p)
		}
	}
	invalid := []string{"", "   ", "1234", "12345678901234567890123", "abc12345", "138-0013-8000", "+86 138"}
	for _, p := range invalid {
		if ValidateSmsPhone(p) {
			t.Errorf("expected %q invalid", p)
		}
	}
}

func TestGenerateSmsCode(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		code, err := GenerateSmsCode(6)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("expected 6 digits, got %q", code)
		}
		for _, c := range code {
			if c < '0' || c > '9' {
				t.Fatalf("non-digit code %q", code)
			}
		}
		seen[code] = true
	}
	if len(seen) < 15 {
		t.Fatalf("expected near-unique codes, got %d distinct in 20 draws", len(seen))
	}
	if _, err := GenerateSmsCode(3); err == nil {
		t.Error("expected length error for 3")
	}
}

// ── custom 契约 ─────────────────────────────────────────

func TestHTTPSmsSender_Custom(t *testing.T) {
	sender := NewHTTPSmsSender()
	cfg := &SmsConfig{Provider: SmsCustom, AccessKeyID: "k1", AccessKeySecret: "s1", SignName: "MiniCC", TemplateID: "T1"}

	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer srv.Close()
	cfg.Endpoint = srv.URL
	if err := sender.Send(context.Background(), cfg, "13800138000", "123456"); err != nil {
		t.Fatalf("custom send: %v", err)
	}
	if gotBody["phone"] != "13800138000" || gotBody["code"] != "123456" ||
		gotBody["sign_name"] != "MiniCC" || gotBody["template_id"] != "T1" {
		t.Fatalf("unexpected custom payload: %v", gotBody)
	}

	// success=false → 拒绝
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "quota exceeded"})
	}))
	defer srv2.Close()
	cfg.Endpoint = srv2.URL
	err := sender.Send(context.Background(), cfg, "13800138000", "123456")
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("expected send-failed error, got %v", err)
	}

	// 服务商不可达
	cfg.Endpoint = "http://127.0.0.1:1/send"
	if err := sender.Send(context.Background(), cfg, "13800138000", "123456"); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("expected unreachable error, got %v", err)
	}

	// custom 缺端点
	if err := sender.Send(context.Background(), &SmsConfig{Provider: SmsCustom}, "13800138000", "123456"); err == nil {
		t.Fatal("expected endpoint-required error")
	}
}

// ── 阿里云 POP V1 签名（服务端复算验证）──────────────────

func TestHTTPSmsSender_AliyunSignature(t *testing.T) {
	const secret = "aliyun-secret"
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotForm = r.PostForm
		// 复算签名（独立于实现，按官方文档流程）
		sig := gotForm.Get("Signature")
		gotForm.Del("Signature")
		keys := make([]string, 0, len(gotForm))
		for k := range gotForm {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, aliyunPercentEncode(k)+"="+aliyunPercentEncode(gotForm.Get(k)))
		}
		stringToSign := "POST&" + aliyunPercentEncode("/") + "&" + aliyunPercentEncode(strings.Join(pairs, "&"))
		if sig != aliyunSign(secret, stringToSign) {
			t.Errorf("signature mismatch: got %q", sig)
		}
		json.NewEncoder(w).Encode(map[string]string{"Code": "OK", "BizId": "b1", "RequestId": "r1"})
	}))
	defer srv.Close()

	sender := NewHTTPSmsSender()
	cfg := &SmsConfig{
		Provider: SmsAliyun, AccessKeyID: "LTAI-key", AccessKeySecret: secret,
		SignName: "MiniCC", TemplateID: "SMS_123", Endpoint: srv.URL,
	}
	if err := sender.Send(context.Background(), cfg, "13800138000", "654321"); err != nil {
		t.Fatalf("aliyun send: %v", err)
	}
	if gotForm.Get("PhoneNumbers") != "13800138000" || gotForm.Get("TemplateCode") != "SMS_123" ||
		gotForm.Get("SignName") != "MiniCC" || gotForm.Get("Action") != "SendSms" {
		t.Fatalf("unexpected aliyun params: %v", gotForm)
	}
	var tp map[string]string
	json.Unmarshal([]byte(gotForm.Get("TemplateParam")), &tp)
	if tp["code"] != "654321" {
		t.Fatalf("expected TemplateParam code=654321, got %v", tp)
	}
}

func TestHTTPSmsSender_AliyunRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"Code": "isv.BUSINESS_LIMIT_CONTROL", "Message": "触发分钟级流控"})
	}))
	defer srv.Close()
	sender := NewHTTPSmsSender()
	cfg := &SmsConfig{Provider: SmsAliyun, AccessKeyID: "k", AccessKeySecret: "s",
		SignName: "x", TemplateID: "T", Endpoint: srv.URL}
	err := sender.Send(context.Background(), cfg, "13800138000", "123456")
	if err == nil || !strings.Contains(err.Error(), "isv.BUSINESS_LIMIT_CONTROL") {
		t.Fatalf("expected aliyun rejection error, got %v", err)
	}
}

// ── 腾讯云 TC3 签名（服务端复算验证）────────────────────

func TestHTTPSmsSender_Tencent(t *testing.T) {
	const (
		secretID  = "AKIDtest"
		secretKey = "tencent-secret"
	)
	var gotPayload map[string]any
	var gotAuth, gotAction string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAction = r.Header.Get("X-TC-Action")
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		json.Unmarshal(body, &gotPayload)
		json.NewEncoder(w).Encode(map[string]any{
			"Response": map[string]any{
				"SendStatusSet": []map[string]string{{"Code": "Ok", "Message": "send success"}},
				"RequestId":     "r1",
			},
		})
	}))
	defer srv.Close()

	sender := NewHTTPSmsSender()
	cfg := &SmsConfig{
		Provider: SmsTencent, AccessKeyID: secretID, AccessKeySecret: secretKey,
		SignName: "MiniCC", TemplateID: "T-100", Endpoint: srv.URL,
	}
	if err := sender.Send(context.Background(), cfg, "+8613800138000", "888888"); err != nil {
		t.Fatalf("tencent send: %v", err)
	}
	if gotAction != "SendSms" {
		t.Errorf("expected X-TC-Action=SendSms, got %q", gotAction)
	}
	if !strings.HasPrefix(gotAuth, "TC3-HMAC-SHA256 Credential="+secretID+"/") {
		t.Errorf("unexpected Authorization: %q", gotAuth)
	}
	// "+" 前缀剥离 → E.164 裸号
	phones := gotPayload["PhoneNumberSet"].([]any)
	if phones[0] != "8613800138000" {
		t.Errorf("expected E.164 phone, got %v", phones)
	}
	if gotPayload["TemplateId"] != "T-100" || gotPayload["SignName"] != "MiniCC" {
		t.Fatalf("unexpected payload: %v", gotPayload)
	}
	params := gotPayload["TemplateParamSet"].([]any)
	if params[0] != "888888" {
		t.Errorf("expected code param, got %v", params)
	}
}

func TestHTTPSmsSender_TencentError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"Response": map[string]any{
				"Error": map[string]string{"Code": "AuthFailure.SignatureFailure", "Message": "签名错误"},
			},
		})
	}))
	defer srv.Close()
	sender := NewHTTPSmsSender()
	cfg := &SmsConfig{Provider: SmsTencent, AccessKeyID: "k", AccessKeySecret: "s",
		SignName: "x", TemplateID: "T", Endpoint: srv.URL}
	err := sender.Send(context.Background(), cfg, "8613800138000", "123456")
	if err == nil || !strings.Contains(err.Error(), "AuthFailure.SignatureFailure") {
		t.Fatalf("expected tencent error, got %v", err)
	}
}

func TestHTTPSmsSender_Guards(t *testing.T) {
	sender := NewHTTPSmsSender()
	if err := sender.Send(context.Background(), nil, "13800138000", "1"); err == nil {
		t.Error("expected nil config error")
	}
	if err := sender.Send(context.Background(), &SmsConfig{Provider: "twilio"}, "13800138000", "1"); err == nil {
		t.Error("expected unknown provider error")
	}
	if err := sender.Send(context.Background(), &SmsConfig{Provider: SmsCustom, Endpoint: "http://x"}, "", "1"); err == nil {
		t.Error("expected empty phone error")
	}
}
