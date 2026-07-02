package commands

import (
	"context"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	cmd := &Command{
		Name:        "test",
		Description: "A test command",
		Aliases:     []string{"t"},
		Handler:     func(ctx context.Context, args string) (string, error) { return "done", nil },
	}
	r.Register(cmd)

	got := r.Get("test")
	if got == nil {
		t.Fatal("expected to find command")
	}
	if got.Name != "test" {
		t.Fatalf("expected 'test', got %q", got.Name)
	}
}

func TestGetByAlias(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{
		Name:    "help",
		Aliases: []string{"h", "?"},
		Handler: func(ctx context.Context, args string) (string, error) { return "help", nil },
	})

	got := r.Get("h")
	if got == nil {
		t.Fatal("expected to find by alias")
	}
	if got.Name != "help" {
		t.Fatalf("expected 'help', got %q", got.Name)
	}
}

func TestGet_Missing(t *testing.T) {
	r := NewRegistry()
	got := r.Get("nonexistent")
	if got != nil {
		t.Fatal("expected nil for missing command")
	}
}

func TestList(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{Name: "a", Handler: func(ctx context.Context, args string) (string, error) { return "", nil }})
	r.Register(&Command{Name: "b", Handler: func(ctx context.Context, args string) (string, error) { return "", nil }})
	cmds := r.List()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
}

func TestList_DeduplicatesAliases(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{
		Name:    "help",
		Aliases: []string{"h", "?"},
		Handler: func(ctx context.Context, args string) (string, error) { return "", nil },
	})
	cmds := r.List()
	if len(cmds) != 1 {
		t.Fatalf("expected 1 unique command, got %d", len(cmds))
	}
}

func TestExecute(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{
		Name: "echo",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "echo: " + args, nil
		},
	})

	result, err := r.Execute(context.Background(), "/echo hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "echo: hello" {
		t.Fatalf("expected 'echo: hello', got %q", result)
	}
}

func TestExecute_Unknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute(context.Background(), "/unknown")
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestExecute_NoArgs(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{
		Name: "ping",
		Handler: func(ctx context.Context, args string) (string, error) {
			return "pong", nil
		},
	})
	result, err := r.Execute(context.Background(), "/ping")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "pong" {
		t.Fatalf("expected 'pong', got %q", result)
	}
}
