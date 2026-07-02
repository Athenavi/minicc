package llm

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter implements multi-dimensional token bucket rate limiting
// with Redis as the distributed counter backend.
//
// Three layers (all must pass):
//   - User-level:  requests/minute per user
//   - Model-level: requests/minute per model
//   - Global:      requests/minute across all users
type RateLimiter struct {
	rdb          *redis.Client
	userLimit    int64 // requests per minute per user
	modelLimit   int64 // requests per minute per model
	globalLimit  int64 // requests per minute globally
	localOnly    bool  // fallback to in-memory counting when Redis is nil
	localCounts  map[string]*window
	mu           sync.Mutex
}

type window struct {
	count    int64
	resetAt  time.Time
}

const rateLimitPrefix = "ratelimit:"

// NewRateLimiter creates a rate limiter. Pass nil for rdb to use local-only mode.
func NewRateLimiter(rdb *redis.Client, userRPM, modelRPM, globalRPM int64) *RateLimiter {
	if userRPM <= 0 {
		userRPM = 100
	}
	if modelRPM <= 0 {
		modelRPM = 1000
	}
	if globalRPM <= 0 {
		globalRPM = 10000
	}

	rl := &RateLimiter{
		rdb:         rdb,
		userLimit:   userRPM,
		modelLimit:  modelRPM,
		globalLimit: globalRPM,
		localOnly:   rdb == nil,
	}

	if rl.localOnly {
		rl.localCounts = make(map[string]*window)
	}

	return rl
}

// Allow checks if a request should be allowed through all three rate limit layers.
// Returns true if the request is within limits.
func (rl *RateLimiter) Allow(ctx context.Context, userID, model string) bool {
	if userID == "" {
		userID = "anonymous"
	}
	if model == "" {
		model = "unknown"
	}

	if rl.localOnly {
		return rl.allowLocal(userID, model)
	}

	keys := []struct {
		key   string
		limit int64
	}{
		{rateLimitPrefix + "user:" + userID, rl.userLimit},
		{rateLimitPrefix + "model:" + model, rl.modelLimit},
		{rateLimitPrefix + "global", rl.globalLimit},
	}

	for _, k := range keys {
		val, err := rl.rdb.Incr(ctx, k.key).Result()
		if err != nil {
			slog.Warn("rate limit incr failed", "key", k.key, "error", err)
			continue // allow on Redis error
		}
		if val == 1 {
			rl.rdb.Expire(ctx, k.key, time.Minute)
		}
		if val > k.limit {
			return false
		}
	}

	return true
}

// allowLocal provides in-memory rate limiting as a fallback when Redis is unavailable.
func (rl *RateLimiter) allowLocal(userID, model string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	keys := []struct {
		key   string
		limit int64
	}{
		{"user:" + userID, rl.userLimit},
		{"model:" + model, rl.modelLimit},
		{"global", rl.globalLimit},
	}

	for _, k := range keys {
		w, ok := rl.localCounts[k.key]
		if !ok || now.After(w.resetAt) {
			rl.localCounts[k.key] = &window{count: 1, resetAt: now.Add(time.Minute)}
			continue
		}
		w.count++
		if w.count > k.limit {
			return false
		}
	}

	return true
}

// Cleanup removes expired local windows to prevent memory leaks.
func (rl *RateLimiter) Cleanup(interval time.Duration) {
	if !rl.localOnly {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			rl.mu.Lock()
			now := time.Now()
			for key, w := range rl.localCounts {
				if now.After(w.resetAt) {
					delete(rl.localCounts, key)
				}
			}
			rl.mu.Unlock()
		}
	}()
}

// RemoteRateLimiter is an alternative Redis-backed limiter used via the
// existing api.RateLimiter pattern (token bucket, not sliding window).
// Use NewRateLimiter for the sliding-window approach above.

// defaultRateLimits returns sensible default rate limits.
func defaultRateLimits() (user, model, global int64) {
	return 100, 1000, 10000
}

// ── Token Bucket (in-memory, used by api middleware) ──────────────────────

// TokenBucket implements a simple in-memory token bucket per visitor.
type TokenBucket struct {
	mu       sync.Mutex
	visitors map[string]*bucket
	rate     int   // tokens per minute
	capacity int   // max burst
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

// NewTokenBucket creates a token bucket with the given rate (tokens/minute) and burst capacity.
func NewTokenBucket(rate, capacity int) *TokenBucket {
	return &TokenBucket{
		visitors: make(map[string]*bucket),
		rate:     rate,
		capacity: capacity,
	}
}

// Allow checks if a visitor has tokens available.
func (tb *TokenBucket) Allow(visitor string) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	b, ok := tb.visitors[visitor]
	if !ok {
		tb.visitors[visitor] = &bucket{
			tokens:    float64(tb.capacity - 1),
			lastCheck: time.Now(),
		}
		return true
	}

	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Minutes()
	b.tokens += elapsed * float64(tb.rate)
	if b.tokens > float64(tb.capacity) {
		b.tokens = float64(tb.capacity)
	}
	b.lastCheck = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// CleanupVisitors periodically removes stale entries.
func (tb *TokenBucket) CleanupVisitors(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			tb.mu.Lock()
			threshold := time.Now().Add(-10 * time.Minute)
			for ip, b := range tb.visitors {
				if b.lastCheck.Before(threshold) {
					delete(tb.visitors, ip)
				}
			}
			tb.mu.Unlock()
		}
	}()
}

// Helper: parseLimit extracts numeric limit from a rate limit key.
func parseLimit(key string) int64 {
	switch {
	case strings.Contains(key, "global"):
		return 10000
	case strings.Contains(key, "model:"):
		return 1000
	default:
		return 100
	}
}
