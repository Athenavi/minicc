package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 鈹€鈹€ OAuth2 Exchanger锛圙itHub / 寰俊 / 閽夐拤 / 椋炰功 / QQ锛夆攢鈹€
//
// 瀹炵幇 OIDCExchanger 鍚屼竴鎺ュ彛锛歋SOHandler 鎸?provider.protocol 鍒嗘淳鍒?
// OIDC锛坓o-oidc 鍙戠幇锛夋垨鏈疄鐜帮紙鏄惧紡绔偣 + provider 鐗规湁韬唤鎶藉彇锛夈€?
// 绔偣鍙鏁版嵁搴撹鐩栵紙cfg.AuthURL/TokenURL/UserinfoURL 闈炵┖浼樺厛锛夛紝
// 鍥犳鍗曟祴鍙敤 httptest 鏈嶅姟鍣ㄦ敞鍏ュ叏閮ㄧ鐐癸紝鏃犻渶鐪熷疄鍑嵁銆?

// qqOpenIDURL QQ 鑾峰彇 openid 鐨勫浐瀹氱鐐广€?
const qqOpenIDURL = "https://graph.qq.com/oauth2.0/me"

// OAuth2Exchanger 鏄函 OAuth2 鍗忚鐨勬巿鏉冪爜浜ゆ崲瀹炵幇銆?
type OAuth2Exchanger struct {
	client *http.Client
}

// NewOAuth2Exchanger 鏋勯€犱氦鎹㈠櫒锛?5s 瓒呮椂锛夈€?
func NewOAuth2Exchanger() *OAuth2Exchanger {
	return &OAuth2Exchanger{client: &http.Client{Timeout: 15 * time.Second}}
}

// AuthURL 鏋勯€犳巿鏉冮〉璺宠浆 URL銆?
func (e *OAuth2Exchanger) AuthURL(ctx context.Context, p *OIDCProviderConfig, state, nonce string) (string, error) {
	if p.AuthURL == "" {
		return "", fmt.Errorf("oauth2: provider %q has no auth_url", p.ProviderType)
	}
	base := p.AuthURL
	scopes := p.Scopes
	extra := url.Values{}

	switch p.ProviderType {
	case ProviderWeChat:
		// 寰俊鐢?appid 鑰岄潪 client_id锛涙壂鐮佹ā寮忚拷鍔?#wechat_redirect
		base = wechatEffectiveAuthURL(p.AuthURL, p.Extra)
		if p.Extra["mode"] == "mp" {
			scopes = []string{"snsapi_userinfo"}
		}
		extra.Set("appid", p.ClientID)
	case ProviderDingTalk:
		// 閽夐拤鐢?client_id + prompt=consent
		extra.Set("client_id", p.ClientID)
		extra.Set("prompt", "consent")
	default:
		// github/feishu/qq 鐢ㄦ爣鍑嗗弬鏁?
	}
	return buildAuthURLSpecial(base, p.ClientID, p.RedirectURL, scopes, state, extra, p.ProviderType), nil
}

// buildAuthURLSpecial 鍦ㄩ€氱敤鏋勯€犲熀纭€涓婂鐞嗗悇 provider 鍙傛暟鍚嶅樊寮傘€?
func buildAuthURLSpecial(base, clientID, redirectURL string, scopes []string, state string, extra url.Values, providerType string) string {
	q := url.Values{}
	if extra.Has("appid") {
		q.Set("appid", clientID) // 寰俊
	} else if extra.Has("client_id") {
		q.Set("client_id", clientID) // 閽夐拤锛堝悓鍚嶏紝璧伴€氱敤閿級
	} else {
		q.Set("client_id", clientID) // github/feishu/qq 鏍囧噯
	}
	q.Set("redirect_uri", redirectURL)
	q.Set("response_type", "code")
	if len(scopes) > 0 {
		q.Set("scope", strings.Join(scopes, " "))
	}
	q.Set("state", state)
	for k := range extra {
		if k == "appid" || k == "client_id" {
			continue
		}
		q.Set(k, extra.Get(k))
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	u := base + sep + q.Encode()
	if providerType == ProviderWeChat {
		u += "#wechat_redirect"
	}
	return u
}

// ExchangeAndVerify 鎺堟潈鐮佹崲 access_token 鈫?鎷夊彇鐢ㄦ埛韬唤銆?
// OAuth2 鍗忚鏃?id_token/nonce锛宔xpectedNonce 鐢?state HMAC 鎵挎媴闃查噸鏀俱€?
func (e *OAuth2Exchanger) ExchangeAndVerify(ctx context.Context, p *OIDCProviderConfig, code, expectedNonce string) (*IDTokenResult, error) {
	if p.TokenURL == "" || p.UserinfoURL == "" {
		return nil, fmt.Errorf("oauth2: provider %q missing token_url/userinfo_url", p.ProviderType)
	}
	accessToken, err := e.exchangeToken(ctx, p, code)
	if err != nil {
		return nil, err
	}
	return e.fetchIdentity(ctx, p, accessToken)
}

// exchangeToken 鍚?provider 鎺堟潈鐮?鈫?access_token銆?
func (e *OAuth2Exchanger) exchangeToken(ctx context.Context, p *OIDCProviderConfig, code string) (string, error) {
	switch p.ProviderType {
	case ProviderGitHub:
		return e.exchangeGitHub(ctx, p, code)
	case ProviderWeChat:
		return e.exchangeWeChat(ctx, p, code)
	case ProviderDingTalk:
		return e.exchangeDingTalk(ctx, p, code)
	case ProviderFeishu:
		return e.exchangeFeishu(ctx, p, code)
	case ProviderQQ:
		return e.exchangeQQ(ctx, p, code)
	default:
		return "", fmt.Errorf("oauth2: no token exchange for provider %q", p.ProviderType)
	}
}

// exchangeGitHub: POST form锛坈lient_id/secret/code/redirect_uri锛? Accept: JSON銆?
func (e *OAuth2Exchanger) exchangeGitHub(ctx context.Context, p *OIDCProviderConfig, code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", p.RedirectURL)
	data, err := e.do(ctx, http.MethodPost, p.TokenURL, "application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()), map[string]string{"Accept": "application/json"})
	if err != nil {
		return "", err
	}
	var resp struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("oauth2 github: malformed token response: %w", err)
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("oauth2 github: token exchange failed: %s %s", resp.Error, resp.ErrorDescription)
	}
	return resp.AccessToken, nil
}

// exchangeWeChat: GET token_url?appid&secret&code&grant_type銆?
func (e *OAuth2Exchanger) exchangeWeChat(ctx context.Context, p *OIDCProviderConfig, code string) (string, error) {
	q := url.Values{}
	q.Set("appid", p.ClientID)
	q.Set("secret", p.ClientSecret)
	q.Set("code", code)
	q.Set("grant_type", "authorization_code")
	sep := "?"
	if strings.Contains(p.TokenURL, "?") {
		sep = "&"
	}
	data, err := e.do(ctx, http.MethodGet, p.TokenURL+sep+q.Encode(), "", nil, nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		AccessToken string `json:"access_token"`
		OpenID      string `json:"openid"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("oauth2 wechat: malformed token response: %w", err)
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("oauth2 wechat: token exchange failed: errcode=%d errmsg=%s", resp.ErrCode, resp.ErrMsg)
	}
	return resp.AccessToken + "|" + resp.OpenID, nil // openid 闅?token 浼犻€?
}

// exchangeDingTalk: POST JSON {clientId, clientSecret, code, grantType}銆?
func (e *OAuth2Exchanger) exchangeDingTalk(ctx context.Context, p *OIDCProviderConfig, code string) (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"clientId":     p.ClientID,
		"clientSecret": p.ClientSecret,
		"code":         code,
		"grantType":    "authorization_code",
	})
	data, err := e.do(ctx, http.MethodPost, p.TokenURL, "application/json", strings.NewReader(string(payload)), nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		AccessToken string `json:"accessToken"`
		ErrCode     int    `json:"code"`
		ErrMsg      string `json:"message"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("oauth2 dingtalk: malformed token response: %w", err)
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("oauth2 dingtalk: token exchange failed: code=%d message=%s", resp.ErrCode, resp.ErrMsg)
	}
	return resp.AccessToken, nil
}

// exchangeFeishu: POST JSON {app_id, app_secret, code, grant_type}锛屽搷搴斿吋瀹归《灞?data 涓ょ褰㈡€併€?
func (e *OAuth2Exchanger) exchangeFeishu(ctx context.Context, p *OIDCProviderConfig, code string) (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"app_id":     p.ClientID,
		"app_secret": p.ClientSecret,
		"code":       code,
		"grant_type": "authorization_code",
	})
	data, err := e.do(ctx, http.MethodPost, p.TokenURL, "application/json", strings.NewReader(string(payload)), nil)
	if err != nil {
		return "", err
	}
	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("oauth2 feishu: malformed token response: %w", err)
	}
	token := firstString(resp, "access_token", "data.access_token")
	if token == "" {
		return "", fmt.Errorf("oauth2 feishu: token exchange failed: %s", snippet(data))
	}
	return token, nil
}

// exchangeQQ: GET token_url?client_id&client_secret&code&redirect_uri&fmt=json銆?
func (e *OAuth2Exchanger) exchangeQQ(ctx context.Context, p *OIDCProviderConfig, code string) (string, error) {
	q := url.Values{}
	q.Set("client_id", p.ClientID)
	q.Set("client_secret", p.ClientSecret)
	q.Set("code", code)
	q.Set("redirect_uri", p.RedirectURL)
	q.Set("fmt", "json")
	sep := "?"
	if strings.Contains(p.TokenURL, "?") {
		sep = "&"
	}
	data, err := e.do(ctx, http.MethodGet, p.TokenURL+sep+q.Encode(), "", nil, nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		AccessToken    string `json:"access_token"`
		Error          int    `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("oauth2 qq: malformed token response: %w", err)
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("oauth2 qq: token exchange failed: error=%d %s", resp.Error, resp.ErrorDescription)
	}
	return resp.AccessToken, nil
}

// fetchIdentity 鎸?provider 鎷夊彇 userinfo 骞舵娊鍙栬韩浠姐€?
func (e *OAuth2Exchanger) fetchIdentity(ctx context.Context, p *OIDCProviderConfig, accessToken string) (*IDTokenResult, error) {
	switch p.ProviderType {
	case ProviderGitHub:
		return e.identityGitHub(ctx, p, accessToken)
	case ProviderWeChat:
		return e.identityWeChat(ctx, p, accessToken)
	case ProviderDingTalk:
		return e.identityDingTalk(ctx, p, accessToken)
	case ProviderFeishu:
		return e.identityFeishu(ctx, p, accessToken)
	case ProviderQQ:
		return e.identityQQ(ctx, p, accessToken)
	default:
		return nil, fmt.Errorf("oauth2: no identity fetch for provider %q", p.ProviderType)
	}
}

// identityGitHub: GET /user锛圔earer锛夛紱email 缂哄け鏃惰ˉ鏌?/user/emails銆?
func (e *OAuth2Exchanger) identityGitHub(ctx context.Context, p *OIDCProviderConfig, accessToken string) (*IDTokenResult, error) {
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Accept":        "application/vnd.github+json",
	}
	data, err := e.do(ctx, http.MethodGet, p.UserinfoURL, "", nil, headers)
	if err != nil {
		return nil, err
	}
	var user struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("oauth2 github: malformed userinfo: %w", err)
	}
	if user.ID == 0 {
		return nil, errors.New("oauth2 github: userinfo missing id")
	}
	name := user.Name
	if name == "" {
		name = user.Login
	}
	email := user.Email
	if email == "" {
		// /user/emails 琛ユ煡锛堝叕寮€閭甯稿父涓?null锛?
		emailsURL := strings.TrimSuffix(p.UserinfoURL, "/") + "/emails"
		if strings.HasSuffix(p.UserinfoURL, "/user") {
			if data2, err2 := e.do(ctx, http.MethodGet, emailsURL, "", nil, headers); err2 == nil {
				var emails []struct {
					Email   string `json:"email"`
					Primary bool   `json:"primary"`
				}
				if json.Unmarshal(data2, &emails) == nil {
					for _, em := range emails {
						if em.Primary && em.Email != "" {
							email = em.Email
							break
						}
					}
				}
			}
		}
	}
	return &IDTokenResult{
		Subject:   strconv.FormatInt(user.ID, 10),
		Email:     email,
		Name:      name,
		AvatarURL: user.AvatarURL,
	}, nil
}

// identityWeChat: GET userinfo?access_token&openid锛泂ubject = unionid 浼樺厛銆?
func (e *OAuth2Exchanger) identityWeChat(ctx context.Context, p *OIDCProviderConfig, accessTokenAndOpenID string) (*IDTokenResult, error) {
	parts := strings.SplitN(accessTokenAndOpenID, "|", 2)
	accessToken, openid := parts[0], ""
	if len(parts) == 2 {
		openid = parts[1]
	}
	if openid == "" {
		return nil, errors.New("oauth2 wechat: openid missing in token response")
	}
	q := url.Values{}
	q.Set("access_token", accessToken)
	q.Set("openid", openid)
	q.Set("lang", "zh_CN")
	sep := "?"
	if strings.Contains(p.UserinfoURL, "?") {
		sep = "&"
	}
	data, err := e.do(ctx, http.MethodGet, p.UserinfoURL+sep+q.Encode(), "", nil, nil)
	if err != nil {
		return nil, err
	}
	var user struct {
		OpenID   string `json:"openid"`
		UnionID  string `json:"unionid"`
		Nickname string `json:"nickname"`
		HeadImg  string `json:"headimgurl"`
		ErrCode  int    `json:"errcode"`
		ErrMsg   string `json:"errmsg"`
	}
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("oauth2 wechat: malformed userinfo: %w", err)
	}
	if user.ErrCode != 0 && user.OpenID == "" {
		return nil, fmt.Errorf("oauth2 wechat: userinfo failed: errcode=%d errmsg=%s", user.ErrCode, user.ErrMsg)
	}
	// unionid 浼樺厛锛氬悓涓讳綋澶氬簲鐢ㄤ笉浜х敓閲嶅韬唤
	subject := user.UnionID
	if subject == "" {
		subject = user.OpenID
	}
	if subject == "" {
		return nil, errors.New("oauth2 wechat: neither unionid nor openid present")
	}
	return &IDTokenResult{
		Subject:   subject,
		Name:      user.Nickname,
		AvatarURL: user.HeadImg,
	}, nil
}

// identityDingTalk: GET contact/users/me锛坔eader x-acs-dingtalk-access-token锛夈€?
func (e *OAuth2Exchanger) identityDingTalk(ctx context.Context, p *OIDCProviderConfig, accessToken string) (*IDTokenResult, error) {
	data, err := e.do(ctx, http.MethodGet, p.UserinfoURL, "", nil,
		map[string]string{"x-acs-dingtalk-access-token": accessToken})
	if err != nil {
		return nil, err
	}
	var user struct {
		UnionID   string `json:"unionId"`
		OpenID    string `json:"openId"`
		Nick      string `json:"nick"`
		Email     string `json:"email"`
		Mobile    string `json:"mobile"`
		AvatarURL string `json:"avatarUrl"`
	}
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("oauth2 dingtalk: malformed userinfo: %w", err)
	}
	subject := user.UnionID
	if subject == "" {
		subject = user.OpenID
	}
	if subject == "" {
		return nil, errors.New("oauth2 dingtalk: neither unionId nor openId present")
	}
	return &IDTokenResult{
		Subject:   subject,
		Email:     user.Email,
		Name:      user.Nick,
		AvatarURL: user.AvatarURL,
		Phone:     user.Mobile,
	}, nil
}

// identityFeishu: GET userinfo锛圔earer锛夛紝鍝嶅簲鍏煎椤跺眰/data 涓ょ褰㈡€併€?
func (e *OAuth2Exchanger) identityFeishu(ctx context.Context, p *OIDCProviderConfig, accessToken string) (*IDTokenResult, error) {
	data, err := e.do(ctx, http.MethodGet, p.UserinfoURL, "", nil,
		map[string]string{"Authorization": "Bearer " + accessToken})
	if err != nil {
		return nil, err
	}
	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("oauth2 feishu: malformed userinfo: %w", err)
	}
	src := resp
	if nested, ok := resp["data"].(map[string]any); ok {
		src = nested
	}
	subject := firstString(src, "sub", "open_id", "union_id")
	if subject == "" {
		return nil, fmt.Errorf("oauth2 feishu: no subject in userinfo: %s", snippet(data))
	}
	return &IDTokenResult{
		Subject:   subject,
		Email:     firstString(src, "email"),
		Name:      firstString(src, "name", "en_name"),
		AvatarURL: firstString(src, "avatar_url", "avatar"),
		Phone:     firstString(src, "mobile", "phone_number"),
	}, nil
}

// identityQQ: 鍏?GET /oauth2.0/me 鎷?openid锛屽啀 GET userinfo銆?
func (e *OAuth2Exchanger) identityQQ(ctx context.Context, p *OIDCProviderConfig, accessToken string) (*IDTokenResult, error) {
	meURL := qqOpenIDURL + "?access_token=" + url.QueryEscape(accessToken) + "&fmt=json"
	data, err := e.do(ctx, http.MethodGet, meURL, "", nil, nil)
	if err != nil {
		return nil, err
	}
	var me struct {
		ClientID string `json:"client_id"`
		OpenID   string `json:"openid"`
	}
	if err := json.Unmarshal(data, &me); err != nil {
		return nil, fmt.Errorf("oauth2 qq: malformed /me response: %w", err)
	}
	if me.OpenID == "" {
		return nil, errors.New("oauth2 qq: openid missing in /me response")
	}

	q := url.Values{}
	q.Set("access_token", accessToken)
	q.Set("oauth_consumer_key", p.ClientID)
	q.Set("openid", me.OpenID)
	sep := "?"
	if strings.Contains(p.UserinfoURL, "?") {
		sep = "&"
	}
	data, err = e.do(ctx, http.MethodGet, p.UserinfoURL+sep+q.Encode(), "", nil, nil)
	if err != nil {
		return nil, err
	}
	var user struct {
		Ret      int    `json:"ret"`
		Msg      string `json:"msg"`
		Nickname string `json:"nickname"`
		FigureQQ string `json:"figureurl_qq_1"`
	}
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("oauth2 qq: malformed userinfo: %w", err)
	}
	if user.Ret != 0 {
		return nil, fmt.Errorf("oauth2 qq: userinfo failed: ret=%d msg=%s", user.Ret, user.Msg)
	}
	return &IDTokenResult{
		Subject:   me.OpenID, // QQ 鏃?unionid锛堥櫎闈炴帴浜掕仈楂樼骇鎺ュ彛锛?
		Name:      user.Nickname,
		AvatarURL: user.FigureQQ,
	}, nil
}

// 鈹€鈹€ HTTP 杈呭姪 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func (e *OAuth2Exchanger) do(ctx context.Context, method, rawURL, contentType string, body io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth2: request %s failed: %w", redactURL(rawURL), err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth2: %s %s -> HTTP %d: %s", method, redactURL(rawURL), resp.StatusCode, snippet(data))
	}
	return data, nil
}

// redactURL 鎶瑰幓 URL 涓殑 secret/token/code 鍙傛暟锛岄槻鏃ュ織娉勯湶銆?
func redactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "<unparseable-url>"
	}
	q := u.Query()
	for key := range q {
		switch key {
		case "secret", "client_secret", "access_token", "code", "app_secret":
			q.Set(key, "***")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// firstString 渚濇灏濊瘯澶氫釜鐐瑰垎璺緞锛岃繑鍥為涓潪绌哄瓧绗︿覆銆?
func firstString(m map[string]any, paths ...string) string {
	for _, path := range paths {
		var cur any = m
		matched := true
		for _, seg := range strings.Split(path, ".") {
			node, isMap := cur.(map[string]any)
			if !isMap {
				matched = false
				break
			}
			v, exists := node[seg]
			if !exists {
				matched = false
				break
			}
			cur = v
		}
		if matched {
			if s, isStr := cur.(string); isStr && s != "" {
				return s
			}
		}
	}
	return ""
}

// snippet 鎴彇鍝嶅簲鐗囨鐢ㄤ簬閿欒淇℃伅锛堜笂闄?200 瀛楄妭锛夈€?
func snippet(data []byte) string {
	s := string(data)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
