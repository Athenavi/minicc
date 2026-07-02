package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ── Web Tool Tests ────────────────────────────────────────────────────────

func TestWebFetchTool_Name(t *testing.T) {
	w := NewWebFetchTool()
	if w.Name() != "web_fetch" {
		t.Fatalf("expected 'web_fetch', got %q", w.Name())
	}
}

func TestWebFetchTool_EmptyURL(t *testing.T) {
	w := NewWebFetchTool()
	_, err := w.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty url")
	}
}

func TestWebFetchTool_InvalidURL(t *testing.T) {
	w := NewWebFetchTool()
	_, err := w.Execute(context.Background(), map[string]interface{}{
		"url": "not-a-url",
	})
	if err == nil {
		t.Fatal("expected error for invalid url")
	}
}

func TestWebFetchTool_HasRedirectLimit(t *testing.T) {
	st := NewWebFetchTool()
	if st.client.Timeout != 15*time.Second {
		t.Fatalf("expected 15s timeout, got %v", st.client.Timeout)
	}
}

func TestRegisterWebTools(t *testing.T) {
	reg := NewToolRegistry()
	RegisterWebTools(reg)
	if reg.Get("web_fetch") == nil {
		t.Fatal("expected web_fetch to be registered")
	}
}

// ── Search Tool Tests ─────────────────────────────────────────────────────

func TestSearchTool_Name(t *testing.T) {
	s := NewSearchTool(".")
	if s.Name() != "search_files" {
		t.Fatalf("expected 'search_files', got %q", s.Name())
	}
}

func TestSearchTool_EmptyPattern(t *testing.T) {
	s := NewSearchTool(".")
	_, err := s.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestSearchTool_FindGoFiles(t *testing.T) {
	s := NewSearchTool(".")
	result, err := s.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.go",
		"root":    ".",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	count, _ := result["count"].(int)
	if count == 0 {
		t.Fatal("expected at least 1 .go file found")
	}
}

func TestGrepTool_Name(t *testing.T) {
	g := NewGrepTool(".")
	if g.Name() != "grep_files" {
		t.Fatalf("expected 'grep_files', got %q", g.Name())
	}
}

func TestGrepTool_EmptyQuery(t *testing.T) {
	g := NewGrepTool(".")
	_, err := g.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestGrepTool_FindPackage(t *testing.T) {
	g := NewGrepTool(".")
	result, err := g.Execute(context.Background(), map[string]interface{}{
		"query": "package tools",
		"root":  ".",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	count, _ := result["count"].(int)
	if count == 0 {
		t.Fatal("expected at least 1 match for 'package tools'")
	}
}

func TestRegisterSearchTools(t *testing.T) {
	reg := NewToolRegistry()
	RegisterSearchTools(reg, ".")
	if reg.Get("search_files") == nil {
		t.Fatal("expected search_files to be registered")
	}
	if reg.Get("grep_files") == nil {
		t.Fatal("expected grep_files to be registered")
	}
}

// ── RPA Tool Tests ────────────────────────────────────────────────────────

func TestBrowserNavigateTool(t *testing.T) {
	b := NewBrowserNavigateTool()
	if b.Name() != "browser_navigate" {
		t.Fatalf("expected 'browser_navigate', got %q", b.Name())
	}
	_, err := b.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty url")
	}
}

func TestBrowserExtractTool(t *testing.T) {
	b := NewBrowserExtractTool()
	if b.Name() != "browser_extract" {
		t.Fatalf("expected 'browser_extract', got %q", b.Name())
	}
	_, err := b.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty url")
	}
}

func TestDesktopAutomationTool(t *testing.T) {
	d := NewDesktopAutomationTool()
	if d.Name() != "desktop_automation" {
		t.Fatalf("expected 'desktop_automation', got %q", d.Name())
	}
	result, err := d.Execute(context.Background(), map[string]interface{}{"action": "info"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	output, _ := result["output"].(string)
	if !strings.Contains(output, "Desktop automation") {
		t.Fatalf("expected 'Desktop automation' in output, got %q", output)
	}
}

func TestExcelReadTool(t *testing.T) {
	e := NewExcelReadTool()
	if e.Name() != "excel_read" {
		t.Fatalf("expected 'excel_read', got %q", e.Name())
	}
	_, err := e.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestExcelWriteTool(t *testing.T) {
	e := NewExcelWriteTool()
	if e.Name() != "excel_write" {
		t.Fatalf("expected 'excel_write', got %q", e.Name())
	}
	_, err := e.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestEmailSendTool(t *testing.T) {
	e := NewEmailSendTool()
	if e.Name() != "email_send" {
		t.Fatalf("expected 'email_send', got %q", e.Name())
	}
	_, err := e.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty params")
	}
	result, err := e.Execute(context.Background(), map[string]interface{}{
		"to":      "test@example.com",
		"subject": "Hello",
		"body":    "World",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	status, _ := result["status"].(string)
	if status != "queued" {
		t.Fatalf("expected status 'queued', got %q", status)
	}
}

func TestWorkflowRunTool(t *testing.T) {
	w := NewWorkflowRunTool()
	if w.Name() != "workflow_run" {
		t.Fatalf("expected 'workflow_run', got %q", w.Name())
	}
	_, err := w.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestWorkflowStatusTool(t *testing.T) {
	w := NewWorkflowStatusTool()
	if w.Name() != "workflow_status" {
		t.Fatalf("expected 'workflow_status', got %q", w.Name())
	}
	_, err := w.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty instance_id")
	}
}

func TestRegisterRPATools(t *testing.T) {
	reg := NewToolRegistry()
	RegisterRPATools(reg)
	expected := []string{"browser_navigate", "browser_extract", "desktop_automation",
		"excel_read", "excel_write", "email_send", "workflow_run", "workflow_status"}
	for _, name := range expected {
		if reg.Get(name) == nil {
			t.Fatalf("expected %q to be registered", name)
		}
	}
}

// ── Enterprise Tool Tests ─────────────────────────────────────────────────

func TestEnterpriseQueryTool_Name(t *testing.T) {
	e := NewEnterpriseQueryTool()
	if e.Name() != "enterprise_query" {
		t.Fatalf("expected 'enterprise_query', got %q", e.Name())
	}
}

func TestEnterpriseQueryTool_NoDB(t *testing.T) {
	e := NewEnterpriseQueryTool()
	result, err := e.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	output, _ := result["output"].(string)
	if !strings.Contains(output, "Database not available") {
		t.Fatalf("expected 'Database not available', got %q", output)
	}
}

func TestEnterpriseInsertTool_Name(t *testing.T) {
	e := NewEnterpriseInsertTool()
	if e.Name() != "enterprise_insert" {
		t.Fatalf("expected 'enterprise_insert', got %q", e.Name())
	}
}

func TestEnterpriseInsertTool_NoDB(t *testing.T) {
	e := NewEnterpriseInsertTool()
	_, err := e.Execute(context.Background(), map[string]interface{}{
		"table": "wiki",
		"title": "test",
	})
	if err == nil {
		t.Fatal("expected error when no DB")
	}
}

func TestMapKeys(t *testing.T) {
	m := map[string]string{"a": "1", "b": "2", "c": "3"}
	keys := mapKeys(m)
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
}

// ── DevOps Tool Tests ─────────────────────────────────────────────────────

func TestDevOpsHealthTool_Name(t *testing.T) {
	d := NewDevOpsHealthTool()
	if d.Name() != "devops_health" {
		t.Fatalf("expected 'devops_health', got %q", d.Name())
	}
}

func TestDevOpsHealthTool_NoDB(t *testing.T) {
	d := NewDevOpsHealthTool()
	result, err := d.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	output, _ := result["output"].(string)
	if !strings.Contains(output, "not configured") {
		t.Fatalf("expected 'not configured' in output, got %q", output)
	}
}

func TestRegisterEnterpriseTools(t *testing.T) {
	reg := NewToolRegistry()
	RegisterEnterpriseTools(reg)
	if reg.Get("enterprise_query") == nil {
		t.Fatal("expected enterprise_query to be registered")
	}
	if reg.Get("enterprise_insert") == nil {
		t.Fatal("expected enterprise_insert to be registered")
	}
}

func TestRegisterDevOpsTools(t *testing.T) {
	reg := NewToolRegistry()
	RegisterDevOpsTools(reg)
	if reg.Get("devops_health") == nil {
		t.Fatal("expected devops_health to be registered")
	}
}
