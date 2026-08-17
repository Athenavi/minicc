package middleware

import (
	"net/http"
	"time"

	"minicc/internal/admin/store"

	"github.com/gin-gonic/gin"
)

// QuotaCheck checks if the API key has remaining quota.
func QuotaCheck(kStore store.APIKeyStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get API key ID from context
		apiKeyID, exists := c.Get("api_key_id")
		if !exists {
			// No API key in request, skip quota check
			c.Next()
			return
		}

		key, err := kStore.GetByID(c.Request.Context(), apiKeyID.(string))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			c.Abort()
			return
		}

		// Check monthly quota (0 means unlimited)
		if key.MonthlyQuota > 0 {
			if key.UsedCredits >= int64(key.MonthlyQuota) {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":       "monthly quota exceeded",
					"used_credits": key.UsedCredits,
					"quota":        key.MonthlyQuota,
				})
				c.Abort()
				return
			}
		}

		// Check rate limit (QPS)
		if key.RateLimitQPS > 0 {
			// TODO: Implement sliding window rate limiting
			// For now, just a placeholder
			lastRequestTime, _ := c.Get("last_request_time")
			if lastRequestTime != nil {
				lastTime := lastRequestTime.(time.Time)
				interval := time.Since(lastTime)
				minInterval := time.Second / time.Duration(key.RateLimitQPS)
				
				if interval < minInterval {
					c.JSON(http.StatusTooManyRequests, gin.H{
						"error":           "rate limit exceeded",
						"rate_limit_qps":   key.RateLimitQPS,
						"retry_after_ms":   (minInterval - interval).Milliseconds(),
					})
					c.Abort()
					return
				}
			}
		}

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
		
		// Calculate credits consumed (simplified)
		credits := int(duration / 1000) // 1 credit per second
		
		// Only track successful requests
		if statusCode >= 200 && statusCode < 300 {
			go func() {
				if err := kStore.IncrementUsage(c.Request.Context(), apiKeyID.(string), credits); err != nil {
					// Log error but don't fail the request
					// TODO: Use proper logger
				}
			}()
		}
	}
}
