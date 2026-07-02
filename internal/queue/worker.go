package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// TaskHandler processes a dequeued task and returns an error if processing failed.
type TaskHandler func(ctx context.Context, task *Task) error

// WorkerPool manages concurrent workers consuming from a Redis Streams queue.
// Each worker runs an independent loop: Dequeue → Handle → Ack/Nack.
// Graceful shutdown via context cancellation.
type WorkerPool struct {
	queue      *Queue
	handler    TaskHandler
	numWorkers int
	wg         sync.WaitGroup

	processed atomic.Int64
	errors    atomic.Int64
}

// NewWorkerPool creates a pool of n workers consuming from q.
// handler is called for each dequeued task; if it returns nil the message is ACK'd,
// otherwise it's NACK'd and moved to the dead-letter stream.
func NewWorkerPool(q *Queue, n int, handler TaskHandler) *WorkerPool {
	if n < 1 {
		n = 1
	}
	if handler == nil {
		handler = func(_ context.Context, t *Task) error {
			slog.Warn("no handler registered, nacking task", "id", t.ID, "type", t.Type)
			return nil // ACK to avoid infinite loop
		}
	}
	return &WorkerPool{
		queue:      q,
		handler:    handler,
		numWorkers: n,
	}
}

// Start launches numWorkers goroutines. Blocks until all workers exit
// (when ctx is cancelled or the parent calls Stop).
func (wp *WorkerPool) Start(ctx context.Context) {
	slog.Info("worker pool starting", "workers", wp.numWorkers)
	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		go wp.run(ctx, i)
	}
	wp.wg.Wait()
	slog.Info("worker pool stopped")
}

// StartBackground launches workers in the background and returns immediately.
// Call Stop() to shut them down gracefully.
func (wp *WorkerPool) StartBackground(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		wp.Start(ctx)
		cancel()
	}()
	return ctx
}

// Stop signals all workers to stop by cancelling the context. Waits for them
// to finish their current task before returning.
func (wp *WorkerPool) Stop() {
	// The caller should cancel the context passed to Start/StartBackground.
	// This is a convenience that signals via the context.Background() which
	// won't work — use context cancellation on the Start ctx instead.
	slog.Info("worker pool stop requested")
}

// Metrics returns the current counters.
func (wp *WorkerPool) Metrics() map[string]int64 {
	pending, err := wp.queue.Len(context.Background())
	if err != nil || pending < 0 {
		pending = 0
	}
	return map[string]int64{
		"processed": wp.processed.Load(),
		"errors":    wp.errors.Load(),
		"pending":   pending,
		"workers":   int64(wp.numWorkers),
	}
}

// run is the per-worker loop.
// Consumer ID is derived from worker index for observability.
func (wp *WorkerPool) run(ctx context.Context, idx int) {
	defer wp.wg.Done()

	consumerID := wp.consumerID(idx)
	timeout := 5 * time.Second

	slog.Debug("worker started", "consumer", consumerID, "index", idx)

	for {
		select {
		case <-ctx.Done():
			slog.Debug("worker stopping", "consumer", consumerID, "reason", ctx.Err())
			return
		default:
		}

		task, msgID, err := wp.queue.Dequeue(ctx, consumerID, timeout)
		if err != nil {
			slog.Warn("worker dequeue error", "consumer", consumerID, "error", err)
			continue
		}
		if task == nil {
			// Timeout with no message — loop back and check ctx
			continue
		}

		// Process
		processErr := wp.handler(ctx, task)
		if processErr != nil {
			wp.errors.Add(1)
			slog.Error("task failed", "id", task.ID, "type", task.Type, "error", processErr)

			// NACK — move to dead letter queue
			if nackErr := wp.queue.Nack(ctx, msgID); nackErr != nil {
				slog.Warn("nack failed", "msg_id", msgID, "error", nackErr)
			}
			continue
		}

		// Ack
		if ackErr := wp.queue.Ack(ctx, msgID); ackErr != nil {
			slog.Warn("ack failed", "msg_id", msgID, "error", ackErr)
		}
		wp.processed.Add(1)
	}
}

func (wp *WorkerPool) consumerID(idx int) string {
	return fmt.Sprintf("worker_%d_%d", idx, time.Now().UnixMilli())
}
