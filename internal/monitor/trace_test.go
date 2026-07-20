package monitor

import (
	"context"
	"testing"
)

func TestNewTraceID(t *testing.T) {
	id1 := newTraceID()
	id2 := newTraceID()
	if id1 == id2 {
		t.Fatal("expected unique trace IDs")
	}
	if id1.String() == "" {
		t.Fatal("expected non-empty string")
	}
}

func TestNewSpanID(t *testing.T) {
	id1 := newSpanID()
	id2 := newSpanID()
	if id1 == id2 {
		t.Fatal("expected unique span IDs")
	}
}

func TestStartSpan_NoParent(t *testing.T) {
	ctx, span := StartSpan(context.Background(), "test", "internal")
	if span == nil {
		t.Fatal("expected non-nil span")
	}
	if span.Name != "test" {
		t.Fatalf("expected name 'test', got %q", span.Name)
	}
	if span.Kind != "internal" {
		t.Fatalf("expected kind 'internal', got %q", span.Kind)
	}
	if span.ParentID != (SpanID{}) {
		t.Fatal("expected empty parent span ID")
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
}

func TestStartSpan_WithParent(t *testing.T) {
	ctx, parent := StartSpan(context.Background(), "parent", "server")
	ctx, child := StartSpan(ctx, "child", "client")
	if child.TraceID != parent.TraceID {
		t.Fatal("expected child to inherit trace ID")
	}
	if child.ParentID != parent.SpanID {
		t.Fatal("expected child to reference parent span ID")
	}
}

func TestGetSpan_Empty(t *testing.T) {
	s := GetSpan(context.Background())
	if s != nil {
		t.Fatal("expected nil span for empty context")
	}
}

func TestGetSpan_Found(t *testing.T) {
	ctx, span := StartSpan(context.Background(), "test", "internal")
	got := GetSpan(ctx)
	if got == nil {
		t.Fatal("expected to find span")
	}
	if got.SpanID != span.SpanID {
		t.Fatal("expected same span ID")
	}
}

func TestSpan_SetTag(t *testing.T) {
	_, span := StartSpan(context.Background(), "test", "internal")
	span.SetTag("key", "value")
	if span.Tags["key"] != "value" {
		t.Fatalf("expected tag 'value', got %v", span.Tags["key"])
	}
}

func TestSpan_AddEvent(t *testing.T) {
	_, span := StartSpan(context.Background(), "test", "internal")
	span.AddEvent("cache_miss", map[string]interface{}{"key": "foo"})
	if len(span.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(span.Events))
	}
	if span.Events[0].Name != "cache_miss" {
		t.Fatalf("expected 'cache_miss', got %q", span.Events[0].Name)
	}
}

func TestSpan_SetStatus(t *testing.T) {
	_, span := StartSpan(context.Background(), "test", "internal")
	span.SetStatus("ERROR", "something went wrong")
	if span.StatusCode != "ERROR" {
		t.Fatalf("expected 'ERROR', got %q", span.StatusCode)
	}
	if span.StatusMsg != "something went wrong" {
		t.Fatalf("expected 'something went wrong', got %q", span.StatusMsg)
	}
}

func TestSpan_End(t *testing.T) {
	_, span := StartSpan(context.Background(), "test", "internal")
	span.End()
	if span.EndTime.IsZero() {
		t.Fatal("expected non-zero end time")
	}
	if span.StatusCode != "OK" {
		t.Fatalf("expected default status 'OK', got %q", span.StatusCode)
	}
}

func TestTrace_Success(t *testing.T) {
	err := Trace(context.Background(), "op", "internal", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestTrace_Error(t *testing.T) {
	err := Trace(context.Background(), "op", "internal", func(ctx context.Context) error {
		return &errCustom{msg: "something went wrong"}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "something went wrong" {
		t.Fatalf("expected 'something went wrong', got %v", err)
	}
}

type errCustom struct{ msg string }
func (e *errCustom) Error() string { return e.msg }

func TestTraceIDFromContext(t *testing.T) {
	ctx, span := StartSpan(context.Background(), "test", "internal")
	id := TraceIDFromContext(ctx)
	if id == "" {
		t.Fatal("expected non-empty trace ID")
	}
	expected := span.TraceID.String()
	if id != expected {
		t.Fatalf("expected %q, got %q", expected, id)
	}
}

func TestTraceIDFromContext_Empty(t *testing.T) {
	id := TraceIDFromContext(context.Background())
	if id != "" {
		t.Fatal("expected empty string")
	}
}

func TestNewTracer_CustomExport(t *testing.T) {
	exported := false
	tracer := NewTracer(func(s *Span) {
		exported = true
	})
	GlobalTracer = tracer
	_, span := StartSpan(context.Background(), "test", "internal")
	span.End()
	if !exported {
		t.Fatal("expected export function to be called")
	}
	// Restore default
	GlobalTracer = NewTracer(nil)
}

func TestStartSpan_NilContext(t *testing.T) {
	s := GetSpan(nil)
	if s != nil {
		t.Fatal("expected nil span for nil context")
	}
}

func TestStartSpan_NilContext_NoPanic(t *testing.T) {
	ctx, span := StartSpan(nil, "test", "internal")
	if span == nil {
		t.Fatal("expected non-nil span")
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if span.Name != "test" {
		t.Fatalf("expected name 'test', got %q", span.Name)
	}
}
