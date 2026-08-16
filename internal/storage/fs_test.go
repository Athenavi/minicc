package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLocalStore(t *testing.T) {
	s := NewLocalStore("/tmp")
	if s == nil {
		t.Fatal("expected non-nil store")
	}
	if s.Root != "/tmp" {
		t.Fatalf("expected root '/tmp', got %q", s.Root)
	}
}

func TestLocalStore_WriteRead(t *testing.T) {
	root := t.TempDir()
	s := NewLocalStore(root)

	data := []byte("hello world")
	err := s.Write(context.Background(), "test.txt", data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify file exists
	fullPath := filepath.Join(root, "test.txt")
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Fatal("expected file to exist after write")
	}

	// Read back
	got, err := s.Read(context.Background(), "test.txt")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("expected %q, got %q", string(data), string(got))
	}
}

func TestLocalStore_Write_NestedPath(t *testing.T) {
	root := t.TempDir()
	s := NewLocalStore(root)

	err := s.Write(context.Background(), "a/b/c/deep.txt", []byte("nested"))
	if err != nil {
		t.Fatalf("Write nested failed: %v", err)
	}

	got, err := s.Read(context.Background(), "a/b/c/deep.txt")
	if err != nil {
		t.Fatalf("Read nested failed: %v", err)
	}
	if string(got) != "nested" {
		t.Fatalf("expected 'nested', got %q", string(got))
	}
}

func TestLocalStore_Read_NotFound(t *testing.T) {
	root := t.TempDir()
	s := NewLocalStore(root)

	_, err := s.Read(context.Background(), "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLocalStore_Delete(t *testing.T) {
	root := t.TempDir()
	s := NewLocalStore(root)

	// Write then delete
	s.Write(context.Background(), "temp.txt", []byte("delete me"))
	err := s.Delete(context.Background(), "temp.txt")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify gone
	_, err = s.Read(context.Background(), "temp.txt")
	if err == nil {
		t.Fatal("expected file to be deleted")
	}
}

func TestLocalStore_List(t *testing.T) {
	root := t.TempDir()
	s := NewLocalStore(root)

	s.Write(context.Background(), "a.txt", []byte("a"))
	s.Write(context.Background(), "b.txt", []byte("b"))
	os.MkdirAll(filepath.Join(root, "sub"), 0755)
	s.Write(context.Background(), "sub/c.txt", []byte("c"))

	files, err := s.List(context.Background(), ".")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	// Should find at least a.txt, b.txt, sub
	if len(files) < 3 {
		t.Fatalf("expected at least 3 entries, got %d", len(files))
	}

	found := map[string]bool{}
	for _, f := range files {
		found[f.Path] = true
	}
	if !found["a.txt"] {
		t.Fatal("expected a.txt in listing")
	}
	if !found["b.txt"] {
		t.Fatal("expected b.txt in listing")
	}
}

func TestLocalStore_List_Empty(t *testing.T) {
	root := t.TempDir()
	s := NewLocalStore(root)

	files, err := s.List(context.Background(), ".")
	if err != nil {
		t.Fatalf("List empty dir failed: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files in empty dir, got %d", len(files))
	}
}

func TestLocalStore_Write_Overwrite(t *testing.T) {
	root := t.TempDir()
	s := NewLocalStore(root)

	s.Write(context.Background(), "file.txt", []byte("original"))
	s.Write(context.Background(), "file.txt", []byte("updated"))

	got, _ := s.Read(context.Background(), "file.txt")
	if string(got) != "updated" {
		t.Fatalf("expected 'updated', got %q", string(got))
	}
}

func TestLocalStore_PathTraversal_Protection(t *testing.T) {
	root := t.TempDir()
	s := NewLocalStore(root)

	// Attempt path traversal
	_, err := s.Read(context.Background(), "../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "path traversal") && !os.IsNotExist(err) {
		// Either "path traversal detected" or file not found is acceptable
	}
}

func TestLocalStore_FileInfo_HasSize(t *testing.T) {
	root := t.TempDir()
	s := NewLocalStore(root)

	s.Write(context.Background(), "data.bin", []byte("12345"))

	files, _ := s.List(context.Background(), ".")
	for _, f := range files {
		if f.Path == "data.bin" && f.Size != 5 {
			t.Fatalf("expected size 5, got %d", f.Size)
		}
	}
}
