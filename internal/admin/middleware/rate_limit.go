package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements a sliding window rate limiter using in-memory counting.
type RateLimiter struct {
	defaultRPM int // default requests per minute
}

// RateLimitConfig holds configuration for rate limiting.
type RateLimitConfig struct {
	RPM     int    // requests per minute (0 = unlimited)
	Burst   int    // maximum burst size (0 = same as RPM)
	KeyFunc func(c *gin.Context) string // custom key generator
}

// NewRateLimiter creates a new rate limiter instance.
func NewRateLimiter(defaultRPM int) *RateLimiter {
	return &RateLimiter{
		defaultRPM: defaultRPM,
	}
}

// DefaultRateLimiter returns the global rate limiter instance.
var DefaultRateLimiter *RateLimiter

// InitRateLimiter initializes the global rate limiter.
func InitRateLimiter(defaultRPM int) {
	DefaultRateLimiter = NewRateLimiter(defaultRPM)
}

// Middleware returns a Gin middleware for rate limiting.
func (rl *RateLimiter) Middleware(cfg *RateLimitConfig) gin.HandlerFunc {
	rpm := cfg.RPM
	if rpm <= 0 {
		rpm = rl.defaultRPM
	}
	burst := cfg.Burst
	if burst <= 0 {
		burst = rpm
	}
	keyFunc := cfg.KeyFunc
	if keyFunc == nil {
		keyFunc = defaultKeyGenerator
	}

	return func(c *gin.Context) {
		if rpm <= 0 {
			// Rate limiting disabled
			c.Next()
			return
		}

		key := keyFunc(c)
		windowKey := key + ":" + time.Now().Format("2006-01-02T15:04")

		inMemoryLimiter.mu.Lock()
		defer inMemoryLimiter.mu.Unlock()

		now := time.Now()

		// Clean old entries (> 2 minutes)
		for k := range inMemoryLimiter.requests {
			if now.Sub(inMemoryLimiter.timestamps[k]) > 2*time.Minute {
				delete(inMemoryLimiter.requests, k)
				delete(inMemoryLimiter.timestamps, k)
			}
		}

		// Check if over limit
		if cnt, exists := inMemoryLimiter.requests[windowKey]; exists && cnt >= int64(rpm) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": 60,
			})
			c.Abort()
			return
		}

		// Record request
		inMemoryLimiter.requests[windowKey]++
		inMemoryLimiter.timestamps[windowKey] = now
		c.Next()
	}
}

// In-memory rate limiter
type memoryRateLimiter struct {
	requests   map[string]int64
	timestamps map[string]time.Time
	mu         sync.Mutex
}

var inMemoryLimiter = &memoryRateLimiter{
	requests:   make(map[string]int64),
	timestamps: make(map[string]time.Time),
}

// defaultKeyGenerator generates a unique key per user/IP.
func defaultKeyGenerator(c *gin.Context) string {
	// Try user ID first (from JWT claims)
	if userID, exists := c.Get("user_id"); exists {
		return fmt.Sprintf("user:%s", userID)
	}

	// Fall back to IP address
	ip := c.ClientIP()
	return fmt.Sprintf("ip:%s", ip)
}
