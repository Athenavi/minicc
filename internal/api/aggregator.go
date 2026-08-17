package api

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/athenavi/minicc/internal/broadcast"
)

// AggregationEvent represents a cross-workstation event that needs to be pushed to frontend
type AggregationEvent struct {
	Type        string      `json:"type"`         // agent_status_update | trace_span_append | workflow_progress | notification
	TenantID    string      `json:"tenant_id"`    // SaaS 租户隔离
	SessionID   string      `json:"session_id"`   // 对话会话 ID
	TraceID     string      `json:"trace_id"`     // 链路追踪 ID
	Workstation string      `json:"workstation"`  // dialogue | agent | workflow | skill | knowledge | plugin
	Payload     interface{} `json:"payload"`      // 具体事件数据
	Timestamp   int64       `json:"timestamp"`    // Unix 时间戳 (ms)
}

// Aggregator aggregates events from multiple workstations and pushes to SSE
type Aggregator struct {
	mu           sync.RWMutex
	eventBuffer  map[string]*EventBuffer // session_id -> buffer (断线重连)
	hub          *broadcast.Hub
	maxBufferLen int                       // 最大缓冲区长度 (默认 1000)
}

// EventBuffer stores recent events for reconnection
type EventBuffer struct {
	mu       sync.Mutex
	events   []AggregationEvent
	capacity int
}

func NewAggregator(hub *broadcast.Hub, maxBufferLen int) *Aggregator {
	if maxBufferLen <= 0 {
		maxBufferLen = 1000
	}
	
	a := &Aggregator{
		eventBuffer:  make(map[string]*EventBuffer),
		hub:          hub,
		maxBufferLen: maxBufferLen,
	}
	
	// 启动清理 goroutine (每 5 分钟清理过期 buffer)
	go a.cleanupLoop()
	
	return a
}

// Aggregate ingests an event from Python side and broadcasts to SSE clients
func (a *Aggregator) Aggregate(event AggregationEvent) {
	event.Timestamp = time.Now().UnixMilli()
	
	slog.Debug(
		"Aggregating event",
		"type", event.Type,
		"tenant", event.TenantID,
		"session", event.SessionID,
		"trace", event.TraceID,
		"workstation", event.Workstation,
	)
	
	// Store in buffer for reconnection
	if event.SessionID != "" {
		a.storeEvent(event)
	}
	
	// Convert to broadcast.Event and publish
	bcastEvent := broadcast.Event{
		Type:      "cross_workstation:" + event.Type,
		SessionID: event.SessionID,
		Data:      event,
	}
	
	a.hub.Publish(bcastEvent)
}

// storeEvent buffers the event for reconnection
func (a *Aggregator) storeEvent(event AggregationEvent) {
	a.mu.RLock()
	buffer, exists := a.eventBuffer[event.SessionID]
	a.mu.RUnlock()
	
	if !exists {
		a.mu.Lock()
		buffer = &EventBuffer{
			capacity: a.maxBufferLen,
		}
		a.eventBuffer[event.SessionID] = buffer
		a.mu.Unlock()
	}
	
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	
	// Append and trim if exceeded
	buffer.events = append(buffer.events, event)
	if len(buffer.events) > buffer.capacity {
		buffer.events = buffer.events[len(buffer.events)-buffer.capacity:]
	}
}

// getRecentEvents returns buffered events for reconnection
func (a *Aggregator) getRecentEvents(sessionID string, sinceMs int64) []AggregationEvent {
	a.mu.RLock()
	buffer, exists := a.eventBuffer[sessionID]
	a.mu.RUnlock()
	
	if !exists {
		return nil
	}
	
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	
	// Filter events by timestamp
	var result []AggregationEvent
	for _, event := range buffer.events {
		if event.Timestamp > sinceMs {
			result = append(result, event)
		}
	}
	
	return result
}

// cleanupLoop periodically removes expired event buffers (older than 1 hour)
func (a *Aggregator) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		a.mu.Lock()
		now := time.Now().UnixMilli()
		expireDuration := 1 * time.Hour // 1 小时过期
		
		for sessionID, buffer := range a.eventBuffer {
			buffer.mu.Lock()
			if len(buffer.events) == 0 {
				delete(a.eventBuffer, sessionID)
				buffer.mu.Unlock()
				continue
			}
			
			// Remove events older than expireDuration
			cutoff := now - expireDuration
			validIdx := 0
			for i, event := range buffer.events {
				if event.Timestamp > cutoff {
					if i != validIdx {
						buffer.events[validIdx] = event
					}
					validIdx++
				}
			}
			
			if validIdx == 0 {
				delete(a.eventBuffer, sessionID)
			} else {
				buffer.events = buffer.events[:validIdx]
			}
			buffer.mu.Unlock()
		}
		a.mu.Unlock()
	}
}

// FormatSSE converts an AggregationEvent to SSE format
func FormatSSE(event AggregationEvent) string {
	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("format SSE: failed to marshal event", "error", err)
		return "data: {\"error\":\"marshal failed\"}\n\n"
	}
	return "data: " + string(data) + "\n\n"
}

// BatchSend sends multiple events at once (more efficient)
func (a *Aggregator) BatchSend(events []AggregationEvent) {
	for _, event := range events {
		a.Aggregate(event)
	}
}

// GetStats returns current aggregator statistics
type AggregatorStats struct {
	TotalSessions  int       `json:"total_sessions"`
	TotalBuffers   int       `json:"total_buffers"`
	CleanupTime    string    `json:"cleanup_time"`
	LastCleanupAt  time.Time `json:"last_cleanup_at"`
}

func (a *Aggregator) GetStats() AggregatorStats {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	return AggregatorStats{
		TotalSessions: len(a.eventBuffer),
		TotalBuffers:  len(a.eventBuffer),
		CleanupTime:   "5m",
		LastCleanupAt: time.Now(),
	}
}

// MarshalJSON implements json.Marshaler
func (s AggregatorStats) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"total_sessions": s.TotalSessions,
		"total_buffers":  s.TotalBuffers,
		"cleanup_time":   s.CleanupTime,
		"last_cleanup":   s.LastCleanupAt.Format(time.RFC3339),
	})
}
