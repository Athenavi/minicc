package broadcast

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewHub_NilRedis(t *testing.T) {
	h := NewHub(nil)
	if h == nil {
		t.Fatal("expected non-nil hub")
	}
	if !h.localOnly {
		t.Fatal("expected localOnly with nil redis")
	}
}

func TestHub_Subscribe(t *testing.T) {
	h := NewHub(nil)
	ch := h.Subscribe("test-id")
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	if len(h.subs) != 1 {
		t.Fatalf("expected 1 subscriber, got %d", len(h.subs))
	}
}

func TestHub_Subscribe_Duplicate(t *testing.T) {
	h := NewHub(nil)
	ch1 := h.Subscribe("same-id")
	ch2 := h.Subscribe("same-id")
	if ch1 == ch2 {
		t.Fatal("expected different channels for re-subscribe")
	}
}

func TestHub_Unsubscribe(t *testing.T) {
	h := NewHub(nil)
	h.Subscribe("test-id")
	h.Unsubscribe("test-id")
	if len(h.subs) != 0 {
		t.Fatalf("expected 0 subscribers after unsubscribe, got %d", len(h.subs))
	}
}

func TestHub_Unsubscribe_Nonexistent(t *testing.T) {
	h := NewHub(nil)
	// Should not panic
	h.Unsubscribe("nonexistent")
}

func TestHub_Publish(t *testing.T) {
	h := NewHub(nil)
	ch := h.Subscribe("test-id")

	event := Event{Type: "text", Data: map[string]string{"content": "hello"}}
	h.Publish(event)

	select {
	case received := <-ch:
		if received.Type != "text" {
			t.Fatalf("expected type 'text', got %q", received.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestHub_Publish_MultipleSubscribers(t *testing.T) {
	h := NewHub(nil)
	ch1 := h.Subscribe("id-1")
	ch2 := h.Subscribe("id-2")

	h.Publish(Event{Type: "test"})

	select {
	case <-ch1:
	case <-time.After(time.Second):
		t.Fatal("timeout for subscriber 1")
	}
	select {
	case <-ch2:
	case <-time.After(time.Second):
		t.Fatal("timeout for subscriber 2")
	}
}

func TestHub_Close(t *testing.T) {
	h := NewHub(nil)
	h.Subscribe("id-1")
	h.Subscribe("id-2")

	h.Close()
	if len(h.subs) != 0 {
		t.Fatalf("expected 0 subs after close, got %d", len(h.subs))
	}
}

func TestFormatSSE(t *testing.T) {
	event := Event{Type: "connected", Data: map[string]string{"id": "client-1"}}
	sse := FormatSSE(event)
	if sse == "" {
		t.Fatal("expected non-empty SSE string")
	}
	// Verify it starts with "data: "
	if len(sse) < 6 || sse[:6] != "data: " {
		t.Fatalf("expected 'data: ' prefix, got %q", sse[:6])
	}
	// Verify it ends with "\n\n"
	if len(sse) < 2 || sse[len(sse)-2:] != "\n\n" {
		t.Fatalf("expected '\\n\\n' suffix, got %q", sse[len(sse)-2:])
	}
}

func TestFormatSSE_ValidJSON(t *testing.T) {
	event := Event{Type: "text", Data: map[string]string{"content": "hello"}}
	sse := FormatSSE(event)

	// Extract JSON after "data: "
	if len(sse) < 6 {
		t.Fatal("too short")
	}
	jsonStr := sse[6 : len(sse)-2] // remove "data: " prefix and "\n\n" suffix

	var decoded Event
	if err := json.Unmarshal([]byte(jsonStr), &decoded); err != nil {
		t.Fatalf("invalid JSON in SSE: %v", err)
	}
	if decoded.Type != "text" {
		t.Fatalf("expected type 'text', got %q", decoded.Type)
	}
}

func TestEvent_JSON(t *testing.T) {
	event := Event{
		Type: "tool_result",
		Data: map[string]interface{}{
			"tool_name": "read_file",
			"output":    "file content",
		},
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != "tool_result" {
		t.Fatalf("expected 'tool_result', got %q", decoded.Type)
	}
}
