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

// WechatClient 瀵规帴寰俊鏀粯 APIv3锛圢ative 鎵爜锛夈€?// 浣跨敤瀹樻柟 SDK锛氳嚜鍔ㄥ畬鎴愬钩鍙拌瘉涔︿笅杞?鏇存柊銆佽姹傜鍚嶄笌鍥炶皟楠岀/AES-GCM 瑙ｅ瘑銆?type WechatClient struct {
	mchID   string
	appID   string
	client  *core.Client
	svc     *native.NativeApiService
	handler *notify.Handler
}

// NewWechatClient 鏋勯€犲井淇℃敮浠樺鎴风銆?// mchPrivateKeyPEM 涓哄晢鎴?API 璇佷功绉侀挜锛圥EM锛夈€?func NewWechatClient(mchID, appID, apiV3Key, mchCertSerialNo, mchPrivateKeyPEM string) (*WechatClient, error) {
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

	// 鍥炶皟澶勭悊鍣細骞冲彴璇佷功楠岀 + AES-GCM 瑙ｅ瘑锛圓utoAuth 宸插皢璇佷功涓嬭浇鍣ㄦ敞鍐屽埌鍗曚緥锛?	handler, err := notify.NewRSANotifyHandler(apiV3Key, verifiers.NewSHA256WithRSAVerifier(
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

// Precreate 寰俊 Native 棰勪笅鍗曪紝杩斿洖浜岀淮鐮佸唴瀹癸紙code_url锛夈€?func (c *WechatClient) Precreate(ctx context.Context, outTradeNo string, amountCents int64, description, notifyURL string) (string, error) {
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

// ParseCallback 瑙ｆ瀽骞堕獙绛惧井淇℃敮浠樺洖璋冦€?// 杩斿洖 (outTradeNo, transactionId, paid, amountCents, err)锛沘mountCents 涓哄洖璋冭鍗曢噾棰濓紙鍒嗭級锛?// 鐢辫皟鐢ㄦ柟涓庡唴閮ㄨ鍗曟瘮瀵癸紙闃茬鏀癸級銆?func (c *WechatClient) ParseCallback(r *http.Request) (string, string, bool, *int64, error) {
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

// Query 鎸夊晢鎴疯鍗曞彿鏌ヨ鏀粯鐘舵€侊紝杩斿洖 (tradeNo, paid, err)銆?func (c *WechatClient) Query(ctx context.Context, outTradeNo string) (string, bool, error) {
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
