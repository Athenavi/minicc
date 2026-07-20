package session

import (
	"context"
	"testing"
	"time"
	"unicode/utf8"
)

func TestGetSession_EmptyID(t *testing.T) {
	m := NewManager(nil, nil)
	_, err := m.GetSession(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestCreateSession_EmptyID(t *testing.T) {
	m := NewManager(nil, nil)
	_, err := m.CreateSession(context.Background(), "", "user1", "test")
	if err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestDeleteSession_EmptyID(t *testing.T) {
	m := NewManager(nil, nil)
	err := m.DeleteSession(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestCreateSession_DefaultTitle(t *testing.T) {
	m := NewManager(nil, nil)
	s, err := m.CreateSession(context.Background(), "test-sess-1", "user1", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if s.Title != "New Chat" {
		t.Fatalf("expected default title 'New Chat', got %q", s.Title)
	}
	if s.UserID != "user1" {
		t.Fatalf("expected user_id 'user1', got %q", s.UserID)
	}
	if s.ID != "test-sess-1" {
		t.Fatalf("expected id 'test-sess-1', got %q", s.ID)
	}
	if s.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt")
	}
	if s.UpdatedAt.IsZero() {
		t.Fatal("expected non-zero UpdatedAt")
	}
}

func TestCreateSession_WithTitle(t *testing.T) {
	m := NewManager(nil, nil)
	s, err := m.CreateSession(context.Background(), "test-sess-2", "user2", "My Chat Title")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if s.Title != "My Chat Title" {
		t.Fatalf("expected 'My Chat Title', got %q", s.Title)
	}
}

func TestTruncateTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello", "Hello"},
		{"Hello\nWorld", "Hello"},
		{"", "New Chat"},
		{"A", "A"},
	}
	for _, tc := range tests {
		got := truncateTitle(tc.input)
		if got != tc.want {
			t.Errorf("truncateTitle(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestTruncateTitle_LongLine(t *testing.T) {
	b := make([]byte, 200)
	for i := range b {
		b[i] = 'A' + byte(i%26)
	}
	got := truncateTitle(string(b))
	if utf8.RuneCountInString(got) > 120 {
		t.Fatalf("expected max 120 runes, got %d", utf8.RuneCountInString(got))
	}
}

func TestTruncateTitle_MultiByte(t *testing.T) {
	// 41 CJK characters = 123 bytes, should truncate to 40 chars (120 bytes)
	cjk := make([]rune, 41)
	for i := range cjk {
		cjk[i] = '中'
	}
	got := truncateTitle(string(cjk))
	runeCount := utf8.RuneCountInString(got)
	if runeCount > 120 {
		t.Fatalf("expected max 120 runes, got %d", runeCount)
	}
	// Verify output is valid UTF-8
	if !utf8.ValidString(got) {
		t.Fatal("truncateTitle produced invalid UTF-8 for multi-byte input")
	}
}

func TestGenID_Unique(t *testing.T) {
	id1 := genID()
	time.Sleep(1 * time.Nanosecond)
	id2 := genID()
	if id1 == id2 {
		t.Fatal("expected unique IDs")
	}
}

func TestNullableStr(t *testing.T) {
	if nullableStr("") != nil {
		t.Fatal("expected nil for empty string")
	}
	if nullableStr("a") == nil {
		t.Fatal("expected non-nil for non-empty string")
	}
}

func TestListSessions_NoDB(t *testing.T) {
	// Simulate no DB: Pool is nil
	m := NewManager(nil, nil)
	sessions, err := m.ListSessions(context.Background(), "user1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sessions != nil {
		t.Fatalf("expected nil sessions when no db, got %v", sessions)
	}
}

func TestGetMessages_NoDB(t *testing.T) {
	m := NewManager(nil, nil)
	msgs, err := m.GetMessages(context.Background(), "session1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if msgs != nil {
		t.Fatalf("expected nil messages when no db, got %v", msgs)
	}
}

func TestSaveMessage_NoDB(t *testing.T) {
	m := NewManager(nil, nil)
	err := m.SaveMessage(context.Background(), "session1", "user", "hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestSaveMessages_NoDB(t *testing.T) {
	m := NewManager(nil, nil)
	// Should not panic
	m.SaveMessages(context.Background(), "session1", "user1", "hello", "world")
}
