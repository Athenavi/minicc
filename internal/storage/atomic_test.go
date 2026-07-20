package storage

import (
	"context"
	"sync"
	"testing"
)

func TestAtomicStore_DelegatesToLocal(t *testing.T) {
	root := t.TempDir()
	local := NewLocalStore(root)
	atomic := NewAtomicStore(local)

	err := atomic.Write(context.Background(), "test.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := atomic.Read(context.Background(), "test.txt")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(data))
	}
}

func TestAtomicStore_Swap(t *testing.T) {
	root1 := t.TempDir()
	root2 := t.TempDir()

	local1 := NewLocalStore(root1)
	local2 := NewLocalStore(root2)

	atomic := NewAtomicStore(local1)

	atomic.Write(context.Background(), "a.txt", []byte("first"))

	atomic.Swap(local2)

	atomic.Write(context.Background(), "b.txt", []byte("second"))

	_, err := local1.Read(context.Background(), "a.txt")
	if err != nil {
		t.Fatal("expected a.txt in first backend")
	}
	_, err = local1.Read(context.Background(), "b.txt")
	if err == nil {
		t.Fatal("expected b.txt to NOT be in first backend")
	}

	_, err = local2.Read(context.Background(), "b.txt")
	if err != nil {
		t.Fatal("expected b.txt in second backend")
	}
	_, err = local2.Read(context.Background(), "a.txt")
	if err == nil {
		t.Fatal("expected a.txt to NOT be in second backend")
	}
}

func TestAtomicStore_Backend(t *testing.T) {
	root := t.TempDir()
	local := NewLocalStore(root)
	atomic := NewAtomicStore(local)

	if atomic.Backend() != "local" {
		t.Fatalf("expected 'local', got %q", atomic.Backend())
	}
}

func TestAtomicStore_ConcurrentAccess(t *testing.T) {
	root := t.TempDir()
	local := NewLocalStore(root)
	atomic := NewAtomicStore(local)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			atomic.Write(context.Background(), "file.txt", []byte("data"))
		}(i)
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.Swap(NewLocalStore(t.TempDir()))
		}()
	}
	wg.Wait()
}

func TestAtomicStore_List(t *testing.T) {
	root := t.TempDir()
	local := NewLocalStore(root)
	atomic := NewAtomicStore(local)

	atomic.Write(context.Background(), "a.txt", []byte("a"))
	atomic.Write(context.Background(), "b.txt", []byte("b"))

	files, err := atomic.List(context.Background(), ".")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("expected at least 2 files, got %d", len(files))
	}
}

func TestAtomicStore_Delete(t *testing.T) {
	root := t.TempDir()
	local := NewLocalStore(root)
	atomic := NewAtomicStore(local)

	atomic.Write(context.Background(), "del.txt", []byte("bye"))
	err := atomic.Delete(context.Background(), "del.txt")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = atomic.Read(context.Background(), "del.txt")
	if err == nil {
		t.Fatal("expected file to be deleted")
	}
}
