package model

import (
	"testing"
	"time"
)

func TestUser(t *testing.T) {
	u := User{
		ID:    "user-1",
		Email: "test@test.com",
		Name:  "Test User",
		Role:  "user",
	}
	if u.ID != "user-1" {
		t.Fatalf("expected user-1, got %q", u.ID)
	}
}

func TestSessionModel(t *testing.T) {
	s := Session{
		ID:     "sess-1",
		UserID: "user-1",
		Title:  "Test Chat",
	}
	if s.Title != "Test Chat" {
		t.Fatalf("expected 'Test Chat', got %q", s.Title)
	}
	if !s.CreatedAt.IsZero() {
		t.Fatal("expected zero CreatedAt")
	}
}

func TestMessage(t *testing.T) {
	m := Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Now(),
	}
	if m.Role != "user" {
		t.Fatalf("expected 'user', got %q", m.Role)
	}
	if m.Content != "hello" {
		t.Fatalf("expected 'hello', got %q", m.Content)
	}
}

func TestToolCallModel(t *testing.T) {
	tc := ToolCall{
		ID:       "tc-1",
		ToolName: "read_file",
		Input:    `{"path":"test.txt"}`,
		Output:   "content",
	}
	if tc.ToolName != "read_file" {
		t.Fatalf("expected 'read_file', got %q", tc.ToolName)
	}
	if tc.IsError {
		t.Fatal("expected no error")
	}
}

func TestTaskModel(t *testing.T) {
	task := Task{
		ID:     "task-1",
		UserID: "user-1",
		Type:   "llm",
		Status: "pending",
	}
	if task.Status != "pending" {
		t.Fatalf("expected 'pending', got %q", task.Status)
	}
}

func TestUserRoleDefaults(t *testing.T) {
	u := User{Role: "admin"}
	if u.Role != "admin" {
		t.Fatalf("expected 'admin', got %q", u.Role)
	}
}
