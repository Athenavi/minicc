package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/athenavi/minicc/internal/id"
)

// 支付渠道
const (
	ChannelAlipay = "alipay" // 支付宝当面付（Native 扫码）
	ChannelWechat = "wechat" // 微信 Native 扫码
	ChannelPayPal = "paypal" // PayPal（保留）
)

// 订单状态
const (
	PayStatusPending  = "pending"  // 已创建，等待支付
	PayStatusPaid     = "paid"     // 支付成功且已入账
	PayStatusFailed   = "failed"   // 支付失败
	PayStatusExpired  = "expired"  // 超时未支付
)

// Payment 是一笔充值订单（支付宝/微信/PayPal 共用）。
// 金额以"分"为单位：支付宝/微信为人民币分（CNY），PayPal 为美元分（USD）。
type Payment struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	Channel         string     `json:"channel"`
	Credits         int        `json:"credits"`
	AmountCents     int64      `json:"amount_cents"`
	Currency        string     `json:"currency"`
	Status          string     `json:"status"`
	QRCode          string     `json:"qr_code,omitempty"`           // 扫码支付二维码内容
	ProviderOrderID string     `json:"provider_order_id,omitempty"` // 渠道侧订单号（out_trade_no / paypal order id）
	TradeNo         string     `json:"trade_no,omitempty"`          // 渠道交易流水号（支付成功后回填）
	CreatedAt       time.Time  `json:"created_at"`
	PaidAt          *time.Time `json:"paid_at,omitempty"`
	ExpiredAt       *time.Time `json:"expired_at,omitempty"`
}

// PaymentStore 是支付订单的持久化接口。
type PaymentStore interface {
	CreatePayment(ctx context.Context, p *Payment) error
	GetPayment(ctx context.Context, id string) (*Payment, error)
	// GetPaymentByProviderOrderID 供渠道回调按第三方订单号定位（支付宝回调无内部订单号时可回查）
	GetPaymentByProviderOrderID(ctx context.Context, providerOrderID string) (*Payment, error)
	// MarkPaymentPaid 幂等推进 pending→paid；若订单非 pending 返回 (nil, nil) 表示已处理。
	// 成功返回订单（含 credits/user_id，供入账）。
	MarkPaymentPaid(ctx context.Context, id, tradeNo string) (*Payment, error)
	MarkPaymentFailed(ctx context.Context, id string) error
	// UpdatePaymentProvider 预下单成功后回填二维码与渠道订单号。
	UpdatePaymentProvider(ctx context.Context, id, qrCode, providerOrderID string) error
}

// NewPayment 创建一笔 pending 订单。
func NewPayment(userID, channel string, credits int, amountCents int64, currency string) *Payment {
	now := time.Now()
	exp := now.Add(2 * time.Hour)
	return &Payment{
		ID:          "pay_" + id.NextID(),
		UserID:      userID,
		Channel:     channel,
		Credits:     credits,
		AmountCents: amountCents,
		Currency:    currency,
		Status:      PayStatusPending,
		CreatedAt:   now,
		ExpiredAt:   &exp,
	}
}

// ConfirmPayment 幂等确认一笔支付成功并入账：
//  1. 订单 pending→paid 原子推进（重复回调/轮询只入账一次）；
//  2. 推进成功后向用户加 credits（reason = "<channel>_topup"）。
//
// 返回 (payment, credited, err)：credited=false 表示订单此前已入账（幂等命中）。
func (m *Manager) ConfirmPayment(ctx context.Context, id, tradeNo string) (*Payment, bool, error) {
	p, err := m.store.MarkPaymentPaid(ctx, id, tradeNo)
	if err != nil {
		return nil, false, fmt.Errorf("mark payment paid: %w", err)
	}
	if p == nil {
		// 已处理（幂等）——回查当前状态确认
		cur, err := m.store.GetPayment(ctx, id)
		if err != nil {
			return nil, false, fmt.Errorf("get payment: %w", err)
		}
		return cur, false, nil
	}

	reason := p.Channel + "_topup"
	if _, err := m.AddCredits(p.UserID, reason, p.Credits); err != nil {
		return p, false, fmt.Errorf("credit after payment: %w", err)
	}
	return p, true, nil
}
