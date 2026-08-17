package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// QuotaCheck checks if the API key has remaining quota.
func QuotaCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from context
		userID, exists := c.Get("user_id")
		if !exists {
			// No user in request, skip quota check
			c.Next()
			return
		}

		// Check monthly quota - TODO: Implement when billing system is ready
		// For now, skip quota check

		// Skip rate limit check - handled by rate_limit middleware
		// TODO: Remove legacy rate limiting code once migrated

		c.Next()
	}
}

// UsageTracking tracks API usage for billing and monitoring.
func UsageTracking(kStore store.APIKeyStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		
		// Process request
		c.Next()
		
		// After response, track usage
		apiKeyID, exists := c.Get("api_key_id")
		if !exists {
			return
		}

		duration := time.Since(start).Milliseconds()
		statusCode := c.Writer.Status()
		
		// Usage tracking - TODO: Implement when billing system is ready
		// For now, skip usage tracking
	}
}
