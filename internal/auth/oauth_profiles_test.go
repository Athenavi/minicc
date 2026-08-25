package auth

import (
	"encoding/base64"
	"strings"
	"testing"
)

// signLegacyState 鏋勯€犳棫鐗堟牸寮?state锛坧ayload 鏃?m/u 瀛楁锛夛紝
// 妯℃嫙鍘嗗彶绛惧彂浠ょ墝浠ラ獙璇佸悜鍚庡吋瀹广€?
func signLegacyState(c *StateCodec, payloadJSON string) string {
	body := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return body + "." + c.sign(body)
}

func TestGetProviderProfile_AllBuiltins(t *testing.T) {
	for _, pt := range KnownProviderTypes() {
		p, ok := GetProviderProfile(pt)
		if !ok {
			t.Fatalf("profile %q missing", pt)
		}
		if p.Type != pt {
			t.Fatalf("profile type mismatch: %q vs %q", p.Type, pt)
		}
		if !ValidProtocol(p.Protocol) {
			t.Fatalf("profile %q invalid protocol %q", pt, p.Protocol)
		}
		if p.Protocol == ProtocolOAuth2 && (p.AuthURL == "" || p.TokenURL == "" || p.UserinfoURL == "") {
			t.Fatalf("oauth2 profile %q missing endpoints", pt)
		}
	}
}

func TestGetProviderProfile_Unknown(t *testing.T) {
	if _, ok := GetProviderProfile("myspace"); ok {
		t.Fatal("unknown provider must not resolve")
	}
	if IsKnownProviderType("myspace") {
		t.Fatal("IsKnownProviderType must reject unknown")
	}
}

func TestResolveEndpoints_TemplateFillAndOverride(t *testing.T) {
	// 妯℃澘濉厖
	issuer, authURL, tokenURL, userinfoURL := ResolveEndpoints(ProviderGitHub, "", "", "", "")
	if issuer != "" || authURL != "https://github.com/login/oauth/authorize" ||
		tokenURL != "https://github.com/login/oauth/access_token" ||
		userinfoURL != "https://api.github.com/user" {
		t.Fatalf("template fill wrong: %q %q %q %q", issuer, authURL, tokenURL, userinfoURL)
	}
	// 鏄惧紡瑕嗙洊浼樺厛锛堜緥濡?GitHub Enterprise锛?
	_, authURL, _, _ = ResolveEndpoints(ProviderGitHub, "", "https://ghe.corp/login/oauth/authorize", "", "")
	if authURL != "https://ghe.corp/login/oauth/authorize" {
		t.Fatalf("override lost: %q", authURL)
	}
	// 鏈煡绫诲瀷鍘熸牱杩斿洖
	_, authURL, _, _ = ResolveEndpoints("nope", "iss", "au", "tu", "uu")
	if authURL != "au" {
		t.Fatalf("unknown type must pass through, got %q", authURL)
	}
}

func TestDefaultScopes(t *testing.T) {
	if s := DefaultScopes(ProviderWeChat); len(s) == 0 || s[0] != "snsapi_login" {
		t.Fatalf("wechat scopes = %v", s)
	}
	if s := DefaultScopes("nope"); s[0] != "openid" {
		t.Fatalf("default scopes = %v", s)
	}
}

func TestBuildAuthURL_Params(t *testing.T) {
	u := buildAuthURL("https://idp.example.com/authorize", "cid", "http://cb", []string{"a", "b"}, "st", nil)
	for _, want := range []string{"client_id=cid", "redirect_uri=http%3A%2F%2Fcb", "response_type=code", "scope=a+b", "state=st"} {
		if !strings.Contains(u, want) {
			t.Fatalf("missing %q in %s", want, u)
		}
	}
	// 宸插甫 query 鐨?base 鐢?& 鎷兼帴
	u2 := buildAuthURL("https://idp.example.com/authorize?x=1", "cid", "http://cb", nil, "st", nil)
	if !strings.Contains(u2, "authorize?x=1&") {
		t.Fatalf("expected & join, got %s", u2)
	}
}

func TestStateCodecIssueMode(t *testing.T) {
	codec := NewStateCodec([]byte("k"), 0)

	// bind 妯″紡蹇呴』甯?uid
	if _, err := codec.IssueMode("p", "n", StateModeBind, ""); err == nil {
		t.Fatal("expected error for bind without uid")
	}
	// 鏈煡妯″紡
	if _, err := codec.IssueMode("p", "n", "other", ""); err == nil {
		t.Fatal("expected error for unknown mode")
	}

	// bind 浠ょ墝寰€杩?
	state, err := codec.IssueMode("prov-1", "n-1", StateModeBind, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := codec.Verify(state)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Mode != StateModeBind || payload.UID != "user-1" {
		t.Fatalf("payload = %+v", payload)
	}

	// login 浠ょ墝涓嶆惡甯?uid锛堥槻绡℃敼鍗囩骇锛?
	state, err = codec.Issue("prov-1", "n-1")
	if err != nil {
		t.Fatal(err)
	}
	payload, err = codec.Verify(state)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Mode != StateModeLogin || payload.UID != "" {
		t.Fatalf("login payload = %+v", payload)
	}
}

// TestStateCodecLegacyFormat 鏃ф牸寮?state锛堟棤 mode 瀛楁锛夋寜 login 澶勭悊銆?
func TestStateCodecLegacyFormat(t *testing.T) {
	// 鎵嬪伐鏋勯€犳棫 payload锛堟棤 m/u 瀛楁锛?
	legacy := `{"p":"prov-1","n":"n-1","e":9999999999}`
	codec := NewStateCodec([]byte("k"), 0)
	state := signLegacyState(codec, legacy)
	payload, err := codec.Verify(state)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Mode != "" || payload.UID != "" {
		t.Fatalf("legacy payload = %+v", payload)
	}
}
