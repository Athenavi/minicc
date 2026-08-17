package handler

import (
	"net/http"

	"minicc/internal/admin/models"
	"minicc/internal/admin/store"

	"github.com/gin-gonic/gin"
)

// RedisHandler handles HTTP requests for Redis management.
type RedisHandler struct {
	store store.RedisStore
}

// NewRedisHandler creates a new RedisHandler.
func NewRedisHandler(store store.RedisStore) *RedisHandler {
	return &RedisHandler{store: store}
}

// GetStatus handles GET /admin/redis/status
func (h *RedisHandler) GetStatus(c *gin.Context) {
	status, err := h.store.GetStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get redis status"})
		return
	}
	
	c.JSON(http.StatusOK, status)
}

// GetPool handles GET /admin/redis/pool
func (h *RedisHandler) GetPool(c *gin.Context) {
	poolStats, err := h.store.GetPoolStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get pool stats"})
		return
	}
	
	c.JSON(http.StatusOK, poolStats)
}

// FlushCache handles DELETE /admin/redis/cache
func (h *RedisHandler) FlushCache(c *gin.Context) {
	prefix := c.Query("prefix")
	
	if prefix == "" || prefix == "*" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prefix parameter required"})
		return
	}
	
	if err := h.store.FlushCache(c.Request.Context(), prefix); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to flush cache"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "cache flushed successfully", "prefix": prefix})
}

// FlushAll handles POST /admin/redis/flush-all
func (h *RedisHandler) FlushAll(c *gin.Context) {
	// Require confirmation
	if c.Query("confirm") != "yes" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "set confirm=yes to confirm this dangerous operation"})
		return
	}
	
	if err := h.store.FlushAll(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to flush all cache"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "all cache flushed successfully"})
}

// GetSlowLog handles GET /admin/redis/slow-log
func (h *RedisHandler) GetSlowLog(c *gin.Context) {
	limit := 50
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	
	slowLog, err := h.store.GetSlowLog(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get slow log"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"data": slowLog})
}

// UpdateConfig handles PUT /admin/redis/config
func (h *RedisHandler) UpdateConfig(c *gin.Context) {
	var config models.RedisConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	if err := h.store.UpdateConfig(c.Request.Context(), config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update config"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "config updated successfully"})
}
