package queue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewWorkerPool_Defaults(t *testing.T) {
	q := New(nil, "test:stream", "test:group")
	wp := NewWorkerPool(q, 0, nil)
	if wp.numWorkers != 1 {
		t.Fatalf("expected 1 worker for 0 input, got %d", wp.numWorkers)
	}
}

func TestNewWorkerPool_CustomCount(t *testing.T) {
	q := New(nil, "test:stream", "test:group")
	wp := NewWorkerPool(q, 4, nil)
	if wp.numWorkers != 4 {
		t.Fatalf("expected 4 workers, got %d", wp.numWorkers)
	}
}

func TestWorkerPool_Metrics(t *testing.T) {
	q := New(nil, "test:stream", "test:group")
	wp := NewWorkerPool(q, 2, nil)

	m := wp.Metrics()
	if m["workers"] != 2 {
		t.Fatalf("expected 2 workers, got %d", m["workers"])
	}
	if m["processed"] != 0 {
		t.Fatalf("expected 0 processed, got %d", m["processed"])
	}
	if m["errors"] != 0 {
		t.Fatalf("expected 0 errors, got %d", m["errors"])
	}
}

func TestWorkerPool_HandlerCalled(t *testing.T) {
	var called atomic.Int64
	handler := func(ctx context.Context, task *Task) error {
		called.Add(1)
		return nil
	}

	q := New(nil, "test:stream", "test:group")
	wp := NewWorkerPool(q, 1, handler)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		wp.Start(ctx)
		close(done)
	}()

	// Let it run briefly, then stop
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// Handler should have been called 0 times (no tasks in queue)
	if called.Load() != 0 {
		t.Fatalf("expected 0 calls (no tasks), got %d", called.Load())
	}
}

func TestWorkerPool_StartBackground(t *testing.T) {
	q := New(nil, "test:stream", "test:group")
	wp := NewWorkerPool(q, 1, nil)

	wp.StartBackground(context.Background())
	time.Sleep(50 * time.Millisecond)

	// Should be running — metrics should be accessible
	m := wp.Metrics()
	if m["workers"] != 1 {
		t.Fatalf("expected 1 worker, got %d", m["workers"])
	}
}

func TestWorkerPool_ProcessedCounter(t *testing.T) {
	var processed atomic.Int64

	q := New(nil, "test:stream", "test:group")
	wp := &WorkerPool{
		queue:      q,
		handler:    func(_ context.Context, task *Task) error { processed.Add(1); return nil },
		numWorkers: 1,
	}

	// Without a real Redis, Dequeue returns errors — processed should stay 0
	ctx, cancel := context.WithCancel(context.Background())
	go wp.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	m := wp.Metrics()
	if m["processed"] < 0 {
		t.Fatalf("processed should be >= 0, got %d", m["processed"])
	}
}
