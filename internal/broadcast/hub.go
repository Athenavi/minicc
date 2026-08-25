package broadcast

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/athenavi/chiron/internal/db"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// slowSubSem 闄愬埗鎱㈣闃呰€呴噸璇?goroutine 鏁伴噺锛圥1 淇锛氫簨浠堕鏆翠笅闃叉
// goroutine 鏃犵晫鍫嗙Н瀵艰嚧 DoS锛夈€傝秴鍑轰笂闄愭椂鐩存帴涓㈠純浜嬩欢锛圫SE 鍙噸杩炶ˉ鍙戯級銆?var slowSubSem = make(chan struct{}, 512)

// Event is a generic event for SSE broadcasting.
type Event struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	SessionID string      `json:"session_id,omitempty"`
}

// Hub manages SSE subscribers and cross-instance event broadcasting.
type Hub struct {
	mu         sync.RWMutex
	subs       map[string]chan Event
	closed     bool
	pubsub     *redis.PubSub
	rdb        db.RedisClient
	channel    string
	localOnly  bool
	instanceID string
}

// envelope wraps an event with its originating instance for deduplication.
type envelope struct {
	Origin string `json:"origin"`
	Event  Event  `json:"event"`
}

func NewHub(rdb db.RedisClient) *Hub {
	h := &Hub{
		subs:       make(map[string]chan Event),
		rdb:        rdb,
		channel:    "chiron:events",
		localOnly:  rdb == nil,
		instanceID: uuid.New().String(),
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

	if oldCh, ok := h.subs[id]; ok {
		close(oldCh)
	}
	ch := make(chan Event, 256)
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
	// Local fan-out with goroutine per slow subscriber (non-blocking, no dropping)
	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return
	}
	for _, ch := range h.subs {
		select {
		case ch <- event:
				default:
			// Slow subscriber: spawn goroutine so fast subscribers aren't blocked
			select {
			case slowSubSem <- struct{}{}:
				go func(c chan Event) {
					defer func() {
						if r := recover(); r != nil {
							// channel 宸插叧闂紝涓㈠純浜嬩欢鍗冲彲
						}
						<-slowSubSem
					}()
					timer := time.NewTimer(3 * time.Second)
					defer timer.Stop()
					select {
					case c <- event:
					case <-timer.C:
						slog.Warn("subscriber too slow, dropping event after 3s timeout")
					}
				}(ch)
			default:
				slog.Warn("too many slow subscribers, dropping event")
			}
		}
	}
	h.mu.RUnlock()

	// Cross-instance via Redis
	if !h.localOnly {
		env := envelope{Origin: h.instanceID, Event: event}
		data, err := json.Marshal(env)
		if err != nil {
			slog.Error("publish: failed to marshal envelope", "error", err)
			return
		}
		if err := h.rdb.Publish(context.Background(), h.channel, data).Err(); err != nil {
			slog.Error("redis publish failed", "error", err)
		}
	}
}

func (h *Hub) redisListener() {
	ch := h.pubsub.Channel()
	for msg := range ch {
		var env envelope
		if err := json.Unmarshal([]byte(msg.Payload), &env); err != nil {
			continue
		}

		// Skip events that originated from this instance (already delivered locally)
		if env.Origin == h.instanceID {
			continue
		}

		h.mu.RLock()
		if h.closed {
			h.mu.RUnlock()
			return
		}
		for _, subCh := range h.subs {
			select {
			case subCh <- env.Event:
			default:
				// Use semaphore to limit goroutine spawning (same pattern as Publish)
				select {
				case slowSubSem <- struct{}{}:
					go func(c chan Event) {
						defer func() {
							if r := recover(); r != nil {
								// channel closed, discard
							}
							<-slowSubSem
						}()
						timer := time.NewTimer(3 * time.Second)
						defer timer.Stop()
						select {
						case c <- env.Event:
						case <-timer.C:
							slog.Warn("subscriber slow, dropping after 3s timeout")
						}
					}(subCh)
				default:
					slog.Warn("too many slow subscribers (redis), dropping event")
				}
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
	h.closed = true
	for id, ch := range h.subs {
		close(ch)
		delete(h.subs, id)
	}
}

// SSE channel format: JSON lines
func FormatSSE(event Event) string {
	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("format SSE: failed to marshal event", "error", err)
		return "data: {\"error\":\"marshal failed\"}\n\n"
	}
	return "data: " + string(data) + "\n\n"
}
