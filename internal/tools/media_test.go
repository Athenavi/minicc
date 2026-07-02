package tools

import (
	"context"
	"strings"
	"testing"
)

func TestMediaCreateTool_Name(t *testing.T) {
	m := NewMediaCreateTool()
	if m.Name() != "media_create" {
		t.Fatalf("expected 'media_create', got %q", m.Name())
	}
}

func TestMediaCreateTool_EmptyName(t *testing.T) {
	m := NewMediaCreateTool()
	_, err := m.Execute(context.Background(), map[string]interface{}{
		"content": "hello",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestMediaCreateTool_EmptyContent(t *testing.T) {
	m := NewMediaCreateTool()
	_, err := m.Execute(context.Background(), map[string]interface{}{
		"name": "test",
	})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestMediaCreateTool_NoDB(t *testing.T) {
	// When db.Pool is nil, should return error
	m := NewMediaCreateTool()
	_, err := m.Execute(context.Background(), map[string]interface{}{
		"name":    "test.csv",
		"content": "a,b,c\n1,2,3",
	})
	if err == nil {
		t.Fatal("expected error when no database")
	}
	if !strings.Contains(err.Error(), "database not available") {
		t.Fatalf("expected 'database not available', got %v", err)
	}
}

func TestMediaCreateTool_DefaultType(t *testing.T) {
	// Should not panic with just name and content
	m := NewMediaCreateTool()
	_, err := m.Execute(context.Background(), map[string]interface{}{
		"name":    "test.csv",
		"content": "a,b,c\n1,2,3",
	})
	if err == nil {
		t.Fatal("expected error (no DB)")
	}
	_ = err
}

func TestJoinStrings(t *testing.T) {
	result := joinStrings([]string{"a", "b", "c"}, ",")
	if result != "a,b,c" {
		t.Fatalf("expected 'a,b,c', got %q", result)
	}
}

func TestJoinStrings_Empty(t *testing.T) {
	result := joinStrings([]string{}, ",")
	if result != "" {
		t.Fatalf("expected '', got %q", result)
	}
}

func TestJoinStrings_Single(t *testing.T) {
	result := joinStrings([]string{"only"}, ",")
	if result != "only" {
		t.Fatalf("expected 'only', got %q", result)
	}
}

func TestRegisterMediaTools(t *testing.T) {
	reg := NewToolRegistry()
	RegisterMediaTools(reg)
	if reg.Get("media_create") == nil {
		t.Fatal("expected media_create to be registered")
	}
}
