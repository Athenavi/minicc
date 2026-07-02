package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type TaskType string

const (
	TaskLLM   TaskType = "llm"
	TaskTool  TaskType = "tool"
	TaskBatch TaskType = "batch"
)

type Task struct {
	ID         string            `json:"id"`
	UserID     string            `json:"user_id"`
	Type       TaskType          `json:"type"`
	Status     string            `json:"status"` // pending / running / completed / failed
	Payload    map[string]interface{} `json:"payload"`
	Priority   int               `json:"priority"`
	MaxRetries int               `json:"max_retries"`
	Retries    int               `json:"retries"`
	CreatedAt  time.Time         `json:"created_at"`
}

type Queue struct {
	rdb    *redis.Client
	stream string
	group  string
}

func New(rdb *redis.Client, streamName, groupName string) *Queue {
	return &Queue{
		rdb:    rdb,
		stream: streamName,
		group:  groupName,
	}
}

func (q *Queue) Enqueue(ctx context.Context, task *Task) (string, error) {
	if q.rdb == nil {
		return "", fmt.Errorf("redis not available")
	}
	if task.ID == "" {
		task.ID = fmt.Sprintf("task_%d", time.Now().UnixNano())
	}

	data, err := json.Marshal(task)
	if err != nil {
		return "", fmt.Errorf("marshal task: %w", err)
	}

	// Create consumer group if not exists (idempotent)
	q.rdb.XGroupCreateMkStream(ctx, q.stream, q.group, "0")

	msgID, err := q.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: q.stream,
		Values: map[string]interface{}{
			"type": string(task.Type),
			"data": string(data),
		},
		MaxLen: 100000,
		Approx: true,
	}).Result()
	if err != nil {
		return "", fmt.Errorf("enqueue: %w", err)
	}

	slog.Debug("task enqueued", "id", task.ID, "type", task.Type, "msg_id", msgID)
	return msgID, nil
}

func (q *Queue) Dequeue(ctx context.Context, consumerID string, timeout time.Duration) (*Task, string, error) {
	if q.rdb == nil {
		return nil, "", nil
	}
	results, err := q.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    q.group,
		Consumer: consumerID,
		Streams:  []string{q.stream, ">"},
		Count:    1,
		Block:    timeout,
	}).Result()
	if err != nil {
		return nil, "", fmt.Errorf("dequeue: %w", err)
	}

	if len(results) == 0 || len(results[0].Messages) == 0 {
		return nil, "", nil
	}

	msg := results[0].Messages[0]
	data, ok := msg.Values["data"].(string)
	if !ok {
		return nil, msg.ID, fmt.Errorf("invalid message data")
	}

	var task Task
	if err := json.Unmarshal([]byte(data), &task); err != nil {
		return nil, msg.ID, fmt.Errorf("unmarshal task: %w", err)
	}

	return &task, msg.ID, nil
}

func (q *Queue) Ack(ctx context.Context, msgID string) error {
	if q.rdb == nil {
		return nil
	}
	return q.rdb.XAck(ctx, q.stream, q.group, msgID).Err()
}

func (q *Queue) Nack(ctx context.Context, msgID string) error {
	if q.rdb == nil {
		return nil
	}
	// Move to dead letter stream
	return q.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: q.stream + ":dead",
		Values: map[string]interface{}{"original_id": msgID},
	}).Err()
}

func (q *Queue) Len(ctx context.Context) (int64, error) {
	if q.rdb == nil {
		return -1, nil
	}
	return q.rdb.XLen(ctx, q.stream).Result()
}
