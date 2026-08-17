package handler

import (
	"net/http"
	"strings"

	"minicc/internal/admin/models"
	"minicc/internal/admin/store"

	"github.com/gin-gonic/gin"
)

// DBHandler handles HTTP requests for database management.
type DBHandler struct {
	store store.DBStore
}

// NewDBHandler creates a new DBHandler.
func NewDBHandler(store store.DBStore) *DBHandler {
	return &DBHandler{store: store}
}

// GetStatus handles GET /admin/database/status
func (h *DBHandler) GetStatus(c *gin.Context) {
	status, err := h.store.GetStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get database status"})
		return
	}
	
	c.JSON(http.StatusOK, status)
}

// CreateBackup handles POST /admin/database/backups
func (h *DBHandler) CreateBackup(c *gin.Context) {
	var req struct {
		Description string `json:"description"`
	}
	c.ShouldBindJSON(&req)
	
	backup, err := h.store.CreateBackup(c.Request.Context(), req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create backup"})
		return
	}
	
	c.JSON(http.StatusCreated, backup)
}

// ListBackups handles GET /admin/database/backups
func (h *DBHandler) ListBackups(c *gin.Context) {
	backups, err := h.store.ListBackups(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list backups"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"data": backups})
}

// RestoreBackup handles POST /admin/database/backups/:id/restore
func (h *DBHandler) RestoreBackup(c *gin.Context) {
	backupID := c.Param("id")
	
	// Require confirmation
	if c.Query("confirm") != "yes" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "set confirm=yes to confirm restore"})
		return
	}
	
	if err := h.store.RestoreBackup(c.Request.Context(), backupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restore backup"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "backup restored successfully"})
}

// ExecuteQuery handles POST /admin/database/query
func (h *DBHandler) ExecuteQuery(c *gin.Context) {
	var req struct {
		SQL string `json:"sql" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Only allow SELECT queries
	sqlUpper := strings.ToUpper(strings.TrimSpace(req.SQL))
	if !strings.HasPrefix(sqlUpper, "SELECT") && !strings.HasPrefix(sqlUpper, "SHOW") && !strings.HasPrefix(sqlUpper, "EXPLAIN") {
		c.JSON(http.StatusForbidden, gin.H{"error": "only read-only queries allowed (SELECT/SHOW/EXPLAIN)"})
		return
	}
	
	results, err := h.store.ExecuteQuery(c.Request.Context(), req.SQL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"data": results})
}

// Optimize handles POST /admin/database/optimize
func (h *DBHandler) Optimize(c *gin.Context) {
	action := c.Param("action") // vacuum / analyze / reindex
	
	switch action {
	case "vacuum", "analyze", "reindex":
		if err := h.store.Optimize(c.Request.Context(), action); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "optimization failed"})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action (vacuum/analyze/reindex)"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("%s completed successfully", action),
		"action":  action,
	})
}
