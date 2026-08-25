package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// 鈹€鈹€ state / nonce 缂栬В鐮?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
//
// state 浠ょ墝鏍煎紡: base64url(JSON payload) + "." + base64url(HMAC-SHA256(payload))銆?
// payload 鎼哄甫 provider_id / nonce / 杩囨湡鏃堕棿锛岄槻 CSRF锛圚MAC 涓嶅彲浼€狅級涓庨噸鏀撅紙鐭?TTL锛夈€?

var (
	// ErrStateInvalid 琛ㄧず state 缁撴瀯闈炴硶鎴?HMAC 鏍￠獙澶辫触锛堝惈绡℃敼锛夈€?
	ErrStateInvalid = errors.New("invalid sso state")
	// ErrStateExpired 琛ㄧず state 宸茶繃鏈熴€?
	ErrStateExpired = errors.New("sso state expired")
)

// StatePayload 鏄?state 浠ょ墝鎵胯浇鐨勬暟鎹€?
type StatePayload struct {
	ProviderID string `json:"p"`
	Nonce      string `json:"n"`
	ExpiresAt  int64  `json:"e"` // unix 绉?
	// Mode: "login"锛堥粯璁わ紝鍏煎鏃т护鐗岀殑绌哄€硷級| "bind"锛堢粦瀹氬凡鏈夎处鍙凤級
	Mode string `json:"m,omitempty"`
	// UID 浠?bind 妯″紡鎼哄甫锛氬彂璧风粦瀹氱殑宸茬櫥褰曠敤鎴凤紙state 鐢?authMW 淇濇姢鐨勮矾鐢辩鍙戯級
	UID string `json:"u,omitempty"`
}

// State 鍙屾ā寮忓父閲忋€?
const (
	StateModeLogin = "login"
	StateModeBind  = "bind"
)

// StateCodec 绛惧彂/鏍￠獙 HMAC 绛惧悕鐨?SSO state 浠ょ墝銆?
type StateCodec struct {
	key []byte
	ttl time.Duration
	// now 鍙娴嬭瘯鏇挎崲锛岀敤浜庤繃鏈熷垎鏀獙璇?
	now func() time.Time
}

// NewStateCodec 鏋勯€?StateCodec銆俴ey 涓虹鍚嶅瘑閽ワ紙鈮? 瀛楄妭锛夛紝ttl 涓烘湁鏁堟湡銆?
func NewStateCodec(key []byte, ttl time.Duration) *StateCodec {
	return NewStateCodecWithClock(key, ttl, time.Now)
}

// NewStateCodecWithClock 鍏佽娉ㄥ叆鏃堕挓锛堜緵娴嬭瘯楠岃瘉杩囨湡鍒嗘敮锛夈€?
func NewStateCodecWithClock(key []byte, ttl time.Duration, now func() time.Time) *StateCodec {
	return &StateCodec{key: key, ttl: ttl, now: now}
}

// Issue 绛惧彂 state 浠ょ墝锛坧ayload JSON 鈫?base64url锛岄檮 HMAC-SHA256 绛惧悕锛夈€?
func (c *StateCodec) Issue(providerID, nonce string) (string, error) {
	return c.IssueMode(providerID, nonce, StateModeLogin, "")
}

// IssueMode 绛惧彂鎸囧畾妯″紡鐨?state锛坆ind 妯″紡椤绘惡甯?uid锛夈€?
func (c *StateCodec) IssueMode(providerID, nonce, mode, uid string) (string, error) {
	if providerID == "" || nonce == "" {
		return "", errors.New("state codec: providerID and nonce are required")
	}
	switch mode {
	case StateModeLogin:
		uid = "" // login 妯″紡涓嶆惡甯?uid
	case StateModeBind:
		if uid == "" {
			return "", errors.New("state codec: bind mode requires uid")
		}
	default:
		return "", errors.New("state codec: unknown mode " + mode)
	}
	payload := StatePayload{
		ProviderID: providerID,
		Nonce:      nonce,
		ExpiresAt:  c.now().Add(c.ttl).Unix(),
		Mode:       mode,
		UID:        uid,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	return body + "." + c.sign(body), nil
}

// Verify 鏍￠獙 state 浠ょ墝骞惰繑鍥?payload銆?
// 缁撴瀯闈炴硶/HMAC 涓嶅尮閰?鈫?ErrStateInvalid锛涜秴杩囨湁鏁堟湡 鈫?ErrStateExpired銆?
func (c *StateCodec) Verify(state string) (*StatePayload, error) {
	body, sig, found := cutState(state)
	if !found {
		return nil, ErrStateInvalid
	}
	if !hmac.Equal([]byte(sig), []byte(c.sign(body))) {
		return nil, ErrStateInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, ErrStateInvalid
	}
	var payload StatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, ErrStateInvalid
	}
	if payload.ProviderID == "" || payload.Nonce == "" {
		return nil, ErrStateInvalid
	}
	if c.now().Unix() > payload.ExpiresAt {
		return nil, ErrStateExpired
	}
	return &payload, nil
}

func cutState(state string) (body, sig string, found bool) {
	for i := len(state) - 1; i >= 0; i-- {
		if state[i] == '.' {
			return state[:i], state[i+1:], true
		}
	}
	return "", "", false
}

func (c *StateCodec) sign(body string) string {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// RandomNonce 鐢熸垚 16 瀛楄妭闅忔満鍗佸叚杩涘埗 nonce銆?
func RandomNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// 鈹€鈹€ OIDC IdP 浜や簰 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// OIDCProviderConfig 鏄彂璧?OIDC 娴佺▼鎵€闇€鐨?provider 閰嶇疆锛坰ecret 宸茶В瀵嗭級銆?
// protocol=oauth2 鏃?Issuer 鍙负绌猴紝鏀圭敤 AuthURL/TokenURL/UserinfoURL銆?
type OIDCProviderConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	Scopes       []string
	RedirectURL  string

	// 鈹€鈹€ OAuth2 鎵╁睍锛坧rotocol=oauth2 鏃剁敓鏁堬級鈹€鈹€
	Protocol     string            // "oidc" | "oauth2"锛涚┖瑙嗕负 oidc
	ProviderType string            // github/wechat/dingtalk/feishu/qq/custom
	AuthURL      string            // 鎺堟潈绔偣锛堟ā鏉跨己鐪佽嚜鍔ㄥ～鍏咃級
	TokenURL     string            // token 绔偣
	UserinfoURL  string            // userinfo 绔偣
	Extra        map[string]string // provider 鐗规湁椤癸紙寰俊 mode=open|mp 绛夛級
}

// IDTokenResult 鏄巿鏉冪爜鎹?token + 韬唤鏍￠獙鍚庣殑缁撴灉
//锛圤IDC 鏉ヨ嚜 id_token claims锛汷Auth2 鏉ヨ嚜 userinfo 鎺ュ彛锛夈€?
type IDTokenResult struct {
	Subject string
	Email   string
	// Roles 鏉ヨ嚜 IdP 鐨勫彲閫?"roles" claim锛堝瓧绗︿覆鎴栧瓧绗︿覆鏁扮粍锛夛紝鐢ㄤ簬 role_mapping 鍖归厤
	Roles []string
	// Name / AvatarURL / Phone 涓?userinfo 鍙€夊瓧娈碉紙OAuth2 瀹舵棌濉厖锛?
	Name      string
	AvatarURL string
	Phone     string
}

// OIDCExchanger 鎶借薄涓?IdP 鐨勪氦浜掞紙鍙戠幇/鎺堟潈 URL/鎺堟潈鐮佷氦鎹?id_token 鏍￠獙锛夛紝
// 娴嬭瘯涓彲鏇挎崲涓?fake锛屾棤闇€鐪熷疄缃戠粶銆?
type OIDCExchanger interface {
	// AuthURL 鏋勯€?IdP 鎺堟潈椤?URL锛堟惡甯?state 涓?nonce锛夈€?
	AuthURL(ctx context.Context, p *OIDCProviderConfig, state, nonce string) (string, error)
	// ExchangeAndVerify 鐢ㄦ巿鏉冪爜鎹?token 骞舵牎楠?id_token锛坅ud/iss/exp + nonce锛夈€?
	ExchangeAndVerify(ctx context.Context, p *OIDCProviderConfig, code, expectedNonce string) (*IDTokenResult, error)
}

// RemoteOIDCExchanger 鏄熀浜?go-oidc 鐨勭湡瀹炲疄鐜帮紝鎸?issuer 缂撳瓨鍙戠幇缁撴灉銆?
type RemoteOIDCExchanger struct {
	mu        sync.Mutex
	providers map[string]*gooidc.Provider
}

func NewRemoteOIDCExchanger() *RemoteOIDCExchanger {
	return &RemoteOIDCExchanger{providers: make(map[string]*gooidc.Provider)}
}

func (e *RemoteOIDCExchanger) discover(ctx context.Context, p *OIDCProviderConfig) (*gooidc.Provider, *oauth2.Config, error) {
	e.mu.Lock()
	provider, ok := e.providers[p.Issuer]
	e.mu.Unlock()
	if !ok {
		var err error
		provider, err = gooidc.NewProvider(ctx, p.Issuer)
		if err != nil {
			return nil, nil, fmt.Errorf("oidc discovery %s: %w", p.Issuer, err)
		}
		e.mu.Lock()
		e.providers[p.Issuer] = provider
		e.mu.Unlock()
	}
	scopes := p.Scopes
	if len(scopes) == 0 {
		scopes = []string{gooidc.ScopeOpenID, "email", "profile"}
	}
	cfg := &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  p.RedirectURL,
		Scopes:       scopes,
	}
	return provider, cfg, nil
}

func (e *RemoteOIDCExchanger) AuthURL(ctx context.Context, p *OIDCProviderConfig, state, nonce string) (string, error) {
	_, cfg, err := e.discover(ctx, p)
	if err != nil {
		return "", err
	}
	return cfg.AuthCodeURL(state, gooidc.Nonce(nonce)), nil
}

func (e *RemoteOIDCExchanger) ExchangeAndVerify(ctx context.Context, p *OIDCProviderConfig, code, expectedNonce string) (*IDTokenResult, error) {
	provider, cfg, err := e.discover(ctx, p)
	if err != nil {
		return nil, err
	}
	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oidc exchange: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, errors.New("oidc: id_token missing in token response")
	}
	idToken, err := provider.Verifier(&gooidc.Config{ClientID: p.ClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oidc verify id_token: %w", err)
	}
	// go-oidc 涓嶈嚜鍔ㄦ牎楠?nonce锛岃繖閲屾墜鍔ㄦ亽绛夋瘮杈冿紙闃查噸鏀撅級
	if idToken.Nonce != expectedNonce {
		return nil, errors.New("oidc: id_token nonce mismatch")
	}

	result := &IDTokenResult{Subject: idToken.Subject}
	var claims struct {
		Email string `json:"email"`
		Roles any    `json:"roles"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("oidc parse claims: %w", err)
	}
	result.Email = claims.Email
	result.Roles = normalizeRolesClaim(claims.Roles)
	return result, nil
}

// normalizeRolesClaim 灏?IdP "roles" claim 褰掍竴鍖栦负瀛楃涓插垏鐗囥€?
// 鏀寔鍗曞瓧绗︿覆銆佸瓧绗︿覆鏁扮粍涓ょ甯歌褰㈡€侊紱鍏朵粬褰㈡€佽繑鍥炵┖鍒囩墖銆?
func normalizeRolesClaim(v any) []string {
	switch val := v.(type) {
	case string:
		if val == "" {
			return nil
		}
		return []string{val}
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		sort.Strings(out)
		return out
	default:
		return nil
	}
}
