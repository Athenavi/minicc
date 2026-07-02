package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ReadFileTool reads a file from the workspace.
type ReadFileTool struct {
	workspaceDir string
}

func NewReadFileTool(workspaceDir string) *ReadFileTool {
	return &ReadFileTool{workspaceDir: workspaceDir}
}

func (t *ReadFileTool) Name() string        { return "read_file" }
func (t *ReadFileTool) Description() string  { return "Read the contents of a file" }

func (t *ReadFileTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	path, _ := input["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	fullPath := filepath.Join(t.workspaceDir, path)
	fullPath = filepath.Clean(fullPath)

	// Path traversal protection
	if !isWithin(t.workspaceDir, fullPath) {
		return nil, fmt.Errorf("path traversal denied: %s", path)
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return map[string]interface{}{
		"content":  string(data),
		"path":     path,
		"size":     len(data),
	}, nil
}

// WriteFileTool writes content to a file.
type WriteFileTool struct {
	workspaceDir string
}

func NewWriteFileTool(workspaceDir string) *WriteFileTool {
	return &WriteFileTool{workspaceDir: workspaceDir}
}

func (t *WriteFileTool) Name() string       { return "write_file" }
func (t *WriteFileTool) Description() string { return "Write content to a file" }

func (t *WriteFileTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	path, _ := input["path"].(string)
	content, _ := input["content"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	fullPath := filepath.Join(t.workspaceDir, path)
	fullPath = filepath.Clean(fullPath)

	if !isWithin(t.workspaceDir, fullPath) {
		return nil, fmt.Errorf("path traversal denied: %s", path)
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return map[string]interface{}{
		"path":    path,
		"size":    len(content),
		"written": true,
	}, nil
}

func isWithin(base, target string) bool {
	base = filepath.Clean(base)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return len(rel) < len(target) && !filepath.IsAbs(rel)
}

// RegisterCommonTools registers the most commonly used tools.
func RegisterCommonTools(registry *ToolRegistry, workspaceDir string) {
	registry.Register(NewReadFileTool(workspaceDir))
	registry.Register(NewWriteFileTool(workspaceDir))
}
