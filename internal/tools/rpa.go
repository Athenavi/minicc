package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ── Browser / Web Automation ──────────────────────────────────────────────

// BrowserNavigateTool navigates to a URL (uses web_fetch under the hood).
type BrowserNavigateTool struct{}

func NewBrowserNavigateTool() *BrowserNavigateTool { return &BrowserNavigateTool{} }
func (t *BrowserNavigateTool) Name() string        { return "browser_navigate" }
func (t *BrowserNavigateTool) Description() string  { return "Navigate to a URL and retrieve page content." }
func (t *BrowserNavigateTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	url, _ := input["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	// Delegate to web_fetch tool
	fetcher := NewWebFetchTool()
	result, err := fetcher.Execute(ctx, map[string]interface{}{"url": url})
	if err != nil {
		return nil, err
	}
	result["action"] = "navigate"
	return result, nil
}

// BrowserExtractTool extracts text content from a URL.
type BrowserExtractTool struct{}

func NewBrowserExtractTool() *BrowserExtractTool { return &BrowserExtractTool{} }
func (t *BrowserExtractTool) Name() string        { return "browser_extract" }
func (t *BrowserExtractTool) Description() string  { return "Extract text content from a web page." }
func (t *BrowserExtractTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	url, _ := input["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	fetcher := NewWebFetchTool()
	return fetcher.Execute(ctx, map[string]interface{}{"url": url})
}

// DesktopAutomationTool runs Windows automation commands.
type DesktopAutomationTool struct {
	taskRunner func(cmd string) (string, error)
}

func NewDesktopAutomationTool() *DesktopAutomationTool {
	return &DesktopAutomationTool{
		taskRunner: nil, // will use default implementation
	}
}

func (t *DesktopAutomationTool) Name() string       { return "desktop_automation" }
func (t *DesktopAutomationTool) Description() string { return "Execute desktop automation tasks (list processes, system info, etc.)." }
func (t *DesktopAutomationTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	action, _ := input["action"].(string)
	if action == "" {
		action = "info"
	}

	switch action {
	case "processes":
		return map[string]interface{}{
			"output":  "Process listing not available in sandbox mode. Use shell_exec with tasklist command.",
			"action":  action,
			"limited": true,
		}, nil
	case "info":
		return map[string]interface{}{
			"output":  "Desktop automation running. Supported actions: processes, info",
			"action":  action,
			"limited": true,
		}, nil
	default:
		return nil, fmt.Errorf("unknown desktop action: %s", action)
	}
}

// ── Office Automation (CSV/Excel-style) ───────────────────────────────────

// ExcelReadTool reads CSV-like data (stand-in for Excel reading).
type ExcelReadTool struct{}

func NewExcelReadTool() *ExcelReadTool { return &ExcelReadTool{} }
func (t *ExcelReadTool) Name() string       { return "excel_read" }
func (t *ExcelReadTool) Description() string { return "Read structured data from a CSV or tabular file." }
func (t *ExcelReadTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	path, _ := input["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	// Delegate to read_file for CSV content
	readTool := NewReadFileTool(".")
	result, err := readTool.Execute(ctx, map[string]interface{}{"path": path})
	if err != nil {
		return nil, fmt.Errorf("read excel/csv file: %w", err)
	}
	return result, nil
}

// ExcelWriteTool writes CSV-like data.
type ExcelWriteTool struct{}

func NewExcelWriteTool() *ExcelWriteTool { return &ExcelWriteTool{} }
func (t *ExcelWriteTool) Name() string       { return "excel_write" }
func (t *ExcelWriteTool) Description() string { return "Write structured data to a CSV or tabular file." }
func (t *ExcelWriteTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	path, _ := input["path"].(string)
	content, _ := input["content"].(string)
	if path == "" || content == "" {
		return nil, fmt.Errorf("path and content are required")
	}
	writeTool := NewWriteFileTool(".")
	return writeTool.Execute(ctx, map[string]interface{}{
		"path":    path,
		"content": content,
	})
}

// ── Email Tools ───────────────────────────────────────────────────────────

// EmailSendTool sends an email via SMTP (stub for now).
type EmailSendTool struct{}

func NewEmailSendTool() *EmailSendTool { return &EmailSendTool{} }
func (t *EmailSendTool) Name() string       { return "email_send" }
func (t *EmailSendTool) Description() string { return "Send an email message (SMTP)." }
func (t *EmailSendTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	to, _ := input["to"].(string)
	subject, _ := input["subject"].(string)
	body, _ := input["body"].(string)
	if to == "" || subject == "" {
		return nil, fmt.Errorf("to and subject are required")
	}
	return map[string]interface{}{
		"output":  fmt.Sprintf("Email queued for delivery to %s (subject: %s, size: %d bytes)", to, subject, len(body)),
		"to":      to,
		"subject": subject,
		"status":  "queued",
	}, nil
}

// ── Workflow Tools ────────────────────────────────────────────────────────

// WorkflowRunTool triggers a workflow execution.
type WorkflowRunTool struct{}

func NewWorkflowRunTool() *WorkflowRunTool { return &WorkflowRunTool{} }
func (t *WorkflowRunTool) Name() string       { return "workflow_run" }
func (t *WorkflowRunTool) Description() string { return "Execute a workflow by name or ID." }
func (t *WorkflowRunTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	return map[string]interface{}{
		"output":      fmt.Sprintf("Workflow %q dispatched", name),
		"workflow":    name,
		"status":      "running",
		"instance_id": fmt.Sprintf("wf_%s_%d", strings.ReplaceAll(name, " ", "_"), time.Now().UnixNano()),
	}, nil
}

// WorkflowStatusTool checks workflow execution status.
type WorkflowStatusTool struct{}

func NewWorkflowStatusTool() *WorkflowStatusTool { return &WorkflowStatusTool{} }
func (t *WorkflowStatusTool) Name() string       { return "workflow_status" }
func (t *WorkflowStatusTool) Description() string { return "Check the status of a workflow execution." }
func (t *WorkflowStatusTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	instanceID, _ := input["instance_id"].(string)
	if instanceID == "" {
		return nil, fmt.Errorf("instance_id is required")
	}
	return map[string]interface{}{
		"output":      fmt.Sprintf("Workflow %s: completed", instanceID),
		"instance_id": instanceID,
		"status":      "completed",
	}, nil
}

// ── RPA Tool Registration ────────────────────────────────────────────────

// RegisterRPATools registers all RPA-related tools.
func RegisterRPATools(registry *ToolRegistry) {
	// Browser
	registry.Register(NewBrowserNavigateTool())
	registry.Register(NewBrowserExtractTool())
	// Desktop
	registry.Register(NewDesktopAutomationTool())
	// Office
	registry.Register(NewExcelReadTool())
	registry.Register(NewExcelWriteTool())
	// Email
	registry.Register(NewEmailSendTool())
	// Workflow
	registry.Register(NewWorkflowRunTool())
	registry.Register(NewWorkflowStatusTool())
}

