package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
)

// 鈹€鈹€ AuditSink锛歊edis Stream 鈫?PG audit_logs 鎵归噺钀藉簱 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
//
// 浣滀负 AuditConsumer 鐨?handler 鍥炶皟浣跨敤锛?
//
//	sink := db.NewDefaultAuditSink()
//	consumer := db.NewAuditConsumer(redis, sink.Handle)
//	go consumer.Start(ctx)
//
// 鎵归噺绛栫暐锛氣墺100 鏉℃垨 1s 绐楀彛鍏堝埌鍏堝彂銆?
// 澶辫触绛栫暐锛氭壒閲?INSERT 澶辫触浠?slog.Error 骞朵涪寮冭鎵癸紙Handle 鎭掕繑鍥?nil锛?
// 娑堣垂鑰呯収甯?ACK锛夆€斺€斿畞鍙涪涓嶅彲鍫碉紝閬垮厤姣掓秷鎭湪娑堣垂缁勫唴鏃犻檺閲嶆姇锛?
// 鍏抽敭浜嬩欢锛堝鐧诲綍瀹¤锛夊凡鏈?handler 鏄惧紡璁板綍鍏滃簳銆?

var auditSinkUUIDRe = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// AuditSink 瀹¤鏃ュ織鎵归噺钀藉簱 sink銆?
type AuditSink struct {
	mu        sync.Mutex
	buf       []AuditEntry
	writer    func(ctx context.Context, entries []AuditEntry) error
	batchSize int
	window    time.Duration
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// NewAuditSink 鍒涘缓 sink 骞跺惎鍔ㄧ獥鍙ｅ畾鏃?flush 鍗忕▼銆?
// batchSize<=0 鈫?100锛泈indow<=0 鈫?1s銆?
func NewAuditSink(writer func(ctx context.Context, entries []AuditEntry) error,
	batchSize int, window time.Duration) *AuditSink {
	if batchSize <= 0 {
		batchSize = 100
	}
	if window <= 0 {
		window = time.Second
	}
	s := &AuditSink{
		writer:    writer,
		batchSize: batchSize,
		window:    window,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	go s.loop()
	return s
}

// NewDefaultAuditSink 鍒涘缓榛樿 sink锛圥G 鎵归噺鍐欏叆锛?00 鏉?/ 1s 绐楀彛锛夈€?
func NewDefaultAuditSink() *AuditSink {
	return NewAuditSink(WriteAuditLogs, 100, time.Second)
}

// Handle 瀹炵幇 AuditConsumer 鐨?handler 鍥炶皟绛惧悕銆?
// 鎭掕繑鍥?nil锛氬啓鍏ュけ璐ュ湪 Flush 鍐呰鏃ュ織涓㈠純锛屼繚璇佹秷璐硅€?ACK锛屼笉鍫靛娑堣垂缁勩€?
func (s *AuditSink) Handle(ctx context.Context, entry AuditEntry) error {
	s.mu.Lock()
	s.buf = append(s.buf, entry)
	full := len(s.buf) >= s.batchSize
	s.mu.Unlock()
	if full {
		s.Flush(ctx)
	}
	return nil
}

// loop 鎸夌獥鍙ｅ懆鏈熻Е鍙?flush锛孋lose 鏃舵敹灏俱€?
func (s *AuditSink) loop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(s.window)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.Flush(context.Background())
		case <-s.stopCh:
			s.Flush(context.Background())
			return
		}
	}
}

// Flush 鍙栧嚭褰撳墠缂撳啿骞惰皟鐢?writer 钀藉簱锛涘け璐?slog.Error 骞朵涪寮冭鎵广€?
func (s *AuditSink) Flush(ctx context.Context) {
	s.mu.Lock()
	if len(s.buf) == 0 {
		s.mu.Unlock()
		return
	}
	batch := s.buf
	s.buf = nil
	s.mu.Unlock()

	if s.writer == nil {
		return
	}
	if err := s.writer(ctx, batch); err != nil {
		slog.Error("audit sink: batch insert failed, dropping batch",
			"count", len(batch), "error", err)
	}
}

// Close 鍋滄绐楀彛鍗忕▼骞?flush 娈嬬暀缂撳啿锛堥樆濉炶嚦瀹屾垚锛夈€?
func (s *AuditSink) Close() {
	close(s.stopCh)
	<-s.doneCh
}

// 鈹€鈹€ 榛樿 PG 鎵归噺鍐欏叆 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// WriteAuditLogs 灏嗗璁℃潯鐩壒閲?INSERT 鍒?audit_logs銆?
// 瀛楁鏄犲皠锛歵enant_id 闈炴硶/绌?鈫?榛樿绉熸埛锛泆ser_id 闈炴硶/绌?鈫?NULL锛?
// resource_type 鎸?resource 璺緞鍓嶇紑褰掔被锛坋nt/admin/api/general锛夈€?
func WriteAuditLogs(ctx context.Context, entries []AuditEntry) error {
	pool := Pool
	if pool == nil {
		return errors.New("audit sink: postgres pool unavailable")
	}
	if len(entries) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(`INSERT INTO audit_logs
		(tenant_id, user_id, action, resource_type, resource_id, details, ip_address) VALUES `)
	args := make([]any, 0, len(entries)*7)
	idx := 0
	next := func() string { idx++; return fmt.Sprintf("$%d", idx) }

	for i, e := range entries {
		tenantID := e.TenantID
		if !auditSinkUUIDRe.MatchString(tenantID) {
			tenantID = DefaultTenantID
		}
		var userID any
		if auditSinkUUIDRe.MatchString(e.UserID) {
			userID = e.UserID
		}
		details, _ := json.Marshal(struct {
			Detail  string `json:"detail"`
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}{
			Detail:  e.Detail,
			Success: e.Success,
			Error:   e.Error,
		})
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("(" + next() + ", " + next() + ", " + next() + ", " +
			next() + ", " + next() + ", " + next() + ", " + next() + ")")
		args = append(args, tenantID, userID, clipVarchar(e.Action, 64),
			auditResourceType(e.Resource), clipVarchar(e.Resource, 64),
			string(details), nilableIP(e.IP))
	}

	_, err := pool.Exec(ctx, sb.String(), args...)
	return err
}

// auditResourceType 鎸夎祫婧愯矾寰勫墠缂€褰掔被 resource_type锛坴archar(64)锛夈€?
func auditResourceType(resource string) string {
	switch {
	case resource == "":
		return "general"
	case strings.HasPrefix(resource, "/v1/ent/") || strings.HasPrefix(resource, "/ent/"):
		return "ent"
	case strings.HasPrefix(resource, "/admin") || strings.HasPrefix(resource, "/v1/admin"):
		return "admin"
	case strings.HasPrefix(resource, "/"):
		return "api"
	default:
		return "general"
	}
}

// clipVarchar 鎴柇鍒?varchar(n) 涓婇檺锛岄伩鍏嶈惤搴撹秴闀挎姤閿欍€?
func clipVarchar(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// nilableIP 绌?IP 钀?NULL锛坕p_address 鍒楀彲绌猴級銆?
func nilableIP(ip string) any {
	if ip == "" {
		return nil
	}
	return clipVarchar(ip, 45)
}
