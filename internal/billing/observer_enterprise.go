package billing

import (
	"context"
	"log/slog"
	"time"

	"github.com/athenavi/chiron/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnterpriseBillingObserver 鏄妸娑堣垂浜嬩欢褰掗泦鍒?billing_records 鐨?
// BillingObserver锛堜紒涓氭垚鏈腑蹇冿級銆傚啓鍏ュ畬鍏ㄥ紓姝ワ紙goroutine + recover锛夛紝
// 浠讳綍澶辫触浠?slog.Warn锛岀粷涓嶉樆濉炴垨褰卞搷涓昏璐归摼璺€?
//
// 鎺ョ嚎浣嶇疆锛堥泦鎴愪换鍔★級锛歡ateway_router.go NewGatewayRouter 涓?
// billingMgr.Subscribe 澶勶紙鐜版湁 TransactionRecorder / BalanceSyncer 鏃侊級锛?
//
//	billingMgr.Subscribe(billing.NewEnterpriseBillingObserver(db.Pool))
type EnterpriseBillingObserver struct {
	pool *pgxpool.Pool // nil = 浜嬩欢鏃跺洖閫€鍒板叏灞€ db.Pool
}

var _ BillingObserver = (*EnterpriseBillingObserver)(nil)

// NewEnterpriseBillingObserver 鍒涘缓浼佷笟璁¤垂瑙傚療鑰呫€?
// pool 涓?nil 鏃朵簨浠跺鐞嗗洖閫€鍒板叏灞€ db.Pool锛堟部鐢ㄩ」鐩?PGStore 妯″紡锛夈€?
func NewEnterpriseBillingObserver(pool *pgxpool.Pool) *EnterpriseBillingObserver {
	return &EnterpriseBillingObserver{pool: pool}
}

// OnCreditChange 寮傛鍐欎竴鏉?billing_records 褰掗泦璁板綍銆?
// 浠呭綊闆嗘秷璐癸紙鎵ｅ噺锛変簨浠讹紱鍏呭€?閫€娆剧瓑鍏ヨ处鐢?payments / credit_transactions 瑕嗙洊銆?
func (o *EnterpriseBillingObserver) OnCreditChange(evt CreditEvent) {
	if evt.Amount >= 0 {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("enterprise billing observer panic", "panic", r, "user_id", evt.UserID)
			}
		}()
		o.record(evt)
	}()
}

// record 鎵ц涓€娆″綊闆嗗啓鍏ワ細tenant_id 缁?users 琛ㄥ叧鑱旓紝
// group_id 鍙栫敤鎴蜂富缇ょ粍锛坋nt_group_members 绗竴鏉★級锛屾棤缇ょ粍鍒?NULL銆?
func (o *EnterpriseBillingObserver) record(evt CreditEvent) {
	pool := o.pool
	if pool == nil {
		pool = db.Pool
	}
	if pool == nil {
		slog.Warn("enterprise billing record skipped: no postgres pool", "user_id", evt.UserID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// tenant_id锛氱粡 users 琛ㄥ叧鑱旀煡璇紙users.id 鈫?users.tenant_id锛?
	var tenantID string
	if err := pool.QueryRow(ctx,
		`SELECT tenant_id FROM users WHERE id = $1`, evt.UserID).Scan(&tenantID); err != nil {
		slog.Warn("enterprise billing record skipped: tenant lookup failed",
			"user_id", evt.UserID, "error", err)
		return
	}

	// group_id锛氱敤鎴蜂富缇ょ粍锛坋nt_group_members 绗竴鏉★紝鎸?group_id 鎺掑簭淇濊瘉纭畾鎬э級銆?
	// 鏃犳垚鍛樺叧绯绘垨鍏跺畠鏌ヨ閿欒鍧囬檷绾т负 NULL 褰掗泦锛屼笉褰卞搷鎴愭湰钀借处銆?
	var groupID *string
	_ = pool.QueryRow(ctx,
		`SELECT group_id FROM ent_group_members WHERE user_id = $1 ORDER BY group_id LIMIT 1`,
		evt.UserID).Scan(&groupID)

	// 1 credit = 1 鍒嗭紙涓?payments.amount_cents 鍙ｅ緞涓€鑷达級
	costCents := -evt.Amount
	// 娉ㄦ剰锛欳reditEvent 涓嶆惡甯?token 鏄庣粏锛圡anager 浜嬩欢浠呭惈閲戦锛夛紝
	// input/output_tokens 璁?0锛屾垚鏈互 cost_cents 涓哄噯銆?
	if _, err := pool.Exec(ctx,
		`INSERT INTO billing_records
			(tenant_id, user_id, session_id, input_tokens, output_tokens, cost_cents, group_id)
		 VALUES ($1, $2, NULL, 0, 0, $3, $4)`,
		tenantID, evt.UserID, costCents, groupID); err != nil {
		slog.Warn("enterprise billing record insert failed",
			"user_id", evt.UserID, "tenant_id", tenantID, "error", err)
	}
}
