package broadcast

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"
)

// Event is a generic event for SSE broadcasting.
type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Hub manages SSE subscribers and cross-instance event broadcasting.
type Hub struct {
	mu         sync.RWMutex
	subs       map[string]chan Event
	pubsub     *redis.PubSub
	rdb        *redis.Client
	channel    string
	localOnly  bool
}

func NewHub(rdb *redis.Client) *Hub {
	h := &Hub{
		subs:      make(map[string]chan Event),
		rdb:       rdb,
		channel:   "minicc:events",
		localOnly: rdb == nil,
	}

	if !h.localOnly {
		h.pubsub = rdb.Subscribe(context.Background(), h.channel)
		go h.redisListener()
	}

	return h
}

func (h *Hub) Subscribe(id string) chan Event {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan Event, 64)
	h.subs[id] = ch
	return ch
}

func (h *Hub) Unsubscribe(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ch, ok := h.subs[id]; ok {
		close(ch)
		delete(h.subs, id)
	}
}

func (h *Hub) Publish(event Event) {
	data, _ := json.Marshal(event)

	// Local fan-out
	h.mu.RLock()
	for id, ch := range h.subs {
		select {
		case ch <- event:
		default:
			slog.Warn("subscriber slow, dropping", "id", id)
		}
	}
	h.mu.RUnlock()

	// Cross-instance via Redis
	if !h.localOnly {
		h.rdb.Publish(context.Background(), h.channel, data)
	}
}

func (h *Hub) redisListener() {
	ch := h.pubsub.Channel()
	for msg := range ch {
		var event Event
		if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
			continue
		}

		// Re-broadcast to local subscribers (skip if we originated it)
		h.mu.RLock()
		for _, subCh := range h.subs {
			select {
			case subCh <- event:
			default:
			}
		}
		h.mu.RUnlock()
	}
}

func (h *Hub) Close() {
	if h.pubsub != nil {
		h.pubsub.Close()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, ch := range h.subs {
		close(ch)
		delete(h.subs, id)
	}
}

// SSE channel format: JSON lines
func FormatSSE(event Event) string {
	data, _ := json.Marshal(event)
	return "data: " + string(data) + "\n\n"
}
