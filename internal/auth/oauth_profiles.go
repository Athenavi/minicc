package auth

import (
	"net/url"
	"strings"
)

// 鈹€鈹€ OAuth2 / OIDC Provider 妯℃澘娉ㄥ唽琛?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
//
// 绠＄悊绔€夋嫨 provider_type 鍚庣鐐硅嚜鍔ㄥ～鍏咃紝浠嶅彲鍦ㄦ暟鎹簱涓鐩?
//锛坅uth_url/token_url/userinfo_url 鍒楅潪绌哄嵆瑕嗙洊妯℃澘鍊硷級銆?

// ProviderProfile 鏄璁?provider 妯℃澘銆?
type ProviderProfile struct {
	Type        string   // google/github/wechat/dingtalk/feishu/qq/custom
	Protocol    string   // oidc | oauth2
	Issuer      string   // oidc 鐢?
	AuthURL     string   // oauth2 鐢?
	TokenURL    string   // oauth2 鐢?
	UserinfoURL string   // oauth2 鐢?
	Scopes      []string // 榛樿 scopes
}

// 鍗忚甯搁噺銆?
const (
	ProtocolOIDC   = "oidc"
	ProtocolOAuth2 = "oauth2"
)

// Provider 绫诲瀷甯搁噺銆?
const (
	ProviderGoogle  = "google"
	ProviderGitHub  = "github"
	ProviderWeChat  = "wechat"
	ProviderDingTalk = "dingtalk"
	ProviderFeishu  = "feishu"
	ProviderQQ      = "qq"
	ProviderCustom  = "custom"
)

// providerProfiles 鍐呯疆妯℃澘锛堢鐐逛互鍚勫钩鍙板叕寮€鏂囨。涓哄噯锛夈€?
var providerProfiles = map[string]ProviderProfile{
	ProviderGoogle: {
		Type:     ProviderGoogle,
		Protocol: ProtocolOIDC,
		Issuer:   "https://accounts.google.com",
		Scopes:   []string{"openid", "email", "profile"},
	},
	ProviderGitHub: {
		Type:        ProviderGitHub,
		Protocol:    ProtocolOAuth2,
		AuthURL:     "https://github.com/login/oauth/authorize",
		TokenURL:    "https://github.com/login/oauth/access_token",
		UserinfoURL: "https://api.github.com/user",
		Scopes:      []string{"read:user", "user:email"},
	},
	ProviderWeChat: {
		// 寮€鏀惧钩鍙版壂鐮佺櫥褰曪紱extra.mode=mp 鏃跺垏鎹负鍏紬鍙风綉椤垫巿鏉冪鐐?
		Type:        ProviderWeChat,
		Protocol:    ProtocolOAuth2,
		AuthURL:     "https://open.weixin.qq.com/connect/qrconnect",
		TokenURL:    "https://api.weixin.qq.com/sns/oauth2/access_token",
		UserinfoURL: "https://api.weixin.qq.com/sns/userinfo",
		Scopes:      []string{"snsapi_login"},
	},
	ProviderDingTalk: {
		Type:        ProviderDingTalk,
		Protocol:    ProtocolOAuth2,
		AuthURL:     "https://login.dingtalk.com/oauth2/auth",
		TokenURL:    "https://api.dingtalk.com/v1.0/oauth2/userAccessToken",
		UserinfoURL: "https://api.dingtalk.com/contact/users/me",
		Scopes:      []string{"openid"},
	},
	ProviderFeishu: {
		Type:        ProviderFeishu,
		Protocol:    ProtocolOAuth2,
		AuthURL:     "https://passport.feishu.cn/suite/passport/oauth/v2/authorize",
		TokenURL:    "https://passport.feishu.cn/suite/passport/oauth/v2/token",
		UserinfoURL: "https://passport.feishu.cn/suite/passport/oauth/v2/userinfo",
		Scopes:      []string{"user_info"},
	},
	ProviderQQ: {
		Type:        ProviderQQ,
		Protocol:    ProtocolOAuth2,
		AuthURL:     "https://graph.qq.com/oauth2.0/authorize",
		TokenURL:    "https://graph.qq.com/oauth2.0/token",
		UserinfoURL: "https://graph.qq.com/user/get_user_info",
		Scopes:      []string{"get_user_info"},
	},
	ProviderCustom: {
		Type:     ProviderCustom,
		Protocol: ProtocolOIDC,
		Scopes:   []string{"openid", "email", "profile"},
	},
}

// GetProviderProfile 杩斿洖 provider 妯℃澘锛涙湭鐭ョ被鍨嬭繑鍥?ok=false銆?
func GetProviderProfile(providerType string) (ProviderProfile, bool) {
	p, ok := providerProfiles[providerType]
	return p, ok
}

// KnownProviderTypes 杩斿洖鍏ㄩ儴鍐呯疆 provider 绫诲瀷銆?
func KnownProviderTypes() []string {
	return []string{ProviderGoogle, ProviderGitHub, ProviderWeChat, ProviderDingTalk, ProviderFeishu, ProviderQQ, ProviderCustom}
}

// IsKnownProviderType 鍒ゆ柇 provider 绫诲瀷鏄惁鍙楁敮鎸併€?
func IsKnownProviderType(t string) bool {
	_, ok := providerProfiles[t]
	return ok
}

// ValidProtocol 鍒ゆ柇鍗忚鍙栧€煎悎娉曘€?
func ValidProtocol(p string) bool {
	return p == ProtocolOIDC || p == ProtocolOAuth2
}

// ResolveEndpoints 鐢ㄦā鏉跨己鐪佸€艰ˉ榻?provider 绔偣锛?
// 鏄惧紡瑕嗙洊锛坥verride 闈炵┖锛変紭鍏堜簬妯℃澘銆俰ssuer 鍚岀悊銆?
func ResolveEndpoints(providerType string, issuer, authURL, tokenURL, userinfoURL string) (string, string, string, string) {
	profile, ok := providerProfiles[providerType]
	if !ok {
		return issuer, authURL, tokenURL, userinfoURL
	}
	if issuer == "" {
		issuer = profile.Issuer
	}
	if authURL == "" {
		authURL = profile.AuthURL
	}
	if tokenURL == "" {
		tokenURL = profile.TokenURL
	}
	if userinfoURL == "" {
		userinfoURL = profile.UserinfoURL
	}
	return issuer, authURL, tokenURL, userinfoURL
}

// DefaultScopes 杩斿洖 provider 妯℃澘榛樿 scopes锛堟棤妯℃澘鏃剁敤 OIDC 缂虹渷锛夈€?
func DefaultScopes(providerType string) []string {
	if profile, ok := providerProfiles[providerType]; ok && len(profile.Scopes) > 0 {
		return profile.Scopes
	}
	return []string{"openid", "email", "profile"}
}

// wechatMPAuthURL 鍏紬鍙风綉椤垫巿鏉冪鐐癸紙extra.mode=mp锛夈€?
const wechatMPAuthURL = "https://open.weixin.qq.com/connect/oauth2/authorize"

// wechatEffectiveAuthURL 寰俊鎺堟潈绔偣锛歟xtra.mode=mp 鍒囨崲涓哄叕浼楀彿缃戦〉鎺堟潈銆?
func wechatEffectiveAuthURL(authURL string, extra map[string]string) string {
	if extra["mode"] == "mp" {
		return wechatMPAuthURL
	}
	return authURL
}

// buildAuthURL 鎸夐€氱敤 OAuth2 鍙傛暟鏋勯€犳巿鏉?URL锛坈lient_id/redirect_uri/response_type/scope/state锛夈€?
func buildAuthURL(base, clientID, redirectURL string, scopes []string, state string, extraParams url.Values) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURL)
	q.Set("response_type", "code")
	if len(scopes) > 0 {
		q.Set("scope", strings.Join(scopes, " "))
	}
	q.Set("state", state)
	for k, vs := range extraParams {
		for _, v := range vs {
			q.Set(k, v)
		}
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + q.Encode()
}
