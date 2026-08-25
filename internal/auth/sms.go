package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// 鈹€鈹€ 鐭俊楠岃瘉鐮侊紙SMS锛夌櫥褰?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
//
// 鏀寔鐨勭煭淇℃湇鍔″晢锛?
//   - aliyun : 闃块噷浜戠煭淇★紙dysmsapi锛孭OP RPC 绛惧悕 V1锛?
//   - tencent: 鑵捐浜戠煭淇★紙sms.tencentcloudapi.com锛孴C3-HMAC-SHA256 绛惧悕锛?
//   - custom : 鑷畾涔?HTTP 绔偣锛堢害瀹氬绾﹁ CustomSmsContract锛?
//
// 鎵€鏈夋湇鍔″晢鐨勫彂閫佸潎涓?HTTP 璋冪敤锛屼笉寮曞叆澶?SDK銆?

// SmsProvider 鏄敮鎸佺殑鐭俊鏈嶅姟鍟嗙被鍨嬨€?
const (
	SmsAliyun  = "aliyun"
	SmsTencent = "tencent"
	SmsCustom  = "custom"
)

// SmsKnownProviders 杩斿洖鍏ㄩ儴鍙楁敮鎸佺殑鐭俊鏈嶅姟鍟嗭紙閰嶇疆鏍￠獙鐢級銆?
func SmsKnownProviders() []string {
	return []string{SmsAliyun, SmsTencent, SmsCustom}
}

// IsKnownSmsProvider 鍒ゆ柇 provider 绫诲瀷鏄惁鍙楁敮鎸併€?
func IsKnownSmsProvider(p string) bool {
	switch p {
	case SmsAliyun, SmsTencent, SmsCustom:
		return true
	}
	return false
}

// 鍚勬湇鍔″晢榛樿鍙戦€佺鐐癸紙cfg.Endpoint 闈炵┖鏃惰鐩栵紝渚涙祴璇曚笌浠ｇ悊鍦烘櫙锛夈€?
const (
	DefaultSmsAliyunEndpoint  = "https://dysmsapi.aliyuncs.com"
	DefaultSmsTencentEndpoint = "https://sms.tencentcloudapi.com"
)

// SmsConfig 鏄竴娆″彂閫佹墍闇€鐨勫畬鏁撮厤缃紙secret 宸茶В瀵嗭級銆?
type SmsConfig struct {
	Provider        string // aliyun/tencent/custom
	SignName        string
	TemplateID      string // 闃块噷浜?TemplateCode / 鑵捐浜?TemplateId / custom template_id
	AccessKeyID     string
	AccessKeySecret string
	Endpoint        string // custom 蹇呭～锛涘叾浣欎负瑕嗙洊椤?
}

// ErrSmsSendFailed 琛ㄧず鏈嶅姟鍟嗘嫆缁濆彂閫侊紙涓氬姟鍙洖 502 + 鍏蜂綋鍘熷洜锛夈€?
var ErrSmsSendFailed = errors.New("sms provider rejected the request")

// ErrSmsUnreachable 琛ㄧず鏈嶅姟鍟嗕笉鍙揪锛堜笟鍔″彲鍥?502锛宖ail-loud锛夈€?
var ErrSmsUnreachable = errors.New("sms provider unreachable")

// SmsSender 鎶借薄鐭俊鍙戦€侊紝娴嬭瘯鍙浛鎹€?
type SmsSender interface {
	// Send 鍙戦€侀獙璇佺爜鐭俊锛涙垚鍔熻繑鍥?nil銆?
	Send(ctx context.Context, cfg *SmsConfig, phone, code string) error
}

// HTTPSmsSender 鏄湡瀹炲疄鐜帮細鎸?provider 鍒嗘淳璇锋眰/绛惧悕鏍煎紡銆?
type HTTPSmsSender struct {
	client *http.Client
}

// NewHTTPSmsSender 鏋勯€犲彂閫佸櫒锛?0s 瓒呮椂锛夈€?
func NewHTTPSmsSender() *HTTPSmsSender {
	return &HTTPSmsSender{client: &http.Client{Timeout: 10 * time.Second}}
}

// Send 鎸夋湇鍔″晢鍗忚鍙戦€侀獙璇佺爜銆?
func (s *HTTPSmsSender) Send(ctx context.Context, cfg *SmsConfig, phone, code string) error {
	if cfg == nil {
		return errors.New("sms: nil config")
	}
	if !IsKnownSmsProvider(cfg.Provider) {
		return fmt.Errorf("sms: unknown provider %q", cfg.Provider)
	}
	if phone == "" || code == "" {
		return errors.New("sms: phone and code are required")
	}
	switch cfg.Provider {
	case SmsAliyun:
		return s.sendAliyun(ctx, cfg, phone, code)
	case SmsTencent:
		return s.sendTencent(ctx, cfg, phone, code)
	default:
		return s.sendCustom(ctx, cfg, phone, code)
	}
}

// 鈹€鈹€ 闃块噷浜戠煭淇★紙POP RPC 绛惧悕 V1锛夆攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// aliyunPercentEncode 鏄樋閲屼簯 POP 鍗忚鐨?RFC3986 鐧惧垎鍙风紪鐮併€?
func aliyunPercentEncode(s string) string {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

// aliyunSign 璁＄畻 POP V1 绛惧悕锛歜ase64(HMAC-SHA1(AccessKeySecret+"&", stringToSign))銆?
func aliyunSign(secret, stringToSign string) string {
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (s *HTTPSmsSender) sendAliyun(ctx context.Context, cfg *SmsConfig, phone, code string) error {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = DefaultSmsAliyunEndpoint
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	templateParam, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		return err
	}
	params := map[string]string{
		"AccessKeyId":      cfg.AccessKeyID,
		"Action":           "SendSms",
		"Format":           "JSON",
		"PhoneNumbers":     phone,
		"RegionId":         "cn-hangzhou",
		"SignName":         cfg.SignName,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   hex.EncodeToString(nonce),
		"SignatureVersion": "1.0",
		"TemplateCode":     cfg.TemplateID,
		"TemplateParam":    string(templateParam),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          "2017-05-25",
	}

	// 瑙勮寖鍖栨煡璇覆锛歬ey 鍗囧簭锛宬=percentEncode(v)
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, aliyunPercentEncode(k)+"="+aliyunPercentEncode(params[k]))
	}
	canonicalQuery := strings.Join(pairs, "&")
	stringToSign := "POST&" + aliyunPercentEncode("/") + "&" + aliyunPercentEncode(canonicalQuery)
	params["Signature"] = aliyunSign(cfg.AccessKeySecret, stringToSign)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	body, err := s.post(ctx, endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSmsUnreachable, err)
	}
	var resp struct {
		Code      string `json:"Code"`
		Message   string `json:"Message"`
		BizID     string `json:"BizId"`
		RequestId string `json:"RequestId"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("sms: malformed aliyun response: %w", err)
	}
	if resp.Code != "OK" {
		return fmt.Errorf("%w: aliyun Code=%s Message=%s", ErrSmsSendFailed, resp.Code, resp.Message)
	}
	return nil
}

// 鈹€鈹€ 鑵捐浜戠煭淇★紙TC3-HMAC-SHA256 绛惧悕锛夆攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func (s *HTTPSmsSender) sendTencent(ctx context.Context, cfg *SmsConfig, phone, code string) error {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = DefaultSmsTencentEndpoint
	}
	host := endpoint
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		host = u.Host
	}

	// 鑵捐浜戣姹?E.164 鍙风爜锛堟棤 "+" 鍓嶇紑锛夛紝濡?8613800000000
	tel := strings.TrimPrefix(phone, "+")
	payload, err := json.Marshal(map[string]any{
		"PhoneNumberSet":   []string{tel},
		"SmsSdkAppId":     cfg.AccessKeyID, // 鑵捐浜戜晶 AppID 澶嶇敤 AccessKeyID 瀛楁浼犻€?
		"SignName":         cfg.SignName,
		"TemplateId":       cfg.TemplateID,
		"TemplateParamSet": []string{code},
	})
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	timestamp := fmt.Sprintf("%d", now.Unix())
	date := now.Format("2006-01-02")
	action := strings.ToLower("SendSms")
	hashedPayload := fmt.Sprintf("%x", sha256.Sum256(payload))
	canonicalRequest := strings.Join([]string{
		"POST", "/", "",
		"content-type:application/json; charset=utf-8",
		"host:" + host,
		"x-tc-action:" + action,
		"",
		"content-type;host;x-tc-action",
		hashedPayload,
	}, "\n")
	canonicalHash := fmt.Sprintf("%x", sha256.Sum256([]byte(canonicalRequest)))
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256", timestamp,
		date + "/sms/tc3_request", canonicalHash,
	}, "\n")

	secretDate := hmacSHA256([]byte("TC3"+cfg.AccessKeySecret), []byte(date))
	secretService := hmacSHA256(secretDate, []byte("sms"))
	secretSigning := hmacSHA256(secretService, []byte("tc3_request"))
	signature := hex.EncodeToString(hmacSHA256(secretSigning, []byte(stringToSign)))
	authorization := fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s/sms/tc3_request, SignedHeaders=content-type;host;x-tc-action, Signature=%s",
		cfg.AccessKeyID, date, signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", "SendSms")
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("X-TC-Version", "2021-01-11")
	req.Header.Set("Authorization", authorization)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSmsUnreachable, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSmsUnreachable, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: tencent endpoint status %d", ErrSmsSendFailed, resp.StatusCode)
	}
	var tr struct {
		Response struct {
			Error *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
			SendStatusSet []struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"SendStatusSet"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(data, &tr); err != nil {
		return fmt.Errorf("sms: malformed tencent response: %w", err)
	}
	if tr.Response.Error != nil {
		return fmt.Errorf("%w: tencent %s: %s", ErrSmsSendFailed,
			tr.Response.Error.Code, tr.Response.Error.Message)
	}
	for _, st := range tr.Response.SendStatusSet {
		if st.Code != "Ok" {
			return fmt.Errorf("%w: tencent send status %s: %s", ErrSmsSendFailed, st.Code, st.Message)
		}
	}
	if len(tr.Response.SendStatusSet) == 0 {
		return fmt.Errorf("%w: tencent empty SendStatusSet", ErrSmsSendFailed)
	}
	return nil
}

// CustomSmsContract 鑷畾涔夌煭淇＄鐐瑰绾︼細
//
//	POST {endpoint}
//	Content-Type: application/json
//	{"access_key_id": "...", "access_key_secret": "...", "phone": "...",
//	 "code": "...", "sign_name": "...", "template_id": "..."}
//
//	鈫?HTTP 200 涓?{"success": true} 瑙嗕负鍙戦€佹垚鍔燂紱鍏朵綑涓€寰嬫嫆缁濄€?
//
// 璇ュ绾﹁冻浠ユ帴鍏ヤ换鎰忚嚜寤?绗笁鏂圭煭淇℃湇鍔★紙缃戝叧渚у仛閫傞厤灞傚嵆鍙級銆?
func (s *HTTPSmsSender) sendCustom(ctx context.Context, cfg *SmsConfig, phone, code string) error {
	if cfg.Endpoint == "" {
		return errors.New("sms: custom provider requires endpoint")
	}
	payload, err := json.Marshal(map[string]string{
		"access_key_id":     cfg.AccessKeyID,
		"access_key_secret": cfg.AccessKeySecret,
		"phone":             phone,
		"code":              code,
		"sign_name":         cfg.SignName,
		"template_id":       cfg.TemplateID,
	})
	if err != nil {
		return err
	}
	body, status, err := s.postStatus(ctx, cfg.Endpoint, "application/json", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSmsUnreachable, err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("%w: custom endpoint status %d", ErrSmsSendFailed, status)
	}
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("sms: malformed custom response: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("%w: %s", ErrSmsSendFailed, resp.Message)
	}
	return nil
}

func (s *HTTPSmsSender) post(ctx context.Context, endpoint, contentType string, body io.Reader) ([]byte, error) {
	data, _, err := s.postStatus(ctx, endpoint, contentType, body)
	return data, err
}

func (s *HTTPSmsSender) postStatus(ctx context.Context, endpoint, contentType string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := s.client.Do(req)
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

// 鈹€鈹€ 鍙风爜涓庨獙璇佺爜宸ュ叿锛堢函鍑芥暟锛屼究浜庡崟娴嬶級鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// ValidateSmsPhone 鏍￠獙鎵嬫満鍙凤細鍙€?"+" 鍓嶇紑 + 5-20 浣嶆暟瀛椼€?
// 褰掍竴鍖栦粎鍘婚櫎棣栧熬绌虹櫧锛涗笉鍋氬浗瀹剁爜鎺ㄦ柇锛堝悇鏈嶅姟鍟嗘牸寮忚嚜瀹氾級銆?
func ValidateSmsPhone(phone string) bool {
	p := strings.TrimSpace(phone)
	if p == "" || len(p) > 21 {
		return false
	}
	if p[0] == '+' {
		p = p[1:]
	}
	if len(p) < 5 || len(p) > 20 {
		return false
	}
	for _, c := range p {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// NormalizeSmsPhone 鍘荤┖鐧藉悗杩斿洖鍘熸牱锛堜繚鐣?"+"锛夛紝渚涘瓨鍌?鏌ヨ缁熶竴褰㈡€併€?
func NormalizeSmsPhone(phone string) string {
	return strings.TrimSpace(phone)
}

// GenerateSmsCode 鐢熸垚 n 浣嶇函鏁板瓧楠岃瘉鐮侊紙crypto/rand锛岄浣嶅彲涓?0锛夈€?
func GenerateSmsCode(n int) (string, error) {
	if n < 4 || n > 8 {
		return "", errors.New("sms: code length must be 4-8")
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = '0' + b%10
	}
	return string(out), nil
}
