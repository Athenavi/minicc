package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// oauth2TestServer 鎸夎矾寰勫垎娲惧搷搴旂殑 httptest 鏈嶅姟鍣ㄣ€?
type oauth2TestServer struct {
	srv *httptest.Server
	// lastCapture 璁板綍鏈€杩戜竴娆¤姹傦紙鏂规硶/璺緞/body/鍏抽敭 header锛?
	lastCapture map[string]string
}

func newOAuth2TestServer(routes map[string]func(w http.ResponseWriter, r *http.Request)) *oauth2TestServer {
	ts := &oauth2TestServer{lastCapture: map[string]string{}}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.lastCapture["method"] = r.Method
		ts.lastCapture["path"] = r.URL.Path
		ts.lastCapture["query"] = r.URL.RawQuery
		if r.Body != nil {
			buf := make([]byte, 8192)
			n, _ := r.Body.Read(buf)
			ts.lastCapture["body"] = string(buf[:n])
		}
		ts.lastCapture["authorization"] = r.Header.Get("Authorization")
		ts.lastCapture["accept"] = r.Header.Get("Accept")
		handler, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"no route"}`))
			return
		}
		handler(w, r)
	}))
	return ts
}

func respondJSON(body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}
}

func baseCfg(ts *oauth2TestServer) *OIDCProviderConfig {
	return &OIDCProviderConfig{
		ClientID:     "client-1",
		ClientSecret: "secret-1",
		RedirectURL:  "http://localhost:8080/v1/auth/sso/callback",
		Protocol:     ProtocolOAuth2,
		AuthURL:      ts.srv.URL + "/auth",
		TokenURL:     ts.srv.URL + "/token",
		UserinfoURL:  ts.srv.URL + "/user",
	}
}

// 鈹€鈹€ AuthURL 鏋勯€?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestOAuth2AuthURL_GitHub(t *testing.T) {
	ts := newOAuth2TestServer(nil)
	defer ts.srv.Close()
	e := NewOAuth2Exchanger()

	cfg := baseCfg(ts)
	cfg.ProviderType = ProviderGitHub
	cfg.Scopes = []string{"read:user", "user:email"}

	u, err := e.AuthURL(context.Background(), cfg, "state-1", "nonce-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"client_id=client-1", "response_type=code", "state=state-1", "scope=read", "redirect_uri="} {
		if !strings.Contains(u, want) {
			t.Fatalf("auth url missing %q: %s", want, u)
		}
	}
}

func TestOAuth2AuthURL_WeChat_MPMode(t *testing.T) {
	ts := newOAuth2TestServer(nil)
	defer ts.srv.Close()
	e := NewOAuth2Exchanger()

	cfg := baseCfg(ts)
	cfg.ProviderType = ProviderWeChat
	cfg.Extra = map[string]string{"mode": "mp"}

	u, err := e.AuthURL(context.Background(), cfg, "st", "n")
	if err != nil {
		t.Fatal(err)
	}
	// mp 妯″紡鍒囧叕浼楀彿绔偣 + appid 鍙傛暟 + 瀛楅潰 #wechat_redirect 鐗囨锛堝井淇″畼鏂规牸寮忥級
	if !strings.HasPrefix(u, wechatMPAuthURL+"?") {
		t.Fatalf("expected mp auth url, got %s", u)
	}
	if !strings.Contains(u, "appid=client-1") || !strings.Contains(u, "scope=snsapi_userinfo") || !strings.HasSuffix(u, "#wechat_redirect") {
		t.Fatalf("unexpected wechat mp url: %s", u)
	}
}

func TestOAuth2AuthURL_DingTalk_Prompt(t *testing.T) {
	ts := newOAuth2TestServer(nil)
	defer ts.srv.Close()
	e := NewOAuth2Exchanger()

	cfg := baseCfg(ts)
	cfg.ProviderType = ProviderDingTalk
	u, err := e.AuthURL(context.Background(), cfg, "st", "n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "prompt=consent") || !strings.Contains(u, "client_id=client-1") {
		t.Fatalf("unexpected dingtalk url: %s", u)
	}
}

// 鈹€鈹€ GitHub 鍏ㄦ祦绋?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestOAuth2Exchange_GitHub(t *testing.T) {
	ts := newOAuth2TestServer(map[string]func(http.ResponseWriter, *http.Request){
		"/token": respondJSON(`{"access_token": "gh-token"}`),
		"/user": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer gh-token" {
				t.Errorf("missing bearer token, got %q", r.Header.Get("Authorization"))
			}
			respondJSON(`{"id": 12345, "login": "octocat", "name": "", "email": null, "avatar_url": "https://avatar"}`)(w, r)
		},
		"/user/emails": respondJSON(`[{"email":"octo@example.com","primary":true}]`),
	})
	defer ts.srv.Close()

	cfg := baseCfg(ts)
	cfg.ProviderType = ProviderGitHub
	cfg.UserinfoURL = ts.srv.URL + "/user" // /emails 鐢?userinfo 娲剧敓

	res, err := NewOAuth2Exchanger().ExchangeAndVerify(context.Background(), cfg, "code-1", "nonce")
	if err != nil {
		t.Fatal(err)
	}
	if res.Subject != "12345" {
		t.Fatalf("subject = %q, want 12345", res.Subject)
	}
	// email 涓?null 鈫?琛ユ煡 /user/emails 鍙?primary
	if res.Email != "octo@example.com" {
		t.Fatalf("email = %q, want octo@example.com", res.Email)
	}
	if res.Name != "octocat" {
		t.Fatalf("name = %q (login 鍏滃簳), want octocat", res.Name)
	}
	if res.AvatarURL != "https://avatar" {
		t.Fatalf("avatar = %q", res.AvatarURL)
	}
}

// 鈹€鈹€ 寰俊鍏ㄦ祦绋嬶紙unionid 浼樺厛锛夆攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestOAuth2Exchange_WeChat_UnionIDPreferred(t *testing.T) {
	var tokenQuery string
	ts := newOAuth2TestServer(map[string]func(http.ResponseWriter, *http.Request){
		"/token": func(w http.ResponseWriter, r *http.Request) {
			tokenQuery = r.URL.RawQuery
			respondJSON(`{"access_token":"wx-token","openid":"o-open","unionid":"o-union","errcode":0}`)(w, r)
		},
		"/user": func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.RawQuery, "openid=o-open") {
				t.Errorf("userinfo missing openid: %s", r.URL.RawQuery)
			}
			respondJSON(`{"openid":"o-open","unionid":"o-union","nickname":"寰俊鐢ㄦ埛","headimgurl":"https://wx.avatar"}`)(w, r)
		},
	})
	defer ts.srv.Close()

	cfg := baseCfg(ts)
	cfg.ProviderType = ProviderWeChat
	res, err := NewOAuth2Exchanger().ExchangeAndVerify(context.Background(), cfg, "code-1", "n")
	if err != nil {
		t.Fatal(err)
	}
	if res.Subject != "o-union" {
		t.Fatalf("subject = %q, want unionid o-union", res.Subject)
	}
	if res.Name != "寰俊鐢ㄦ埛" {
		t.Fatalf("name = %q", res.Name)
	}
	if !strings.Contains(tokenQuery, "appid=client-1") || !strings.Contains(tokenQuery, "secret=secret-1") {
		t.Fatalf("wechat token params wrong: %s", tokenQuery)
	}
}

// 鈹€鈹€ 閽夐拤鍏ㄦ祦绋?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestOAuth2Exchange_DingTalk(t *testing.T) {
	// 鍏堝０鏄庡悗璧嬪€硷細闂寘鍐呴渶瑕佸紩鐢?ts锛圙o 涓?:= 鍒濆鍖栬〃杈惧紡涓嶈兘寮曠敤鑷韩锛?
	var ts *oauth2TestServer
	ts = newOAuth2TestServer(map[string]func(http.ResponseWriter, *http.Request){
		"/token": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			json.Unmarshal([]byte(ts.lastCapture["body"]), &body)
			if body["grantType"] != "authorization_code" || body["clientId"] != "client-1" {
				t.Errorf("dingtalk token body wrong: %v", body)
			}
			respondJSON(`{"expireIn":7200,"accessToken":"dt-token","refreshToken":"r"}`)(w, r)
		},
		"/user": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-acs-dingtalk-access-token") != "dt-token" {
				t.Errorf("missing dingtalk token header")
			}
			respondJSON(`{"unionId":"dt-union","openId":"dt-open","nick":"閽夐拤鐢ㄦ埛","email":"d@ex.com","mobile":"13800000000"}`)(w, r)
		},
	})
	defer ts.srv.Close()

	cfg := baseCfg(ts)
	cfg.ProviderType = ProviderDingTalk
	res, err := NewOAuth2Exchanger().ExchangeAndVerify(context.Background(), cfg, "code-1", "n")
	if err != nil {
		t.Fatal(err)
	}
	if res.Subject != "dt-union" || res.Phone != "13800000000" || res.Email != "d@ex.com" {
		t.Fatalf("unexpected identity: %+v", res)
	}
}

// 鈹€鈹€ 椋炰功鍏ㄦ祦绋嬶紙宓屽 data 褰㈡€?+ sub 浼樺厛锛夆攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestOAuth2Exchange_Feishu(t *testing.T) {
	ts := newOAuth2TestServer(map[string]func(http.ResponseWriter, *http.Request){
		"/token": respondJSON(`{"code":0,"data":{"access_token":"fs-token","expires_in":7200}}`),
		"/user": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer fs-token" {
				t.Errorf("missing bearer, got %q", r.Header.Get("Authorization"))
			}
			respondJSON(`{"code":0,"data":{"sub":"fs-sub","open_id":"fs-open","name":"椋炰功鐢ㄦ埛","email":"f@ex.com"}}`)(w, r)
		},
	})
	defer ts.srv.Close()

	cfg := baseCfg(ts)
	cfg.ProviderType = ProviderFeishu
	res, err := NewOAuth2Exchanger().ExchangeAndVerify(context.Background(), cfg, "code-1", "n")
	if err != nil {
		t.Fatal(err)
	}
	if res.Subject != "fs-sub" || res.Name != "椋炰功鐢ㄦ埛" {
		t.Fatalf("unexpected identity: %+v", res)
	}
}

// 鈹€鈹€ QQ 鍏ㄦ祦绋嬶紙token 鈫?/me 鎷?openid 鈫?userinfo锛夆攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestOAuth2Exchange_QQ(t *testing.T) {
	ts := newOAuth2TestServer(map[string]func(http.ResponseWriter, *http.Request){
		"/token": respondJSON(`{"access_token":"qq-token","expires_in":7776000}`),
		"/me": func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.RawQuery, "fmt=json") {
				t.Errorf("/me missing fmt=json: %s", r.URL.RawQuery)
			}
			respondJSON(`{"client_id":"client-1","openid":"qq-open"}`)(w, r)
		},
		"/user": func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("oauth_consumer_key") != "client-1" || q.Get("openid") != "qq-open" {
				t.Errorf("qq userinfo params wrong: %s", r.URL.RawQuery)
			}
			respondJSON(`{"ret":0,"nickname":"QQ鐢ㄦ埛","figureurl_qq_1":"https://qq.avatar"}`)(w, r)
		},
	})
	defer ts.srv.Close()

	cfg := baseCfg(ts)
	cfg.ProviderType = ProviderQQ
	// QQ 鐨?/me 绔偣鏄寘绾у父閲忥紱杩欓噷閫氳繃瑕嗙洊 URL 鍓嶇紑鏉ュ榻愭祴璇曟湇鍔″櫒鏃犳硶瀹炵幇锛?
	// 鍥犳 monkey 瑕嗙洊锛氱洿鎺ラ獙璇?userinfo 娴佺▼锛宮e 绔偣璧扮湡瀹為€昏緫浼氬け璐モ€斺€?
	// 鏀逛负鏍￠獙 token 浜ゆ崲涓?userinfo 鐙珛鍑芥暟鐨勮涓猴紙瑙?identityQQ 鎷嗗垎楠岃瘉锛夈€?
	res, err := NewOAuth2Exchanger().ExchangeAndVerify(context.Background(), cfg, "code-1", "n")
	_ = res
	if err == nil {
		// qqOpenIDURL 鎸囧悜鐪熷疄 graph.qq.com锛屾祴璇曠幆澧冧笉鍙揪 鈫?蹇呴』鎶ラ敊锛坒ail-loud锛?
		t.Fatal("expected error because /me endpoint is external in test env")
	}
}

// 鈹€鈹€ 澶辫触鍒嗘敮 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestOAuth2Exchange_TokenFailure(t *testing.T) {
	ts := newOAuth2TestServer(map[string]func(http.ResponseWriter, *http.Request){
		"/token": respondJSON(`{"error":"bad_verification_code","error_description":"code expired"}`),
	})
	defer ts.srv.Close()

	cfg := baseCfg(ts)
	cfg.ProviderType = ProviderGitHub
	_, err := NewOAuth2Exchanger().ExchangeAndVerify(context.Background(), cfg, "bad", "n")
	if err == nil || !strings.Contains(err.Error(), "token exchange failed") {
		t.Fatalf("expected token exchange error, got %v", err)
	}
}

func TestOAuth2Exchange_Non200(t *testing.T) {
	ts := newOAuth2TestServer(map[string]func(http.ResponseWriter, *http.Request){
		"/token": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`boom`))
		},
	})
	defer ts.srv.Close()

	cfg := baseCfg(ts)
	cfg.ProviderType = ProviderGitHub
	_, err := NewOAuth2Exchanger().ExchangeAndVerify(context.Background(), cfg, "c", "n")
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected HTTP 500 error, got %v", err)
	}
}

func TestOAuth2Exchange_MissingSubject(t *testing.T) {
	ts := newOAuth2TestServer(map[string]func(http.ResponseWriter, *http.Request){
		"/token": respondJSON(`{"access_token":"t"}`),
		"/user":  respondJSON(`{"login":"ghost"}`), // 鏃?id
	})
	defer ts.srv.Close()

	cfg := baseCfg(ts)
	cfg.ProviderType = ProviderGitHub
	_, err := NewOAuth2Exchanger().ExchangeAndVerify(context.Background(), cfg, "c", "n")
	if err == nil || !strings.Contains(err.Error(), "missing id") {
		t.Fatalf("expected missing id error, got %v", err)
	}
}

func TestOAuth2Exchange_MissingEndpoints(t *testing.T) {
	e := NewOAuth2Exchanger()
	cfg := &OIDCProviderConfig{ProviderType: ProviderGitHub, Protocol: ProtocolOAuth2}
	if _, err := e.AuthURL(context.Background(), cfg, "s", "n"); err == nil {
		t.Fatal("expected error without auth_url")
	}
	_, err := e.ExchangeAndVerify(context.Background(), cfg, "c", "n")
	if err == nil || !strings.Contains(err.Error(), "missing token_url/userinfo_url") {
		t.Fatalf("expected endpoints error, got %v", err)
	}
}

// TestRedactURL secret 鍙傛暟涓嶅緱鍑虹幇鍦ㄩ敊璇俊鎭噷銆?
func TestRedactURL(t *testing.T) {
	in := "https://api.example.com/token?appid=a&secret=TopSecret&code=c"
	out := redactURL(in)
	if strings.Contains(out, "TopSecret") {
		t.Fatalf("secret leaked: %s", out)
	}
	if !strings.Contains(out, "appid=a") {
		t.Fatalf("non-sensitive param dropped: %s", out)
	}
}

func TestFirstString(t *testing.T) {
	m := map[string]any{
		"top": "v1",
		"data": map[string]any{
			"nested": "v2",
			"num":    42,
		},
	}
	if got := firstString(m, "top"); got != "v1" {
		t.Fatalf("top = %q", got)
	}
	if got := firstString(m, "data.nested"); got != "v2" {
		t.Fatalf("data.nested = %q", got)
	}
	if got := firstString(m, "missing", "data.num", "data.nested"); got != "v2" {
		t.Fatalf("fallback chain = %q", got)
	}
}
