package db

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// AuditEntry 瀹¤鏃ュ織鏉＄洰
type AuditEntry struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Action    string    `json:"action"`    // "session.create", "tool.execute", "agent.run"
	Resource  string    `json:"resource"`  // 璧勬簮鏍囪瘑
	Detail    string    `json:"detail"`    // 璇︾粏淇℃伅
	IP        string    `json:"ip"`        // 瀹㈡埛绔?IP
	Success   bool      `json:"success"`   // 鏄惁鎴愬姛
	Error     string    `json:"error"`     // 閿欒淇℃伅
	Timestamp time.Time `json:"timestamp"`
}

// Auditor 瀹¤鏃ュ織璁板綍鍣?type Auditor struct {
	rdb      RedisClient
	stream   string
}

// NewAuditor 鍒涘缓瀹¤鏃ュ織璁板綍鍣?func NewAuditor(rdb RedisClient) *Auditor {
	return &Auditor{
		rdb:    rdb,
		stream: "audit:events",
	}
}

// Log 璁板綍瀹¤浜嬩欢鍒?Redis Streams
func (a *Auditor) Log(ctx context.Context, entry AuditEntry) {
	if a.rdb == nil {
		slog.Debug("瀹¤鏃ュ織璺宠繃锛圧edis 涓嶅彲鐢級", "action", entry.Action)
		return
	}

	entry.Timestamp = time.Now()
	data, err := json.Marshal(entry)
	if err != nil {
		slog.Error("瀹¤鏃ュ織搴忓垪鍖栧け璐?, "error", err)
		return
	}

	_, err = a.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: a.stream,
		MaxLen: 100000, // 鏈€澶氫繚鐣?10 涓囨潯
		Approx: true,
		Values: map[string]any{
			"tenant_id": entry.TenantID,
			"user_id":   entry.UserID,
			"action":    entry.Action,
			"resource":  entry.Resource,
			"success":   entry.Success,
			"data":      string(data),
		},
	}).Result()

	if err != nil {
		slog.Error("瀹¤鏃ュ織鍐欏叆澶辫触", "error", err, "action", entry.Action)
		return
	}

	slog.Debug("瀹¤鏃ュ織宸茶褰?,
		"action", entry.Action,
		"user", entry.UserID,
		"tenant", entry.TenantID,
		"success", entry.Success,
	)
}

// UserExtractor 浠庤姹備腑鎻愬彇鐢ㄦ埛淇℃伅鐨勫嚱鏁扮被鍨?// 杩斿洖 (userID, tenantID)
type UserExtractor func(r *http.Request) (string, string)

// LogAuditMiddleware 瀹¤鏃ュ織涓棿浠?// extractUser: 浠庤姹備腑鎻愬彇鐢ㄦ埛淇℃伅鐨勫嚱鏁帮紝鍙互涓?nil
func LogAuditMiddleware(auditor *Auditor, extractUser UserExtractor) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 鍖呰 ResponseWriter 浠ユ崟鑾风姸鎬佺爜
			ww := &auditResponseWriter{ResponseWriter: w, statusCode: 200}

			next.ServeHTTP(ww, r)

			// 璁板綍瀹¤鏃ュ織
			entry := AuditEntry{
				Action:   r.Method + " " + r.URL.Path,
				Resource: r.URL.String(),
				IP:       r.RemoteAddr,
				Success:  ww.statusCode < 400,
			}

			// 鎻愬彇鐢ㄦ埛淇℃伅
			if extractUser != nil {
				entry.UserID, entry.TenantID = extractUser(r)
			}

			auditor.Log(r.Context(), entry)
		})
	}
}

// auditResponseWriter 鍖呰 ResponseWriter 浠ユ崟鑾风姸鎬佺爜
type auditResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *auditResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// AuditLog 鍏ㄥ眬瀹¤鏃ュ織鍑芥暟锛堢畝鍖栨帴鍙ｏ級
// 鐢ㄤ簬 middleware.go 涓殑蹇€熻皟鐢?func AuditLog(ctx context.Context, userID, action, resource, detail, ip string, meta map[string]interface{}) {
	if Redis == nil {
		return
	}

	entry := AuditEntry{
		UserID:   userID,
		Action:   action,
		Resource: resource,
		Detail:   detail,
		IP:       ip,
		Success:  true,
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	if err := Redis.XAdd(ctx, &redis.XAddArgs{
		Stream: "audit:events",
		MaxLen: 100000,
		Approx: true,
		Values: map[string]any{
			"user_id":  userID,
			"action":   action,
			"resource": resource,
			"data":     string(data),
		},
	}).Err(); err != nil {
		slog.Warn("audit log: XAdd failed", "error", err)
	}
}

// AuditConsumer 瀹¤鏃ュ織娑堣垂鑰?type AuditConsumer struct {
	rdb      RedisClient
	stream   string
	group    string
	handler  func(ctx context.Context, entry AuditEntry) error
}

// NewAuditConsumer 鍒涘缓瀹¤鏃ュ織娑堣垂鑰?func NewAuditConsumer(rdb RedisClient, handler func(ctx context.Context, entry AuditEntry) error) *AuditConsumer {
	return &AuditConsumer{
		rdb:     rdb,
		stream:  "audit:events",
		group:   "audit-processor",
		handler: handler,
	}
}

// Start 鍚姩娑堣垂鑰?func (c *AuditConsumer) Start(ctx context.Context) error {
	if c.rdb == nil {
		return nil
	}

	// 鍒涘缓娑堣垂鑰呯粍
	if err := c.rdb.XGroupCreateMkStream(ctx, c.stream, c.group, "0").Err(); err != nil {
		if !strings.Contains(err.Error(), "BUSYGROUP") {
			slog.Warn("audit consumer group create error", "error", err)
		}
	}

	consumerID := "audit-worker-" + time.Now().Format("20060102150405")

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		results, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: consumerID,
			Streams:  []string{c.stream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		if err != nil {
			if err == redis.Nil {
				continue
			}
			slog.Error("瀹¤鏃ュ織娑堣垂澶辫触", "error", err)
			time.Sleep(time.Second)
			continue
		}

		for _, result := range results {
			for _, msg := range result.Messages {
				data, ok := msg.Values["data"].(string)
				if !ok {
					continue
				}

				var entry AuditEntry
				if err := json.Unmarshal([]byte(data), &entry); err != nil {
					slog.Error("瀹¤鏃ュ織鍙嶅簭鍒楀寲澶辫触", "error", err)
					continue
				}

				if err := c.handler(ctx, entry); err != nil {
					slog.Error("瀹¤鏃ュ織澶勭悊澶辫触", "error", err, "action", entry.Action)
					continue
				}

				// 纭娑堟伅
				c.rdb.XAck(ctx, c.stream, c.group, msg.ID)
			}
		}
	}
}
