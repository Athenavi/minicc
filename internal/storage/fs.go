package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileStore is the abstraction for file storage.
type FileStore interface {
	Read(ctx context.Context, path string) ([]byte, error)
	Write(ctx context.Context, path string, data []byte) error
	Delete(ctx context.Context, path string) error
	List(ctx context.Context, prefix string) ([]FileInfo, error)
}

type FileInfo struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	IsDir    bool   `json:"is_dir"`
	Modified string `json:"modified"`
}

// LocalStore implements FileStore on the local filesystem.
type LocalStore struct {
	mu   sync.Mutex
	Root string
}

func NewLocalStore(root string) *LocalStore {
	return &LocalStore{Root: root}
}

func (s *LocalStore) Read(ctx context.Context, path string) ([]byte, error) {
	fullPath, err := s.safePath(path)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.ReadFile(fullPath)
}

func (s *LocalStore) Write(ctx context.Context, path string, data []byte) error {
	fullPath, err := s.safePath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

func (s *LocalStore) Delete(ctx context.Context, path string) error {
	fullPath, err := s.safePath(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.Remove(fullPath)
}

func (s *LocalStore) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	fullPath, err := s.safePath(prefix)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Path:     filepath.Join(prefix, e.Name()),
			Size:     info.Size(),
			IsDir:    e.IsDir(),
			Modified: info.ModTime().Format("2006-01-02T15:04:05Z"),
		})
	}
	return files, nil
}

func (s *LocalStore) safePath(path string) (string, error) {
	fullPath := filepath.Join(s.Root, path)
	cleaned := filepath.Clean(fullPath)
	root := filepath.Clean(s.Root)
	rel, err := filepath.Rel(root, cleaned)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path traversal blocked: %s", path)
	}
	// Resolve symlinks and verify the real path is still under root.
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		// File may not exist yet (for writes) — check the parent directory.
		resolved, err = filepath.EvalSymlinks(filepath.Dir(cleaned))
		if err != nil {
			return cleaned, nil // best effort; let the OS handle it
		}
	}
	resRoot := root
	if r, err := filepath.EvalSymlinks(root); err == nil {
		resRoot = r
	}
	rel2, err := filepath.Rel(resRoot, resolved)
	if err != nil || strings.HasPrefix(rel2, "..") {
		return "", fmt.Errorf("symlink escape blocked: %s", path)
	}
	return cleaned, nil
}

// ─── Store helpers ─────────────────────────────────────────────────────────

func NewStore(backend, root, endpoint, bucket, accessKey, secretKey string, useSSL bool) (FileStore, error) {
	switch backend {
	case "local", "":
		if err := os.MkdirAll(root, 0755); err != nil {
			return nil, fmt.Errorf("create storage root: %w", err)
		}
		return NewLocalStore(root), nil
	case "s3":
		store, err := NewS3Store(endpoint, bucket, "", accessKey, secretKey, "", useSSL)
		if err != nil {
			return nil, fmt.Errorf("s3 init: %w", err)
		}
		return store, nil
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", backend)
	}
}
