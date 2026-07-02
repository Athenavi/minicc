package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// CacheEntry for LLM response caching.
type CacheEntry struct {
	Response  *Response `json:"response"`
	CreatedAt time.Time `json:"created_at"`
	TTL       time.Duration `json:"ttl"`
}

// ResponseCache caches LLM responses based on request hash.
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
	if !ok {
		c.miss++
		return nil
	}

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
	if resp == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.hash(req)
	c.store[key] = &CacheEntry{
		Response:  resp,
		CreatedAt: time.Now(),
		TTL:       c.ttl,
	}
}

func (c *ResponseCache) hash(req *Request) string {
	// Only hash messages + model, not temperature/max_tokens
	h := sha256.New()
	h.Write([]byte(req.Model))
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
	if total > 0 {
		rate = float64(c.hit) / float64(total) * 100
	}

	return map[string]interface{}{
		"size":      len(c.store),
		"hits":      c.hit,
		"misses":    c.miss,
		"hit_rate":  fmt.Sprintf("%.1f%%", rate),
	}
}

// ─── Circuit Breaker ──────────────────────────────────────────────────────

type CircuitState int

const (
	StateClosed   CircuitState = iota // normal operation
	StateOpen                         // failing, reject fast
	StateHalfOpen                      // probe
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
	return &CircuitBreaker{
		state:        StateClosed,
		threshold:    threshold,
		halfOpenTime: halfOpen,
	}
}

func (cb *CircuitBreaker) Allow(name string) bool {
	cb.mu.RLock()
	state := cb.state
	lastFail := cb.lastFail
	cb.mu.RUnlock()

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(lastFail) > cb.halfOpenTime {
			cb.mu.Lock()
			cb.state = StateHalfOpen
			cb.mu.Unlock()
			slog.Info("circuit half-open", "provider", name)
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return true
	}
}

func (cb *CircuitBreaker) Success(name string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		slog.Info("circuit closed (recovered)", "provider", name)
	}
	cb.state = StateClosed
	cb.failCount = 0
}

func (cb *CircuitBreaker) Fail(name string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failCount++
	cb.lastFail = time.Now()
	if cb.failCount >= cb.threshold {
		cb.state = StateOpen
		slog.Warn("circuit opened", "provider", name, "failures", cb.failCount)
	}
}
