package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SearchTool searches for files by name pattern.
type SearchTool struct {
	workspaceDir string
}

func NewSearchTool(workspaceDir string) *SearchTool {
	return &SearchTool{workspaceDir: workspaceDir}
}

func (t *SearchTool) Name() string       { return "search_files" }
func (t *SearchTool) Description() string { return "Search for files matching a glob pattern in the workspace." }

func (t *SearchTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	pattern, _ := input["pattern"].(string)
	if pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	root, _ := input["root"].(string)
	if root == "" {
		root = t.workspaceDir
	}

	// Limit search depth
	maxResults := 100

	var results []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if len(results) >= maxResults {
			return filepath.SkipAll
		}
		if matched, _ := filepath.Match(pattern, d.Name()); matched {
			rel, _ := filepath.Rel(root, path)
			results = append(results, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("search files: %w", err)
	}

	if results == nil {
		results = []string{}
	}

	return map[string]interface{}{
		"results": results,
		"count":   len(results),
		"pattern": pattern,
		"root":    root,
	}, nil
}

// GrepTool searches file contents for a substring or regex pattern.
type GrepTool struct {
	workspaceDir string
}

func NewGrepTool(workspaceDir string) *GrepTool {
	return &GrepTool{workspaceDir: workspaceDir}
}

func (t *GrepTool) Name() string       { return "grep_files" }
func (t *GrepTool) Description() string { return "Search file contents for a text pattern in the workspace." }

func (t *GrepTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	root, _ := input["root"].(string)
	if root == "" {
		root = t.workspaceDir
	}

	maxResults := 50
	var maxFileSize int64 = 10 * 1024 * 1024 // 10MB

	type Match struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Content string `json:"content"`
	}

	var matches []Match

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip hidden dirs
			if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if len(matches) >= maxResults {
			return filepath.SkipAll
		}

		info, err := d.Info()
		if err != nil || info.Size() > maxFileSize {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if len(matches) >= maxResults {
				break
			}
			if strings.Contains(line, query) {
				rel, _ := filepath.Rel(root, path)
				matches = append(matches, Match{
					File:    rel,
					Line:    i + 1,
					Content: strings.TrimSpace(line),
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("grep files: %w", err)
	}

	if matches == nil {
		matches = []Match{}
	}

	return map[string]interface{}{
		"matches": matches,
		"count":   len(matches),
		"query":   query,
	}, nil
}

// RegisterSearchTools registers search tools.
func RegisterSearchTools(registry *ToolRegistry, workspaceDir string) {
	registry.Register(NewSearchTool(workspaceDir))
	registry.Register(NewGrepTool(workspaceDir))
}
