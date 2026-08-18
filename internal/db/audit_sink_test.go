package db

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// sinkRecorder 捕获 sink 每次 flush 的批量内容。
type sinkRecorder struct {
	mu      sync.Mutex
	batches [][]AuditEntry
	fail    bool
}

func (r *sinkRecorder) write(ctx context.Context, entries []AuditEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("pg down")
	}
	cp := make([]AuditEntry, len(entries))
	copy(cp, entries)
	r.batches = append(r.batches, cp)
	return nil
}

func (r *sinkRecorder) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, b := range r.batches {
		n += len(b)
	}
	return n
}

func testEntry(i int) AuditEntry {
	return AuditEntry{ID: "e", Action: "test", UserID: "u", Timestamp: time.Now()}
}

// TestAuditSink_BatchTrigger 达到 batchSize（100 条）立即触发 flush。
func TestAuditSink_BatchTrigger(t *testing.T) {
	rec := &sinkRecorder{}
	// 窗口设长（10s），确保触发只来自批量阈值
	sink := NewAuditSink(rec.write, 100, 10*time.Second)
	defer sink.Close()

	for i := 0; i < 99; i++ {
		if err := sink.Handle(context.Background(), testEntry(i)); err != nil {
			t.Fatalf("Handle should never error, got %v", err)
		}
	}
	if rec.total() != 0 {
		t.Fatalf("expected no flush before batch full, got %d entries", rec.total())
	}

	if err := sink.Handle(context.Background(), testEntry(99)); err != nil {
		t.Fatalf("Handle should never error, got %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.batches) != 1 || len(rec.batches[0]) != 100 {
		t.Fatalf("expected one flush of exactly 100 entries, got %d batches", len(rec.batches))
	}
}

// TestAuditSink_WindowTrigger 未达批量阈值时，1s 窗口到期触发 flush。
func TestAuditSink_WindowTrigger(t *testing.T) {
	rec := &sinkRecorder{}
	sink := NewAuditSink(rec.write, 100, 50*time.Millisecond)
	defer sink.Close()

	if err := sink.Handle(context.Background(), testEntry(0)); err != nil {
		t.Fatalf("Handle should never error, got %v", err)
	}
	if rec.total() != 0 {
		t.Fatal("expected no immediate flush below batch size")
	}

	deadline := time.Now().Add(2 * time.Second)
	for rec.total() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if rec.total() != 1 {
		t.Fatalf("expected window-triggered flush of 1 entry, got %d", rec.total())
	}
}

// TestAuditSink_WriteFailureDropsBatch 写入失败 → Handle 仍返回 nil（消费者 ACK），
// 该批被丢弃不重试：宁可丢不可堵。
func TestAuditSink_WriteFailureDropsBatch(t *testing.T) {
	rec := &sinkRecorder{fail: true}
	sink := NewAuditSink(rec.write, 2, time.Hour)
	defer sink.Close()

	for i := 0; i < 2; i++ {
		if err := sink.Handle(context.Background(), testEntry(i)); err != nil {
			t.Fatalf("Handle must return nil even when writer fails, got %v", err)
		}
	}
	// 缓冲应已清空（失败批被丢弃），后续 flush 不再携带旧数据
	sink.mu.Lock()
	pending := len(sink.buf)
	sink.mu.Unlock()
	if pending != 0 {
		t.Fatalf("expected buffer drained after failed flush, got %d pending", pending)
	}
}

// TestAuditSink_CloseFlushesRemainder Close 时 flush 残留缓冲。
func TestAuditSink_CloseFlushesRemainder(t *testing.T) {
	rec := &sinkRecorder{}
	sink := NewAuditSink(rec.write, 100, time.Hour)
	_ = sink.Handle(context.Background(), testEntry(0))
	sink.Close()
	if rec.total() != 1 {
		t.Fatalf("expected Close to flush remainder, got %d", rec.total())
	}
}

// TestWriteAuditLogs_NilPool PG 不可用时返回错误（由 sink 记日志丢弃）。
func TestWriteAuditLogs_NilPool(t *testing.T) {
	if Pool != nil {
		t.Skip("PG pool available; nil-pool path not testable")
	}
	err := WriteAuditLogs(context.Background(), []AuditEntry{{Action: "x"}})
	if err == nil {
		t.Fatal("expected error when PG pool unavailable")
	}
}

// TestAuditResourceType 路径前缀归类。
func TestAuditResourceType(t *testing.T) {
	cases := map[string]string{
		"/v1/ent/privacy":     "ent",
		"/v1/admin/settings":  "admin",
		"/admin/maintenance":  "admin",
		"/v1/conversations":   "api",
		"session.create":      "general",
		"":                    "general",
	}
	for in, want := range cases {
		if got := auditResourceType(in); got != want {
			t.Errorf("auditResourceType(%q) = %q, want %q", in, got, want)
		}
	}
}
