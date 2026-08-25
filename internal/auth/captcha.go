package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 鈹€鈹€ 浜烘満楠岃瘉锛圕APTCHA锛夐槻鎺ュ彛婊ョ敤 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
//
// 鏀寔鐨勯獙璇佹湇鍔″晢锛?
//   - turnstile : Cloudflare Turnstile锛堥粯璁わ紝鍏嶈垂鏃犳劅锛?
//   - recaptcha : Google reCAPTCHA v2/v3
//   - hcaptcha  : hCaptcha
//   - tencent   : 鑵捐浜戦獙璇佺爜锛圱Captcha锛岄渶 Ticket + Randstr锛?
//   - custom    : 鑷畾涔?HTTP 绔偣锛堢害瀹氬绾﹁ CustomCaptchaContract锛?
//
// 鎵€鏈夋湇鍔″晢鐨勬湇鍔＄鏍￠獙鍧囦负 HTTP 璋冪敤锛屼笉寮曞叆澶?SDK銆?

// CaptchaProvider 鏄敮鎸佺殑楠岃瘉鐮佹湇鍔″晢绫诲瀷銆?
const (
	CaptchaTurnstile = "turnstile"
	CaptchaRecaptcha = "recaptcha"
	CaptchaHCaptcha  = "hcaptcha"
	CaptchaTencent   = "tencent"
	CaptchaCustom    = "custom"
)

// CaptchaKnownProviders 杩斿洖鍏ㄩ儴鍙楁敮鎸佺殑楠岃瘉鐮佹湇鍔″晢锛堥厤缃牎楠岀敤锛夈€?
func CaptchaKnownProviders() []string {
	return []string{CaptchaTurnstile, CaptchaRecaptcha, CaptchaHCaptcha, CaptchaTencent, CaptchaCustom}
}

// IsKnownCaptchaProvider 鍒ゆ柇 provider 绫诲瀷鏄惁鍙楁敮鎸併€?
func IsKnownCaptchaProvider(p string) bool {
	switch p {
	case CaptchaTurnstile, CaptchaRecaptcha, CaptchaHCaptcha, CaptchaTencent, CaptchaCustom:
		return true
	}
	return false
}

// 鍚勬湇鍔″晢榛樿鏈嶅姟绔牎楠岀鐐癸紙cfg.VerifyURL 闈炵┖鏃惰鐩栵紝渚涙祴璇曚笌浠ｇ悊鍦烘櫙锛夈€?
var defaultCaptchaEndpoints = map[string]string{
	CaptchaTurnstile: "https://challenges.cloudflare.com/turnstile/v0/siteverify",
	CaptchaRecaptcha: "https://www.google.com/recaptcha/api/siteverify",
	CaptchaHCaptcha:  "https://api.hcaptcha.com/siteverify",
	CaptchaTencent:   "https://ssl.captcha.qq.com/ticket/verify",
}

// CaptchaConfig 鏄竴娆￠獙璇佹墍闇€鐨勫畬鏁撮厤缃紙secret 宸茶В瀵嗭級銆?
type CaptchaConfig struct {
	Provider  string // turnstile/recaptcha/hcaptcha/tencent/custom
	SiteKey   string
	Secret    string
	VerifyURL string // custom 蹇呭～锛涘叾浣欎负瑕嗙洊椤?
}

// CaptchaToken 鏄墠绔彁浜ょ殑楠岃瘉鍑嵁銆?
type CaptchaToken struct {
	Token  string // turnstile/recaptcha/hcaptcha 鐨?token锛泃encent 鐨?Ticket锛沜ustom 鐨?token
	Randstr string // 鑵捐浜戦獙璇佺爜涓撶敤闅忔満涓?
}

// ErrCaptchaFailed 琛ㄧず楠岃瘉鐮佹牎楠屾湭閫氳繃锛堜笟鍔″彲鍥?400/403锛夈€?
var ErrCaptchaFailed = errors.New("captcha verification failed")

// ErrCaptchaUnreachable 琛ㄧず楠岃瘉鏈嶅姟鍟嗕笉鍙揪锛堜笟鍔″彲鍥?502锛宖ail-loud锛夈€?
var ErrCaptchaUnreachable = errors.New("captcha provider unreachable")

// CaptchaVerifier 鎶借薄楠岃瘉鐮佹湇鍔＄鏍￠獙锛屾祴璇曞彲鏇挎崲銆?
type CaptchaVerifier interface {
	// Verify 鍚戞湇鍔″晢鏍￠獙 token锛涢€氳繃杩斿洖 nil銆?
	Verify(ctx context.Context, cfg *CaptchaConfig, tok *CaptchaToken, remoteIP string) error
}

// HTTPCaptchaVerifier 鏄湡瀹炲疄鐜帮細鎸?provider 鍒嗘淳璇锋眰/鍝嶅簲鏍煎紡銆?
type HTTPCaptchaVerifier struct {
	client *http.Client
}

// NewHTTPCaptchaVerifier 鏋勯€犻獙璇佸櫒锛?0s 瓒呮椂锛夈€?
func NewHTTPCaptchaVerifier() *HTTPCaptchaVerifier {
	return &HTTPCaptchaVerifier{client: &http.Client{Timeout: 10 * time.Second}}
}

func (v *HTTPCaptchaVerifier) endpoint(cfg *CaptchaConfig) (string, error) {
	if cfg.VerifyURL != "" {
		return cfg.VerifyURL, nil
	}
	ep, ok := defaultCaptchaEndpoints[cfg.Provider]
	if !ok {
		return "", fmt.Errorf("captcha: no default endpoint for provider %q", cfg.Provider)
	}
	return ep, nil
}

// Verify 鎸夋湇鍔″晢鍗忚鏍￠獙 token銆?
func (v *HTTPCaptchaVerifier) Verify(ctx context.Context, cfg *CaptchaConfig, tok *CaptchaToken, remoteIP string) error {
	if cfg == nil {
		return errors.New("captcha: nil config")
	}
	if !IsKnownCaptchaProvider(cfg.Provider) {
		return fmt.Errorf("captcha: unknown provider %q", cfg.Provider)
	}
	if cfg.Provider == CaptchaCustom && cfg.VerifyURL == "" {
		return errors.New("captcha: custom provider requires verify_url")
	}
	if tok == nil || strings.TrimSpace(tok.Token) == "" {
		return ErrCaptchaFailed
	}

	endpoint, err := v.endpoint(cfg)
	if err != nil {
		return err
	}

	switch cfg.Provider {
	case CaptchaTencent:
		return v.verifyTencent(ctx, endpoint, cfg, tok, remoteIP)
	case CaptchaCustom:
		return v.verifyCustom(ctx, endpoint, cfg, tok, remoteIP)
	default:
		// turnstile / recaptcha / hcaptcha 鍏辩敤鍚屼竴濂?form 琛ㄥ崟 + {"success": bool} 濂戠害
		return v.verifyFormJSON(ctx, endpoint, cfg, tok, remoteIP)
	}
}

// verifyFormJSON 瑕嗙洊 turnstile / recaptcha / hcaptcha锛?
// POST application/x-www-form-urlencoded锛坰ecret/response/remoteip锛夆啋 {"success": bool}銆?
func (v *HTTPCaptchaVerifier) verifyFormJSON(ctx context.Context, endpoint string, cfg *CaptchaConfig, tok *CaptchaToken, remoteIP string) error {
	form := url.Values{}
	form.Set("secret", cfg.Secret)
	form.Set("response", tok.Token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	body, err := v.post(ctx, endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCaptchaUnreachable, err)
	}
	var resp struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("captcha: malformed provider response: %w", err)
	}
	if !resp.Success {
		return ErrCaptchaFailed
	}
	return nil
}

// verifyTencent 鑵捐浜戦獙璇佺爜锛圱Captcha锛夛細
// POST form锛坅id=site_key, AppSecretKey=secret, Ticket, Randstr, UserIP锛夆啋 {"response":"1"}銆?
func (v *HTTPCaptchaVerifier) verifyTencent(ctx context.Context, endpoint string, cfg *CaptchaConfig, tok *CaptchaToken, remoteIP string) error {
	if tok.Randstr == "" {
		return ErrCaptchaFailed // 鑵捐楠岃瘉鐮佸繀椤绘惡甯?Randstr
	}
	form := url.Values{}
	form.Set("aid", cfg.SiteKey)
	form.Set("AppSecretKey", cfg.Secret)
	form.Set("Ticket", tok.Token)
	form.Set("Randstr", tok.Randstr)
	form.Set("UserIP", remoteIP)

	body, err := v.post(ctx, endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCaptchaUnreachable, err)
	}
	var resp struct {
		Response string `json:"response"`
		ErrMSG   string `json:"err_msg"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("captcha: malformed tencent response: %w", err)
	}
	if resp.Response != "1" {
		return fmt.Errorf("%w: tencent err_msg=%q", ErrCaptchaFailed, resp.ErrMSG)
	}
	return nil
}

// CustomCaptchaContract 鑷畾涔夐獙璇佺鐐瑰绾︼細
//
//	POST {verify_url}
//	Content-Type: application/json
//	{"secret": "...", "token": "...", "randstr": "...", "remote_ip": "..."}
//
//	鈫?HTTP 200 涓?{"success": true} 瑙嗕负閫氳繃锛涘叾浣欎竴寰嬫嫆缁濄€?
//
// 璇ュ绾﹁冻浠ユ帴鍏ヤ换鎰忚嚜寤?绗笁鏂归獙璇佹湇鍔★紙缃戝叧渚у仛閫傞厤灞傚嵆鍙級銆?
func (v *HTTPCaptchaVerifier) verifyCustom(ctx context.Context, endpoint string, cfg *CaptchaConfig, tok *CaptchaToken, remoteIP string) error {
	payload, err := json.Marshal(map[string]string{
		"secret":   cfg.Secret,
		"token":    tok.Token,
		"randstr":  tok.Randstr,
		"remote_ip": remoteIP,
	})
	if err != nil {
		return err
	}
	body, status, err := v.postStatus(ctx, endpoint, "application/json", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCaptchaUnreachable, err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("%w: custom endpoint status %d", ErrCaptchaFailed, status)
	}
	var resp struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("captcha: malformed custom response: %w", err)
	}
	if !resp.Success {
		return ErrCaptchaFailed
	}
	return nil
}

func (v *HTTPCaptchaVerifier) post(ctx context.Context, endpoint, contentType string, body io.Reader) ([]byte, error) {
	data, _, err := v.postStatus(ctx, endpoint, contentType, body)
	return data, err
}

func (v *HTTPCaptchaVerifier) postStatus(ctx context.Context, endpoint, contentType string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	// 涓婇檺 64KB锛岄槻鎭舵剰绔偣鎷栧灝缃戝叧
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}
