package monitor

import (
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ── Duration Histogram ──

// Histogram tracks duration percentiles (p50/p95/p99) for operations.
// Uses a fixed-size ring buffer to avoid slice reallocations on every Record call.
type Histogram struct {
	mu   sync.Mutex
	name string
	data []time.Duration
	pos  int // next write position
	count int // number of samples recorded (up to max)
	max  int // capacity
}

func NewHistogram(name string, maxSamples int) *Histogram {
	return &Histogram{
		name: name,
		data: make([]time.Duration, maxSamples),
		max:  maxSamples,
	}
}

// Record adds a duration to the histogram. O(1) with ring buffer.
func (h *Histogram) Record(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data[h.pos] = d
	h.pos = (h.pos + 1) % h.max
	if h.count < h.max {
		h.count++
	}
}

// Snapshot returns current histogram statistics. Copies data out for sorting.
func (h *Histogram) Snapshot() map[string]interface{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.count == 0 {
		return map[string]interface{}{"name": h.name, "count": 0}
	}
	// Copy valid samples for sorting
	sorted := make([]time.Duration, h.count)
	// Data is written in a ring; the valid window is from pos-count to pos (mod max)
	start := h.pos - h.count
	if start < 0 {
		start += h.max
	}
	for i := 0; i < h.count; i++ {
		sorted[i] = h.data[(start+i)%h.max]
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return map[string]interface{}{
		"name":  h.name,
		"count": len(sorted),
		"p50":   sorted[len(sorted)*50/100].Milliseconds(),
		"p95":   sorted[len(sorted)*95/100].Milliseconds(),
		"p99":   sorted[len(sorted)*99/100].Milliseconds(),
		"max":   sorted[len(sorted)-1].Milliseconds(),
	}
}

var (
	LLMHistogram   = NewHistogram("llm", 1000)
	ToolHistogram  = NewHistogram("tool", 1000)
	RequestHistogram = NewHistogram("request", 1000)
)

// Metrics holds simple counters for monitoring.
type Metrics struct {
	RequestsTotal   atomic.Int64
	RequestsActive  atomic.Int64
	LLMCallsTotal   atomic.Int64
	LLMErrorsTotal  atomic.Int64
	ToolCallsTotal  atomic.Int64
	ToolErrorsTotal atomic.Int64
	StartTime       time.Time
}

var Global = &Metrics{StartTime: time.Now()}

func IncRequests() {
	Global.RequestsTotal.Add(1)
	Global.RequestsActive.Add(1)
}

func DecRequests() {
	Global.RequestsActive.Add(-1)
}

func IncLLMCall() {
	Global.LLMCallsTotal.Add(1)
}

func RecordLLMDuration(d time.Duration) {
	LLMHistogram.Record(d)
}

func IncLLMError() {
	Global.LLMErrorsTotal.Add(1)
}

func IncToolCall() {
	Global.ToolCallsTotal.Add(1)
}

func RecordToolDuration(d time.Duration) {
	ToolHistogram.Record(d)
}

func IncToolError() {
	Global.ToolErrorsTotal.Add(1)
}

var extraStatsMu sync.RWMutex
var extraStatsFuncs []func() map[string]interface{}

// RegisterExtraStats 允许外部包注册额外的统计信息提供者（如 DB 连接池、缓存等）。
func RegisterExtraStats(fn func() map[string]interface{}) {
	extraStatsMu.Lock()
	defer extraStatsMu.Unlock()
	extraStatsFuncs = append(extraStatsFuncs, fn)
}

func Snapshot() map[string]interface{} {
	snap := map[string]interface{}{
		"requests_total":   Global.RequestsTotal.Load(),
		"requests_active":  Global.RequestsActive.Load(),
		"llm_calls":        Global.LLMCallsTotal.Load(),
		"llm_errors":       Global.LLMErrorsTotal.Load(),
		"tool_calls":       Global.ToolCallsTotal.Load(),
		"tool_errors":      Global.ToolErrorsTotal.Load(),
		"uptime_seconds":   time.Since(Global.StartTime).Seconds(),
		"started_at":       Global.StartTime.Format(time.RFC3339),
	}
	extraStatsMu.RLock()
	for _, fn := range extraStatsFuncs {
		if m := fn(); m != nil {
			for k, v := range m {
				snap[k] = v
			}
		}
	}
	extraStatsMu.RUnlock()
	return snap
}

func Init() {
	InitWithSpanStore()
	slog.Info("monitor initialized", "started_at", Global.StartTime.Format(time.RFC3339))
}

// ── Per-session cost tracking (in-memory) ──

var (
	costMu          sync.Mutex
	costBySession    = make(map[string]*SessionCost)
	maxSessionCosts  = 10000
	sessionCostRing  = make([]string, 10000) // ring buffer for FIFO eviction
	sessionCostHead  = 0                      // next eviction position
	sessionCostCount = 0                      // number of entries in ring
)

// SessionCost tracks token usage for a session.
type SessionCost struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalCalls   int `json:"total_calls"`
}

// RecordSessionUsage records LLM token usage for a session.
func RecordSessionUsage(sessionID string, inputTokens, outputTokens int) {
	costMu.Lock()
	defer costMu.Unlock()
	s, ok := costBySession[sessionID]
	if !ok {
		// Evict oldest if at capacity
		if len(costBySession) >= maxSessionCosts {
			oldest := sessionCostRing[sessionCostHead]
			delete(costBySession, oldest)
			sessionCostHead = (sessionCostHead + 1) % maxSessionCosts
			sessionCostCount--
		}
		s = &SessionCost{}
		costBySession[sessionID] = s
		// Add to ring buffer at tail
		tail := (sessionCostHead + sessionCostCount) % maxSessionCosts
		sessionCostRing[tail] = sessionID
		sessionCostCount++
	}
	s.InputTokens += inputTokens
	s.OutputTokens += outputTokens
	s.TotalCalls++
}

// GetSessionCost returns a snapshot of the cost info for a session.
func GetSessionCost(sessionID string) SessionCost {
	costMu.Lock()
	defer costMu.Unlock()
	if s, ok := costBySession[sessionID]; ok {
		return *s
	}
	return SessionCost{}
}

// AllSessionCosts returns snapshots of all tracked session costs.
func AllSessionCosts() map[string]SessionCost {
	costMu.Lock()
	defer costMu.Unlock()
	result := make(map[string]SessionCost, len(costBySession))
	for k, v := range costBySession {
		result[k] = *v
	}
	return result
}

// SnapshotSessionCosts returns a JSON-safe summary of all session costs.
func SnapshotSessionCosts() []map[string]interface{} {
	all := AllSessionCosts()
	result := make([]map[string]interface{}, 0, len(all))
	for sid, cost := range all {
		result = append(result, map[string]interface{}{
			"session_id":    sid,
			"input_tokens":  cost.InputTokens,
			"output_tokens": cost.OutputTokens,
			"total_calls":   cost.TotalCalls,
		})
	}
	return result
}
