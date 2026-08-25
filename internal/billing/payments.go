package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/athenavi/chiron/internal/id"
)

// 鏀粯娓犻亾
const (
	ChannelAlipay = "alipay" // 鏀粯瀹濆綋闈粯锛圢ative 鎵爜锛?	ChannelWechat = "wechat" // 寰俊 Native 鎵爜
	ChannelPayPal = "paypal" // PayPal锛堜繚鐣欙級
)

// 璁㈠崟鐘舵€?const (
	PayStatusPending  = "pending"  // 宸插垱寤猴紝绛夊緟鏀粯
	PayStatusPaid     = "paid"     // 鏀粯鎴愬姛涓斿凡鍏ヨ处
	PayStatusFailed   = "failed"   // 鏀粯澶辫触
	PayStatusExpired  = "expired"  // 瓒呮椂鏈敮浠?)

// Payment 鏄竴绗斿厖鍊艰鍗曪紙鏀粯瀹?寰俊/PayPal 鍏辩敤锛夈€?// 閲戦浠?鍒?涓哄崟浣嶏細鏀粯瀹?寰俊涓轰汉姘戝竵鍒嗭紙CNY锛夛紝PayPal 涓虹編鍏冨垎锛圲SD锛夈€?type Payment struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	Channel         string     `json:"channel"`
	Credits         int        `json:"credits"`
	AmountCents     int64      `json:"amount_cents"`
	Currency        string     `json:"currency"`
	Status          string     `json:"status"`
	QRCode          string     `json:"qr_code,omitempty"`           // 鎵爜鏀粯浜岀淮鐮佸唴瀹?	ProviderOrderID string     `json:"provider_order_id,omitempty"` // 娓犻亾渚ц鍗曞彿锛坥ut_trade_no / paypal order id锛?	TradeNo         string     `json:"trade_no,omitempty"`          // 娓犻亾浜ゆ槗娴佹按鍙凤紙鏀粯鎴愬姛鍚庡洖濉級
	CreatedAt       time.Time  `json:"created_at"`
	PaidAt          *time.Time `json:"paid_at,omitempty"`
	ExpiredAt       *time.Time `json:"expired_at,omitempty"`
}

// PaymentStore 鏄敮浠樿鍗曠殑鎸佷箙鍖栨帴鍙ｃ€?type PaymentStore interface {
	CreatePayment(ctx context.Context, p *Payment) error
	GetPayment(ctx context.Context, id string) (*Payment, error)
	// GetPaymentByProviderOrderID 渚涙笭閬撳洖璋冩寜绗笁鏂硅鍗曞彿瀹氫綅锛堟敮浠樺疂鍥炶皟鏃犲唴閮ㄨ鍗曞彿鏃跺彲鍥炴煡锛?	GetPaymentByProviderOrderID(ctx context.Context, providerOrderID string) (*Payment, error)
	// MarkPaymentPaid 骞傜瓑鎺ㄨ繘 pending鈫抪aid锛涜嫢璁㈠崟闈?pending 杩斿洖 (nil, nil) 琛ㄧず宸插鐞嗐€?	// 鎴愬姛杩斿洖璁㈠崟锛堝惈 credits/user_id锛屼緵鍏ヨ处锛夈€?	MarkPaymentPaid(ctx context.Context, id, tradeNo string) (*Payment, error)
	MarkPaymentFailed(ctx context.Context, id string) error
	// UpdatePaymentProvider 棰勪笅鍗曟垚鍔熷悗鍥炲～浜岀淮鐮佷笌娓犻亾璁㈠崟鍙枫€?	UpdatePaymentProvider(ctx context.Context, id, qrCode, providerOrderID string) error
}

// NewPayment 鍒涘缓涓€绗?pending 璁㈠崟銆?func NewPayment(userID, channel string, credits int, amountCents int64, currency string) *Payment {
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

// ConfirmPayment 骞傜瓑纭涓€绗旀敮浠樻垚鍔熷苟鍏ヨ处锛?//  1. 璁㈠崟 pending鈫抪aid 鍘熷瓙鎺ㄨ繘锛堥噸澶嶅洖璋?杞鍙叆璐︿竴娆★級锛?//  2. 鎺ㄨ繘鎴愬姛鍚庡悜鐢ㄦ埛鍔?credits锛坮eason = "<channel>_topup"锛夈€?//
// 杩斿洖 (payment, credited, err)锛歝redited=false 琛ㄧず璁㈠崟姝ゅ墠宸插叆璐︼紙骞傜瓑鍛戒腑锛夈€?func (m *Manager) ConfirmPayment(ctx context.Context, id, tradeNo string) (*Payment, bool, error) {
	p, err := m.store.MarkPaymentPaid(ctx, id, tradeNo)
	if err != nil {
		return nil, false, fmt.Errorf("mark payment paid: %w", err)
	}
	if p == nil {
		// 宸插鐞嗭紙骞傜瓑锛夆€斺€斿洖鏌ュ綋鍓嶇姸鎬佺‘璁?		cur, err := m.store.GetPayment(ctx, id)
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
