package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════
// Response Cache — caches exact LLM responses (non-streaming)
// ══════════════════════════════════════════════════════════════════════════

type CacheEntry struct {
	Response  *Response     `json:"response"`
	CreatedAt time.Time     `json:"created_at"`
	TTL       time.Duration `json:"ttl"`
}

type ResponseCache struct {
	mu    sync.RWMutex
	store map[string]*CacheEntry
	ttl   time.Duration
	hit   int64
	miss  int64
}

func NewResponseCache(ttl time.Duration) *ResponseCache {
	return &ResponseCache{
		store: make(map[string]*CacheEntry),
		ttl:   ttl,
	}
}

func (c *ResponseCache) Get(req *Request) *Response {
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := c.hash(req)
	entry, ok := c.store[key]
	if !ok { c.miss++; return nil }
	if time.Since(entry.CreatedAt) > entry.TTL {
		delete(c.store, key)
		c.miss++
		return nil
	}
	c.hit++
	resp := *entry.Response
	resp.Cached = true
	return &resp
}

func (c *ResponseCache) Set(req *Request, resp *Response) {
	if resp == nil { return }
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.hash(req)
	c.store[key] = &CacheEntry{Response: resp, CreatedAt: time.Now(), TTL: c.ttl}
}

func (c *ResponseCache) hash(req *Request) string {
	h := sha256.New()
	h.Write([]byte(req.Model))
	if len(req.Tools) > 0 {
		sorted := make([]ToolDef, len(req.Tools))
		copy(sorted, req.Tools)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
		toolJSON, _ := json.Marshal(sorted)
		h.Write([]byte("tools:" + string(toolJSON) + "\n"))
	}
	for _, m := range req.Messages {
		h.Write([]byte(m.Role))
		h.Write([]byte(m.Content))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *ResponseCache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	total := c.hit + c.miss
	rate := 0.0
	if total > 0 { rate = float64(c.hit) / float64(total) * 100 }
	return map[string]interface{}{
		"size": len(c.store), "hits": c.hit, "misses": c.miss,
		"hit_rate": fmt.Sprintf("%.1f%%", rate),
	}
}

// ══════════════════════════════════════════════════════════════════════════
// Prefix Cache — tracks deterministic request prefixes for cost savings
// DeepSeek/OpenAI bill cached prefix tokens at ~1/5 the cost.
// Key: deterministic + append-only context (NO reordering, NO variable content)
// ══════════════════════════════════════════════════════════════════════════

// PrefixCacheEntry tracks a request hash and its token savings.
type PrefixCacheEntry struct {
	PrefixHash string    `json:"prefix_hash"`
	Tokens     int       `json:"tokens"`
	LastSeen   time.Time `json:"last_seen"`
}

type PrefixCache struct {
	mu         sync.RWMutex
	entries    map[string]*PrefixCacheEntry
	hitTokens  int64
	missTokens int64
	hitCount   int64
	missCount  int64
}

func NewPrefixCache() *PrefixCache {
	return &PrefixCache{entries: make(map[string]*PrefixCacheEntry)}
}

// RequestHash computes a DETERMINISTIC hash of the request prefix.
// System prompt first → tools (sorted) → messages in append-only order.
func RequestHash(req *Request) string {
	h := sha256.New()
	systemContent := ""
	messagesToHash := make([]Message, 0)
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemContent = m.Content
		} else {
			messagesToHash = append(messagesToHash, m)
		}
	}
	h.Write([]byte("system:" + systemContent + "\n"))
	if len(req.Tools) > 0 {
		sortedTools := make([]ToolDef, len(req.Tools))
		copy(sortedTools, req.Tools)
		sort.Slice(sortedTools, func(i, j int) bool { return sortedTools[i].Name < sortedTools[j].Name })
		toolJSON, _ := json.Marshal(sortedTools)
		h.Write([]byte("tools:" + string(toolJSON) + "\n"))
	}
	for _, m := range messagesToHash {
		line := fmt.Sprintf("%s|%s", m.Role, m.Content)
		if m.ToolCallID != "" { line += "|tc:" + m.ToolCallID }
		if len(m.ToolCalls) > 0 {
			tcJSON, _ := json.Marshal(m.ToolCalls)
			line += "|calls:" + string(tcJSON)
		}
		h.Write([]byte(line + "\n"))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (pc *PrefixCache) IsPrefixCached(req *Request) (cachedTokens int, newTokens int) {
	hash := RequestHash(req)
	totalEstimate := 0
	for _, m := range req.Messages { totalEstimate += len(m.Content) / 4 }
	pc.mu.RLock()
	entry, exists := pc.entries[hash]
	pc.mu.RUnlock()
	if exists {
		entry.LastSeen = time.Now()
		pc.mu.Lock()
		pc.hitCount++
		pc.hitTokens += int64(entry.Tokens)
		pc.mu.Unlock()
		newContent := 0
		if len(req.Messages) > 0 {
			lastMsg := req.Messages[len(req.Messages)-1]
			newContent = len(lastMsg.Content) / 4
		}
		for _, m := range req.Messages {
			for _, tc := range m.ToolCalls { newContent += len(tc.Arguments) / 4 }
		}
		return entry.Tokens, newContent
	}
	cachedTokens = totalEstimate - (len(req.Messages) * 2)
	if cachedTokens < 0 { cachedTokens = 0 }
	pc.mu.Lock()
	pc.entries[hash] = &PrefixCacheEntry{Tokens: cachedTokens, LastSeen: time.Now()}
	pc.missCount++
	pc.missTokens += int64(totalEstimate)
	pc.mu.Unlock()
	return 0, totalEstimate
}

func (pc *PrefixCache) Stats() map[string]interface{} {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	total := pc.hitCount + pc.missCount
	hitRate := 0.0
	if total > 0 { hitRate = float64(pc.hitCount) / float64(total) * 100 }
	totalTokens := pc.hitTokens + pc.missTokens
	savings := 0.0
	if totalTokens > 0 { savings = float64(pc.hitTokens) * 0.8 }
	return map[string]interface{}{
		"hits": pc.hitCount, "misses": pc.missCount,
		"hit_rate": fmt.Sprintf("%.1f%%", hitRate),
		"cached_tokens": pc.hitTokens, "uncached_tokens": pc.missTokens,
		"est_savings": fmt.Sprintf("%.0f%%", savings/float64(totalTokens)*100),
		"entries": len(pc.entries),
	}
}

// DeterministicSystemPrompt — NO timestamps, NO variable content.
// Any change here invalidates ALL prefix caches.
func DeterministicSystemPrompt() string {
	return "You are MiniCC V2, an enterprise AI agent with access to tools. " +
		"Follow these rules:\n" +
		"1. When a task requires a tool, call it using the available functions.\n" +
		"2. Always explain what you are doing before and after tool calls.\n" +
		"3. Use the results from tools to inform your response.\n" +
		"4. If a tool returns an error, report it clearly.\n" +
		"5. Never refuse to use a tool when it would help the user."
}

func (pc *PrefixCache) LogMetrics(interval time.Duration) {
	go func() {
		for {
			time.Sleep(interval)
			pc.mu.RLock()
			total := pc.hitCount + pc.missCount
			rate := 0.0
			if total > 0 { rate = float64(pc.hitCount) / float64(total) * 100 }
			pc.mu.RUnlock()
			slog.Info("prefix cache", "hit_rate", fmt.Sprintf("%.1f%%", rate),
				"hits", pc.hitCount, "misses", pc.missCount, "entries", len(pc.entries))
		}
	}()
}

// ══════════════════════════════════════════════════════════════════════════
// Circuit Breaker — prevents cascading failures
// ══════════════════════════════════════════════════════════════════════════

type CircuitState int
const (
	StateClosed   CircuitState = iota
	StateOpen
	StateHalfOpen
)

type CircuitBreaker struct {
	mu           sync.RWMutex
	state        CircuitState
	failCount    int
	threshold    int
	halfOpenTime time.Duration
	lastFail     time.Time
}

func NewCircuitBreaker(threshold int, halfOpen time.Duration) *CircuitBreaker {
	return &CircuitBreaker{state: StateClosed, threshold: threshold, halfOpenTime: halfOpen}
}

func (cb *CircuitBreaker) Allow(name string) bool {
	cb.mu.RLock()
	state, lastFail := cb.state, cb.lastFail
	cb.mu.RUnlock()
	switch state {
	case StateClosed: return true
	case StateOpen:
		if time.Since(lastFail) > cb.halfOpenTime {
			cb.mu.Lock(); cb.state = StateHalfOpen; cb.mu.Unlock()
			slog.Info("circuit half-open", "provider", name)
			return true
		}
		return false
	case StateHalfOpen: return true
	default: return true
	}
}

func (cb *CircuitBreaker) Success(name string) {
	cb.mu.Lock(); defer cb.mu.Unlock()
	if cb.state == StateHalfOpen { slog.Info("circuit closed (recovered)", "provider", name) }
	cb.state = StateClosed; cb.failCount = 0
}

func (cb *CircuitBreaker) Fail(name string) {
	cb.mu.Lock(); defer cb.mu.Unlock()
	cb.failCount++; cb.lastFail = time.Now()
	if cb.failCount >= cb.threshold {
		cb.state = StateOpen
		slog.Warn("circuit opened", "provider", name, "failures", cb.failCount)
	}
}
