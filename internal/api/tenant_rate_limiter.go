package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/athenavi/minicc/internal/db"
)

// TenantRateLimiter provides per-tenant QPS limiting for specific resources.
type TenantRateLimiter struct {
	rdb       db.RedisClient
	tokens    map[string]*TokenBucket  // key: "resource:tenant" or "resource:user"
	maxBurst  int
	refillMs  int64
}

// TokenBucket implements token bucket algorithm in Redis.
type TokenBucket struct {
	key      string
	burst    int
	refillMs int64
	tokens   float64
	lastRefill time.Time
}

// NewTenantRateLimiter creates a rate limiter backed by Redis.
func NewTenantRateLimiter(rdb db.RedisClient, maxQPS, burst int) *TenantRateLimiter {
	return &TenantRateLimiter{
		rdb:      rdb,
		tokens:   make(map[string]*TokenBucket),
		maxBurst: burst,
		refillMs: 1000 / int64(maxQPS), // tokens per second
	}
}

// Allow checks if the request is allowed under the rate limit.
// Returns (allowed, retryAfterSeconds).
func (rl *TenantRateLimiter) Allow(resource, tenantID string) (bool, float64) {
	key := resource + ":" + tenantID
	
	bucket, exists := rl.tokens[key]
	if !exists {
		bucket = &TokenBucket{
			key:   key,
			burst: rl.maxBurst,
			tokens: float64(rl.maxBurst),
		}
		rl.tokens[key] = bucket
	}
	
	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill).Milliseconds()
	if elapsed > 0 {
		newTokens := float64(elapsed) / float64(bucket.refillMs)
		bucket.tokens = min(float64(bucket.burst), bucket.tokens+newTokens)
		bucket.lastRefill = now
	}
	
	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true, 0
	}
	
	// Calculate retry-after
	retryAfter := float64(bucket.refillMs-elapsed) / 1000.0
	return false, retryAfter
}

// ClearExpired removes stale buckets (cleanup every 5 minutes).
func (rl *TenantRateLimiter) ClearExpired() {
	// This would be called periodically by a background goroutine
	// For now, just log
	slog.Debug("TenantRateLimiter: cleanup not yet implemented")
}

// Middleware returns an HTTP middleware that enforces per-tenant rate limits.
func (rl *TenantRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := getAuthClaims(r, nil)
		if claims == nil {
			Unauthorized(w, "authentication required")
			return
		}
		
		tenantID := claims.TenantID
		if tenantID == "" {
			tenantID = claims.UserID
		}
		
		// Extract resource type from path (e.g., "/v1/kb/{id}/query" → "kb_query")
		resource := extractResource(r.URL.Path)
		
		allowed, retryAfter := rl.Allow(resource, tenantID)
		if !allowed {
			slog.Warn("rate limit exceeded", "resource", resource, "tenant", tenantID, "retry_after", retryAfter)
			w.Header().Set("Retry-After", formatFloat(retryAfter))
			RateLimited(w, "rate limit exceeded for this resource")
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// extractResource extracts resource name from URL path.
func extractResource(path string) string {
	parts := splitPath(path)
	
	// Pattern: /v1/{resource}/{action} or /v1/{resource}/{id}/{action}
	if len(parts) >= 2 {
		resource := parts[1]
		if len(parts) >= 3 {
			action := parts[2]
			// Common patterns: kb/query, kb/build, mcp/tools
			if action == "query" || action == "build" || action == "test" {
				return resource + "_" + action
			}
		}
		return resource
	}
	return "unknown"
}

func splitPath(path string) []string {
	if path == "/" {
		return []string{""}
	}
	
	result := []string{}
	start := 0
	for i, c := range path {
		if c == '/' && i > 0 {
			result = append(result, path[start:i])
			start = i + 1
		}
	}
	if start < len(path) {
		result = append(result, path[start:])
	}
	return result
}

func formatFloat(f float64) string {
	if f < 0.01 {
		return "1"
	}
	if f < 1 {
		return "1"
	}
	return strconv.Itoa(int(f))
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
