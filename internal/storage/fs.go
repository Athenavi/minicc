package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	Root string
}

func NewLocalStore(root string) *LocalStore {
	return &LocalStore{Root: root}
}

func (s *LocalStore) Read(ctx context.Context, path string) ([]byte, error) {
	fullPath := s.safePath(path)
	return os.ReadFile(fullPath)
}

func (s *LocalStore) Write(ctx context.Context, path string, data []byte) error {
	fullPath := s.safePath(path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, data, 0644)
}

func (s *LocalStore) Delete(ctx context.Context, path string) error {
	return os.Remove(s.safePath(path))
}

func (s *LocalStore) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	fullPath := s.safePath(prefix)
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

func (s *LocalStore) safePath(path string) string {
	fullPath := filepath.Join(s.Root, path)
	return filepath.Clean(fullPath)
}

// ─── Store helpers ─────────────────────────────────────────────────────────

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func readAll(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	return buf.Bytes(), err
}

func NewStore(backend, root, endpoint, bucket, accessKey, secretKey string) (FileStore, error) {
	switch backend {
	case "local", "":
		if err := os.MkdirAll(root, 0755); err != nil {
			return nil, fmt.Errorf("create storage root: %w", err)
		}
		return NewLocalStore(root), nil
	case "s3":
		store, err := NewS3Store(endpoint, bucket, "", accessKey, secretKey, "", true)
		if err != nil {
			return nil, fmt.Errorf("s3 init: %w", err)
		}
		return store, nil
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", backend)
	}
}
