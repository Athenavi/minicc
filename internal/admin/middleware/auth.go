package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AdminAuth verifies admin JWT token.
func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		// Expect "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		tokenString := parts[1]
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "empty token"})
			c.Abort()
			return
		}

		// TODO: Implement actual JWT verification
		// For now, accept any non-empty token for development
		// In production, use: claims, err := ParseToken(tokenString)
		
		// Extract user info from token and store in context
		/*
			claims, err := auth.ParseJWT(tokenString)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				c.Abort()
				return
			}

			if !claims.IsAdmin {
				c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
				c.Abort()
				return
			}

			c.Set("user_id", claims.UserID)
			c.Set("username", claims.Username)
			c.Set("role", claims.Role)
		*/

		c.Next()
	}
}

// APIKeyAuth verifies API key from header or query parameter.
func APIKeyAuth(keyStore interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get API key from header
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			// Try from query parameter
			apiKey = c.Query("api_key")
		}

		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing API key"})
			c.Abort()
			return
		}

		// TODO: Verify API key against database
		/*
			hash := util.HashKey(apiKey)
			key, err := keyStore.(store.APIKeyStore).GetByHash(c.Request.Context(), hash)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
				c.Abort()
				return
			}

			if key.Status != "active" {
				c.JSON(http.StatusForbidden, gin.H{"error": "API key is not active"})
				c.Abort()
				return
			}

			if key.ExpiresAt.Before(time.Now()) {
				c.JSON(http.StatusForbidden, gin.H{"error": "API key has expired"})
				c.Abort()
				return
			}

			// Store key info in context
			c.Set("api_key_id", key.ID)
			c.Set("tenant_id", key.TenantID)
		*/

		c.Next()
	}
}
