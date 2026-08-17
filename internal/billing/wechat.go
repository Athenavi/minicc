package billing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

// WechatClient 对接微信支付 APIv3（Native 扫码）。
// 使用官方 SDK：自动完成平台证书下载/更新、请求签名与回调验签/AES-GCM 解密。
type WechatClient struct {
	mchID   string
	appID   string
	client  *core.Client
	svc     *native.NativeApiService
	handler *notify.Handler
}

// NewWechatClient 构造微信支付客户端。
// mchPrivateKeyPEM 为商户 API 证书私钥（PEM）。
func NewWechatClient(mchID, appID, apiV3Key, mchCertSerialNo, mchPrivateKeyPEM string) (*WechatClient, error) {
	mchPrivateKey, err := utils.LoadPrivateKey(mchPrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("load wechat mch private key: %w", err)
	}
	ctx := context.Background()
	opts := []core.ClientOption{
		option.WithWechatPayAutoAuthCipher(mchID, mchCertSerialNo, mchPrivateKey, apiV3Key),
	}
	client, err := core.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("new wechat client: %w", err)
	}

	// 回调处理器：平台证书验签 + AES-GCM 解密（AutoAuth 已将证书下载器注册到单例）
	handler, err := notify.NewRSANotifyHandler(apiV3Key, verifiers.NewSHA256WithRSAVerifier(
		downloader.MgrInstance().GetCertificateVisitor(mchID)))
	if err != nil {
		return nil, fmt.Errorf("new wechat notify handler: %w", err)
	}

	return &WechatClient{
		mchID:   mchID,
		appID:   appID,
		client:  client,
		svc:     &native.NativeApiService{Client: client},
		handler: handler,
	}, nil
}

// Precreate 微信 Native 预下单，返回二维码内容（code_url）。
func (c *WechatClient) Precreate(ctx context.Context, outTradeNo string, amountCents int64, description, notifyURL string) (string, error) {
	resp, result, err := c.svc.Prepay(ctx, native.PrepayRequest{
		Appid:       core.String(c.appID),
		Mchid:       core.String(c.mchID),
		Description: core.String(description),
		OutTradeNo:  core.String(outTradeNo),
		NotifyUrl:   core.String(notifyURL),
		TimeExpire:  core.Time(time.Now().Add(2 * time.Hour)),
		Amount: &native.Amount{
			Total:    core.Int64(amountCents),
			Currency: core.String("CNY"),
		},
	})
	if err != nil {
		return "", fmt.Errorf("wechat precreate: %w", err)
	}
	if result != nil && result.Response.StatusCode >= 400 {
		return "", fmt.Errorf("wechat precreate http %d", result.Response.StatusCode)
	}
	if resp == nil || resp.CodeUrl == nil || *resp.CodeUrl == "" {
		return "", fmt.Errorf("wechat precreate returned empty code_url")
	}
	return *resp.CodeUrl, nil
}

// ParseCallback 解析并验签微信支付回调。
// 返回 (outTradeNo, transactionId, paid, amountCents, err)；amountCents 为回调订单金额（分），
// 由调用方与内部订单比对（防篡改）。
func (c *WechatClient) ParseCallback(r *http.Request) (string, string, bool, *int64, error) {
	var tx payments.Transaction
	if _, err := c.handler.ParseNotifyRequest(r.Context(), r, &tx); err != nil {
		return "", "", false, nil, fmt.Errorf("wechat notify parse: %w", err)
	}
	if tx.OutTradeNo == nil {
		return "", "", false, nil, fmt.Errorf("wechat notify missing out_trade_no")
	}
	tradeNo := ""
	if tx.TransactionId != nil {
		tradeNo = *tx.TransactionId
	}
	paid := tx.TradeState != nil && (*tx.TradeState == "SUCCESS" || *tx.TradeState == "USERPAYING")
	var amount *int64
	if tx.Amount != nil && tx.Amount.Total != nil {
		amount = tx.Amount.Total
	}
	return *tx.OutTradeNo, tradeNo, paid, amount, nil
}

// Query 按商户订单号查询支付状态，返回 (tradeNo, paid, err)。
func (c *WechatClient) Query(ctx context.Context, outTradeNo string) (string, bool, error) {
	resp, _, err := c.svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(outTradeNo),
		Mchid:      core.String(c.mchID),
	})
	if err != nil {
		return "", false, fmt.Errorf("wechat query: %w", err)
	}
	if resp == nil || resp.TradeState == nil {
		return "", false, nil
	}
	tradeNo := ""
	if resp.TransactionId != nil {
		tradeNo = *resp.TransactionId
	}
	paid := *resp.TradeState == "SUCCESS"
	return tradeNo, paid, nil
}
