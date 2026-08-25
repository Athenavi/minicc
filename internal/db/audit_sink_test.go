package db

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// sinkRecorder 鎹曡幏 sink 姣忔 flush 鐨勬壒閲忓唴瀹广€?
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

// TestAuditSink_BatchTrigger 杈惧埌 batchSize锛?00 鏉★級绔嬪嵆瑙﹀彂 flush銆?
func TestAuditSink_BatchTrigger(t *testing.T) {
	rec := &sinkRecorder{}
	// 绐楀彛璁鹃暱锛?0s锛夛紝纭繚瑙﹀彂鍙潵鑷壒閲忛槇鍊?
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

// TestAuditSink_WindowTrigger 鏈揪鎵归噺闃堝€兼椂锛?s 绐楀彛鍒版湡瑙﹀彂 flush銆?
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

// TestAuditSink_WriteFailureDropsBatch 鍐欏叆澶辫触 鈫?Handle 浠嶈繑鍥?nil锛堟秷璐硅€?ACK锛夛紝
// 璇ユ壒琚涪寮冧笉閲嶈瘯锛氬畞鍙涪涓嶅彲鍫点€?
func TestAuditSink_WriteFailureDropsBatch(t *testing.T) {
	rec := &sinkRecorder{fail: true}
	sink := NewAuditSink(rec.write, 2, time.Hour)
	defer sink.Close()

	for i := 0; i < 2; i++ {
		if err := sink.Handle(context.Background(), testEntry(i)); err != nil {
			t.Fatalf("Handle must return nil even when writer fails, got %v", err)
		}
	}
	// 缂撳啿搴斿凡娓呯┖锛堝け璐ユ壒琚涪寮冿級锛屽悗缁?flush 涓嶅啀鎼哄甫鏃ф暟鎹?
	sink.mu.Lock()
	pending := len(sink.buf)
	sink.mu.Unlock()
	if pending != 0 {
		t.Fatalf("expected buffer drained after failed flush, got %d pending", pending)
	}
}

// TestAuditSink_CloseFlushesRemainder Close 鏃?flush 娈嬬暀缂撳啿銆?
func TestAuditSink_CloseFlushesRemainder(t *testing.T) {
	rec := &sinkRecorder{}
	sink := NewAuditSink(rec.write, 100, time.Hour)
	_ = sink.Handle(context.Background(), testEntry(0))
	sink.Close()
	if rec.total() != 1 {
		t.Fatalf("expected Close to flush remainder, got %d", rec.total())
	}
}

// TestWriteAuditLogs_NilPool PG 涓嶅彲鐢ㄦ椂杩斿洖閿欒锛堢敱 sink 璁版棩蹇椾涪寮冿級銆?
func TestWriteAuditLogs_NilPool(t *testing.T) {
	if Pool != nil {
		t.Skip("PG pool available; nil-pool path not testable")
	}
	err := WriteAuditLogs(context.Background(), []AuditEntry{{Action: "x"}})
	if err == nil {
		t.Fatal("expected error when PG pool unavailable")
	}
}

// TestAuditResourceType 璺緞鍓嶇紑褰掔被銆?
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
