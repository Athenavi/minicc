package router

import (
	"github.com/gin-gonic/gin"
	
	"minicc/internal/admin/handler"
)

// RegisterAPIKeyRoutes registers all API key related routes.
func RegisterAPIKeyRoutes(r *gin.Engine, keyHandler *handler.APIKeyHandler) {
	// Admin group (requires admin authentication)
	admin := r.Group("/admin")
	
	apiKeys := admin.Group("/api-keys")
	{
		// Create a new API key
		apiKeys.POST("", keyHandler.Create)
		
		// List all API keys with filters
		apiKeys.GET("", keyHandler.List)
		
		// Get specific API key by ID
		apiKeys.GET("/:id", keyHandler.GetByID)
		
		// Update API key
		apiKeys.PUT("/:id", keyHandler.Update)
		
		// Delete API key
		apiKeys.DELETE("/:id", keyHandler.Delete)
		
		// Bulk cleanup expired keys
		apiKeys.POST("/bulk-cleanup", keyHandler.BulkCleanup)
	}
}
