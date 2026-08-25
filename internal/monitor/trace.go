package monitor

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type contextKey string

const traceContextKey contextKey = "chiron_trace"

// 鈹€鈹€ Trace ID / Span ID 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// TraceID is a 16-byte unique identifier for a trace.
type TraceID [16]byte

func (t TraceID) String() string {
	return fmt.Sprintf("%x", [16]byte(t))
}

// SpanID is an 8-byte unique identifier for a span.
type SpanID [8]byte

func (s SpanID) String() string {
	return fmt.Sprintf("%x", [8]byte(s))
}

func newTraceID() TraceID {
	var id TraceID
	if _, err := rand.Read(id[:]); err != nil {
		// 澶辫触鏃堕€€鍖栦负鏃堕棿鎴?鍘熷瓙璁℃暟锛岄伩鍏嶅叏闆?ID 寮曞彂 trace 鍐茬獊
		binary.BigEndian.PutUint64(id[8:], uint64(time.Now().UnixNano()))
		binary.BigEndian.PutUint64(id[0:], traceCounter.Add(1))
	}
	return id
}

func newSpanID() SpanID {
	var id SpanID
	if _, err := rand.Read(id[:]); err != nil {
		binary.BigEndian.PutUint64(id[:], traceCounter.Add(1))
	}
	return id
}

// traceCounter 鎻愪緵 rand 澶辫触鏃剁殑鍥為€€鑷璁℃暟锛屼繚璇?ID 鍞竴鎬с€?var traceCounter atomic.Uint64

// 鈹€鈹€ Span 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// Span represents a single operation within a trace.
type Span struct {
	TraceID    TraceID              `json:"trace_id"`
	SpanID     SpanID               `json:"span_id"`
	ParentID   SpanID               `json:"parent_span_id,omitempty"`
	Name       string               `json:"name"`
	Kind       string               `json:"kind"` // internal / server / client
	StartTime  time.Time            `json:"start_time"`
	EndTime    time.Time            `json:"end_time,omitempty"`
	Tags       map[string]interface{} `json:"tags,omitempty"`
	Events     []SpanEvent          `json:"events,omitempty"`
	StatusCode string               `json:"status_code,omitempty"` // OK / ERROR
	StatusMsg  string               `json:"status_msg,omitempty"`
	mu         sync.Mutex
}

type SpanEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	Name      string                 `json:"name"`
	Tags      map[string]interface{} `json:"tags,omitempty"`
}

// Tracer creates and manages spans.
type Tracer struct {
	mu       sync.Mutex
	exportFn func(*Span)
}

// NewTracer creates a new tracer. If exportFn is nil, spans are logged via slog.
func NewTracer(exportFn func(*Span)) *Tracer {
	if exportFn == nil {
		exportFn = defaultExport
	}
	return &Tracer{exportFn: exportFn}
}

func defaultExport(s *Span) {
	attrs := []slog.Attr{
		slog.String("trace_id", s.TraceID.String()),
		slog.String("span_id", s.SpanID.String()),
		slog.String("name", s.Name),
		slog.String("kind", s.Kind),
		slog.String("duration", s.EndTime.Sub(s.StartTime).String()),
		slog.String("status", s.StatusCode),
	}
	if s.ParentID != (SpanID{}) {
		attrs = append(attrs, slog.String("parent_span_id", s.ParentID.String()))
	}
	if s.StatusMsg != "" {
		attrs = append(attrs, slog.String("status_msg", s.StatusMsg))
	}
	if len(s.Tags) > 0 {
		attrs = append(attrs, slog.Any("tags", s.Tags))
	}
	// slog.Debug 鍙傛暟涓?...any锛氭樉寮忓睍寮€ []slog.Attr锛圙o 1.26 涓嶅啀鎺ュ彈鍒囩墖鐩存帴灞曞紑锛?	args := make([]any, 0, len(attrs))
	for _, a := range attrs {
		args = append(args, a)
	}
	slog.Debug("span", args...)
}

// Global tracer instance.
var GlobalTracer = NewTracer(nil)

// StartSpan creates a new span within the given context.
// If the context already contains a trace, the new span becomes a child of it.
// Returns the new context (with the span attached) and the span itself.
func StartSpan(ctx context.Context, name, kind string) (context.Context, *Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	parent := GetSpan(ctx)
	span := &Span{
		TraceID:   newTraceID(),
		SpanID:    newSpanID(),
		Name:      name,
		Kind:      kind,
		StartTime: time.Now(),
		Tags:      make(map[string]interface{}),
	}
	if parent != nil {
		span.TraceID = parent.TraceID
		span.ParentID = parent.SpanID
	}
	ctx = context.WithValue(ctx, traceContextKey, span)
	return ctx, span
}

// GetSpan retrieves the current span from a context. Returns nil if not found.
func GetSpan(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(traceContextKey).(*Span)
	return s
}

// 鈹€鈹€ Span methods 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func (s *Span) SetTag(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tags[key] = value
}

func (s *Span) AddEvent(name string, tags map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = append(s.Events, SpanEvent{
		Timestamp: time.Now(),
		Name:      name,
		Tags:      tags,
	})
}

func (s *Span) SetStatus(code, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.StatusCode = code
	s.StatusMsg = msg
}

// 鈹€鈹€ Exported span store (in-memory, for debugging) 鈹€鈹€

var completedSpansMu sync.Mutex
var completedSpans []*Span
var maxCompletedSpans = 500

// GetCompletedSpans returns a copy of all completed spans.
func GetCompletedSpans(limit int) []*Span {
	completedSpansMu.Lock()
	defer completedSpansMu.Unlock()
	if limit <= 0 || limit > len(completedSpans) {
		limit = len(completedSpans)
	}
	result := make([]*Span, limit)
	copy(result, completedSpans[len(completedSpans)-limit:])
	return result
}

// SpanExport stores a completed span in memory and logs it.
func SpanExport(s *Span) {
	completedSpansMu.Lock()
	defer completedSpansMu.Unlock()
	completedSpans = append(completedSpans, s)
	if len(completedSpans) > maxCompletedSpans {
		completedSpans = completedSpans[len(completedSpans)-maxCompletedSpans:]
	}
	defaultExport(s)
}

// InitWithSpanStore initializes the global tracer to use SpanExport.
func InitWithSpanStore() {
	GlobalTracer = NewTracer(SpanExport)
}

// End finalizes the span and exports it. Call when the operation completes.
func (s *Span) End() {
	s.mu.Lock()
	s.EndTime = time.Now()
	if s.StatusCode == "" {
		s.StatusCode = "OK"
	}
	s.mu.Unlock()

	// 鍦ㄩ攣澶栬皟鐢?exportFn锛岄伩鍏嶅洖璋冧腑鑾峰彇鍏朵粬閿佸鑷存閿?	GlobalTracer.exportFn(s)
}

// 鈹€鈹€ Convenience: context-based tracing 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// Trace wraps a function call with a span. Usage:
//
//	ctx, span := monitor.StartSpan(ctx, "operation", "internal")
//	defer span.End()
func Trace(ctx context.Context, name, kind string, fn func(ctx context.Context) error) error {
	ctx, span := StartSpan(ctx, name, kind)
	err := fn(ctx)
	if err != nil {
		span.SetStatus("ERROR", err.Error())
	} else {
		span.SetStatus("OK", "")
	}
	span.End()
	return err
}

// TraceIDFromContext returns the trace ID as a hex string, or "" if none.
func TraceIDFromContext(ctx context.Context) string {
	s := GetSpan(ctx)
	if s == nil {
		return ""
	}
	return s.TraceID.String()
}
