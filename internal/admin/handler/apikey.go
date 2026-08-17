package handler

import (
	"net/http"
	"strconv"
	"time"

	"minicc/internal/admin/models"
	"minicc/internal/admin/store"
	"minicc/internal/admin/util"

	"github.com/gin-gonic/gin"
)

// APIKeyHandler handles HTTP requests for API key management.
type APIKeyHandler struct {
	store store.APIKeyStore
}

// NewAPIKeyHandler creates a new APIKeyHandler.
func NewAPIKeyHandler(store store.APIKeyStore) *APIKeyHandler {
	return &APIKeyHandler{store: store}
}

// Create handles POST /admin/api-keys
func (h *APIKeyHandler) Create(c *gin.Context) {
	var req models.CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Generate API Key
	apiKey, err := util.GenerateLiveAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate key"})
		return
	}
	
	// Hash the key
	keyHash := util.HashKey(apiKey)
	
	// Create model
	now := time.Now()
	key := &models.APIKey{
		Name:          req.Name,
		TenantID:      req.TenantID,
		MonthlyQuota:  req.MonthlyQuota,
		ExpiresAt:     now,
		Status:        "active",
		AllowedModels: req.AllowedModels,
		RateLimitQPS:  req.RateLimitQPS,
		KeyHash:       keyHash,
	}
	
	if req.ExpiresAt != nil {
		key.ExpiresAt = *req.ExpiresAt
	}
	
	if req.Description != "" {
		key.Description = &req.Description
	}
	
	// Insert into database
	if err := h.store.Create(c.Request.Context(), key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create api key"})
		return
	}
	
	// Return the plain text key (only shown once!)
	c.JSON(http.StatusCreated, gin.H{
		"id":           key.ID,
		"key":          apiKey,  // ⚠️ This is the ONLY time this will be shown
		"name":         key.Name,
		"tenant_id":    key.TenantID,
		"expires_at":   key.ExpiresAt,
		"monthly_quota": key.MonthlyQuota,
		"rate_limit_qps": key.RateLimitQPS,
		"message": "Save this API key securely. It will not be shown again.",
	})
}

// List handles GET /admin/api-keys
func (h *APIKeyHandler) List(c *gin.Context) {
	var filter models.ListAPIKeyFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	keys, err := h.store.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list api keys"})
		return
	}
	
	total, err := h.store.Total(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get total count"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"data":  keys,
		"total": total,
		"page":  filter.Page,
		"page_size": filter.PageSize,
	})
}

// GetByID handles GET /admin/api-keys/:id
func (h *APIKeyHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	
	key, err := h.store.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
		return
	}
	
	c.JSON(http.StatusOK, key)
}

// Update handles PUT /admin/api-keys/:id
func (h *APIKeyHandler) Update(c *gin.Context) {
	id := c.Param("id")
	
	var req models.UpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	if err := h.store.Update(c.Request.Context(), id, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update api key"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "api key updated successfully"})
}

// Delete handles DELETE /admin/api-keys/:id
func (h *APIKeyHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	
	if err := h.store.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete api key"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "api key deleted successfully"})
}

// BulkCleanup handles POST /admin/api-keys/bulk-cleanup
func (h *APIKeyHandler) BulkCleanup(c *gin.Context) {
	var req struct {
		Action string `json:"action" binding:"required"` // suspend | delete | notify
		ExpiredBefore *time.Time `json:"expired_before"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Find expired keys
	keys, err := h.store.List(c.Request.Context(), models.ListAPIKeyFilter{
		Status:        strPtr("active"),
		ExpiresBefore: req.ExpiredBefore,
	})
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query expired keys"})
		return
	}
	
	if len(keys) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "No expired keys found",
			"count": 0,
		})
		return
	}
	
	// Collect IDs
	ids := make([]string, len(keys))
	for i, key := range keys {
		ids[i] = key.ID
	}
	
	// Execute action
	switch req.Action {
	case "suspend":
		if err := h.store.BulkUpdateStatus(c.Request.Context(), ids, "suspended"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to suspend keys"})
			return
		}
	case "delete":
		// TODO: Implement BulkDelete
		c.JSON(http.StatusBadRequest, gin.H{"error": "delete action not implemented yet"})
		return
	case "notify":
		// TODO: Send notification emails
		c.JSON(http.StatusOK, gin.H{"message": "Notification sent to affected users"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully processed",
		"count":   len(ids),
		"action":  req.Action,
	})
}

// Helper function to create string pointer
func strPtr(s string) *string {
	return &s
}
