package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/athenavi/minicc/internal/tools"
)

// EditorHandler handles file browsing and editing for the Monaco editor.
type EditorHandler struct {
	workspaceDir string
	readTool     *tools.ReadFileTool
	writeTool    *tools.WriteFileTool
}

func NewEditorHandler(workspaceDir string) *EditorHandler {
	return &EditorHandler{
		workspaceDir: workspaceDir,
		readTool:     tools.NewReadFileTool(workspaceDir),
		writeTool:    tools.NewWriteFileTool(workspaceDir),
	}
}

// FileNode represents a file or directory in the tree.
type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Type     string     `json:"type"` // "file" or "dir"
	Children []FileNode `json:"children,omitempty"`
}

// ListFiles returns the workspace file tree.
func (h *EditorHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	basePath := h.workspaceDir

	tree, err := buildTree(basePath, basePath, 0)
	if err != nil {
		// Return empty tree if workspace doesn't exist yet
		OK(w, map[string]interface{}{"files": []FileNode{}})
		return
	}

	OK(w, map[string]interface{}{"files": tree.Children})
}

// ReadFile returns the content of a file.
func (h *EditorHandler) ReadFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		BadRequest(w, "path is required")
		return
	}

	input := map[string]interface{}{"path": path}
	result, err := h.readTool.Execute(r.Context(), input)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, result)
}

// WriteFile saves content to a file.
func (h *EditorHandler) WriteFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.Path == "" {
		BadRequest(w, "path is required")
		return
	}

	input := map[string]interface{}{"path": body.Path, "content": body.Content}
	result, err := h.writeTool.Execute(r.Context(), input)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, result)
}

// buildTree recursively builds a file tree from the given directory.
func buildTree(basePath, currentPath string, depth int) (*FileNode, error) {
	if depth > 5 {
		return &FileNode{Name: filepath.Base(currentPath), Path: relPath(basePath, currentPath), Type: "dir"}, nil
	}

	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return nil, err
	}

	node := &FileNode{
		Name: filepath.Base(currentPath),
		Path: relPath(basePath, currentPath),
		Type: "dir",
	}

	// Skip hidden directories, node_modules, .git
	skipDirs := map[string]bool{
		".git": true, "node_modules": true, ".next": true,
		"__pycache__": true, ".cache": true, "venv": true,
	}

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && e.IsDir() {
			continue
		}
		fullPath := filepath.Join(currentPath, e.Name())

		if e.IsDir() {
			if skipDirs[e.Name()] {
				continue
			}
			child, err := buildTree(basePath, fullPath, depth+1)
			if err != nil {
				continue
			}
			node.Children = append(node.Children, *child)
		} else {
			// Only include text-like files
			ext := strings.ToLower(filepath.Ext(e.Name()))
			textExts := map[string]bool{
				".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
				".py": true, ".rs": true, ".java": true, ".c": true, ".h": true,
				".cpp": true, ".hpp": true, ".css": true, ".html": true, ".md": true,
				".json": true, ".yaml": true, ".yml": true, ".toml": true, ".ini": true,
				".cfg": true, ".conf": true, ".sh": true, ".bash": true, ".sql": true,
				".xml": true, ".svg": true, ".txt": true, ".env": true,
				".mod": true, ".sum": true, ".lock": true,
			}
			if !textExts[ext] && ext != "" {
				continue
			}
			node.Children = append(node.Children, FileNode{
				Name: e.Name(),
				Path: relPath(basePath, fullPath),
				Type: "file",
			})
		}
	}

	// Sort: directories first, then alphabetical
	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].Type != node.Children[j].Type {
			return node.Children[i].Type == "dir"
		}
		return node.Children[i].Name < node.Children[j].Name
	})

	return node, nil
}

func relPath(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return rel
}
