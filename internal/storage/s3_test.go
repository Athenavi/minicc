package storage

import (
	"testing"
)

func TestNewS3Store_EmptyBucket(t *testing.T) {
	_, err := NewS3Store("play.min.io", "", "", "key", "secret", "", true)
	if err == nil {
		t.Fatal("expected error for empty bucket")
	}
}

func TestNewS3Store_InvalidEndpoint(t *testing.T) {
	_, err := NewS3Store("", "test-bucket", "", "key", "secret", "", true)
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}
}

func TestObjectPath_NoPrefix(t *testing.T) {
	s := &S3Store{bucket: "b", prefix: ""}
	got := s.objectPath("dir/file.txt")
	if got != "dir/file.txt" {
		t.Fatalf("expected 'dir/file.txt', got %q", got)
	}
}

func TestObjectPath_WithPrefix(t *testing.T) {
	s := &S3Store{bucket: "b", prefix: "workspace"}
	got := s.objectPath("dir/file.txt")
	if got != "workspace/dir/file.txt" {
		t.Fatalf("expected 'workspace/dir/file.txt', got %q", got)
	}
}

func TestObjectPath_WindowsPath(t *testing.T) {
	s := &S3Store{bucket: "b", prefix: ""}
	got := s.objectPath("dir\\file.txt")
	if got != "dir/file.txt" {
		t.Fatalf("expected 'dir/file.txt' (forward slash), got %q", got)
	}
}

func TestStripPrefix_NoPrefix(t *testing.T) {
	s := &S3Store{bucket: "b", prefix: ""}
	got := s.stripPrefix("dir/file.txt")
	if got != "dir/file.txt" {
		t.Fatalf("expected 'dir/file.txt', got %q", got)
	}
}

func TestStripPrefix_WithPrefix(t *testing.T) {
	s := &S3Store{bucket: "b", prefix: "workspace"}
	got := s.stripPrefix("workspace/dir/file.txt")
	if got != "dir/file.txt" {
		t.Fatalf("expected 'dir/file.txt', got %q", got)
	}
}

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"file.html", "text/html"},
		{"file.css", "text/css"},
		{"file.js", "application/javascript"},
		{"file.json", "application/json"},
		{"file.png", "image/png"},
		{"file.jpg", "image/jpeg"},
		{"file.jpeg", "image/jpeg"},
		{"file.gif", "image/gif"},
		{"file.svg", "image/svg+xml"},
		{"file.pdf", "application/pdf"},
		{"file.md", "text/markdown"},
		{"file.txt", "text/plain"},
		{"file.yaml", "application/x-yaml"},
		{"file.yml", "application/x-yaml"},
		{"file.zip", "application/zip"},
		{"file.tar.gz", "application/gzip"},
		{"file.unknown", "application/octet-stream"},
		{"Makefile", "application/octet-stream"},
	}
	for _, tc := range tests {
		got := detectContentType(tc.path)
		if got != tc.want {
			t.Errorf("detectContentType(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestNewStore_S3Backend(t *testing.T) {
	// Should now attempt S3 connection instead of returning "not implemented".
	// With invalid endpoint, it returns an init error (not the old stub error).
	_, err := NewStore("s3", "", "invalid-endpoint-that-does-not-exist", "test-bucket", "key", "secret")
	if err == nil {
		t.Fatal("expected error for unreachable S3 endpoint")
	}
	if err.Error() == "s3 storage not implemented yet" {
		t.Fatal("s3 stub should be replaced with real implementation")
	}
}

func TestNewStore_LocalBackend(t *testing.T) {
	store, err := NewStore("local", t.TempDir(), "", "", "", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	_, ok := store.(*LocalStore)
	if !ok {
		t.Fatal("expected LocalStore type")
	}
}

func TestNewStore_UnknownBackend(t *testing.T) {
	_, err := NewStore("unknown", "", "", "", "", "")
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}
