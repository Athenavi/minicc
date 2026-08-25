package billing

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AlipayClient 瀵规帴鏀粯瀹濆紑鏀惧钩鍙帮紙褰撻潰浠?trade.precreate + 寮傛閫氱煡楠岀锛夈€?// 鑷爺 RSA2 绛惧悕锛屼笉渚濊禆绗笁鏂?SDK銆?type AlipayClient struct {
	appID       string
	privateKey  *rsa.PrivateKey
	publicKey   *rsa.PublicKey // 鏀粯瀹濆叕閽ワ紙鐢ㄤ簬鍝嶅簲/鍥炶皟楠岀锛?	gateway     string
	notifyURL   string
	httpClient  *http.Client
}

// NewAlipayClient 鐢?PEM 绉侀挜/鍏挜鏋勯€犲鎴风銆俫ateway 涓虹┖鏃朵娇鐢ㄧ敓浜х綉鍏炽€?func NewAlipayClient(appID, privateKeyPEM, alipayPublicKeyPEM, gateway, notifyURL string) (*AlipayClient, error) {
	priv, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse alipay private key: %w", err)
	}
	pub, err := parseRSAPublicKey(alipayPublicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse alipay public key: %w", err)
	}
	if gateway == "" {
		gateway = "https://openapi.alipay.com/gateway.do"
	}
	return &AlipayClient{
		appID:      appID,
		privateKey: priv,
		publicKey:  pub,
		gateway:    gateway,
		notifyURL:  notifyURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		// 鍏煎鏃?PEM 澶寸殑瑁?base64 绉侀挜
		der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pemStr))
		if err != nil {
			return nil, fmt.Errorf("decode private key: %w", err)
		}
		block = &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
	}
	if rk, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return rk, nil
	}
	return nil, fmt.Errorf("unsupported private key format")
}

func parseRSAPublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pemStr))
		if err != nil {
			return nil, fmt.Errorf("decode public key: %w", err)
		}
		block = &pem.Block{Type: "PUBLIC KEY", Bytes: der}
	}
	if k, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PublicKey); ok {
			return rk, nil
		}
	}
	if k, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return k, nil
	}
	return nil, fmt.Errorf("unsupported public key format")
}

// buildSignContent 鎷兼帴寰呯鍚嶄覆锛氶潪绌哄弬鏁版寜 key 瀛楀吀搴忥紝key=value 鐢?& 杩炴帴銆?func buildSignContent(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	return strings.Join(parts, "&")
}

// sign 瀵瑰弬鏁板仛 RSA2锛圫HA256withRSA锛夌鍚嶏紝杩斿洖 base64銆?func (c *AlipayClient) sign(params map[string]string) (string, error) {
	content := buildSignContent(params)
	digest := sha256.Sum256([]byte(content))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// verify 鐢ㄦ敮浠樺疂鍏挜楠岀銆?func (c *AlipayClient) verify(params map[string]string, signature string) error {
	content := buildSignContent(params)
	digest := sha256.Sum256([]byte(content))
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	return rsa.VerifyPKCS1v15(c.publicKey, crypto.SHA256, digest[:], sig)
}

// Precreate 鏀粯瀹濆綋闈粯棰勪笅鍗曪紝杩斿洖浜岀淮鐮佸唴瀹逛笌娓犻亾璁㈠崟鍙枫€?func (c *AlipayClient) Precreate(ctx context.Context, outTradeNo string, amountCents int64, subject string) (qrCode string, err error) {
	biz := map[string]any{
		"out_trade_no": outTradeNo,
		"total_amount": fmt.Sprintf("%.2f", float64(amountCents)/100),
		"subject":      subject,
	}
	bizJSON, err := json.Marshal(biz)
	if err != nil {
		return "", err
	}

	params := map[string]string{
		"app_id":      c.appID,
		"method":      "alipay.trade.precreate",
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"biz_content": string(bizJSON),
	}
	if c.notifyURL != "" {
		params["notify_url"] = c.notifyURL
	}
	sign, err := c.sign(params)
	if err != nil {
		return "", fmt.Errorf("sign precreate: %w", err)
	}
	params["sign"] = sign

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.gateway, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("alipay precreate request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var r struct {
		Response struct {
			Code       string `json:"code"`
			Msg        string `json:"msg"`
			SubMsg     string `json:"sub_msg"`
			OutTradeNo string `json:"out_trade_no"`
			QRCode     string `json:"qr_code"`
		} `json:"alipay_trade_precreate_response"`
		Sign string `json:"sign"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("alipay precreate decode: %w", err)
	}
	if r.Response.Code != "10000" {
		return "", fmt.Errorf("alipay precreate failed: %s %s", r.Response.Msg, r.Response.SubMsg)
	}
	if r.Response.QRCode == "" {
		return "", fmt.Errorf("alipay precreate returned empty qr_code")
	}
	return r.Response.QRCode, nil
}

// Query 鏌ヨ璁㈠崟鏀粯鐘舵€併€傝繑鍥?(tradeNo, paid, err)銆?func (c *AlipayClient) Query(ctx context.Context, outTradeNo string) (string, bool, error) {
	biz, _ := json.Marshal(map[string]any{"out_trade_no": outTradeNo})
	params := map[string]string{
		"app_id":      c.appID,
		"method":      "alipay.trade.query",
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"biz_content": string(biz),
	}
	sign, err := c.sign(params)
	if err != nil {
		return "", false, err
	}
	params["sign"] = sign

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.gateway, strings.NewReader(form.Encode()))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("alipay query request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", false, err
	}
	var r struct {
		Response struct {
			Code       string `json:"code"`
			Msg        string `json:"msg"`
			SubMsg     string `json:"sub_msg"`
			TradeNo    string `json:"trade_no"`
			TradeState string `json:"trade_status"`
		} `json:"alipay_trade_query_response"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", false, fmt.Errorf("alipay query decode: %w", err)
	}
	paid := r.Response.TradeState == "TRADE_SUCCESS" || r.Response.TradeState == "TRADE_FINISHED"
	return r.Response.TradeNo, paid, nil
}

// VerifyCallback 鏍￠獙鏀粯瀹濆紓姝ラ€氱煡鍙傛暟锛堥獙绛?+ 浜ゆ槗鎴愬姛鐘舵€侊級銆?// 杩斿洖 (outTradeNo, tradeNo, ok)銆?func (c *AlipayClient) VerifyCallback(params map[string]string) (string, string, bool) {
	sign := params["sign"]
	if sign == "" || params["app_id"] != c.appID {
		return "", "", false
	}
	if params["trade_status"] != "TRADE_SUCCESS" && params["trade_status"] != "TRADE_FINISHED" {
		return "", "", false
	}
	if err := c.verify(params, sign); err != nil {
		return "", "", false
	}
	return params["out_trade_no"], params["trade_no"], true
}
