package handler

import (
	"net/http"
	"time"

	"minicc/internal/admin/models"
	"minicc/internal/admin/store"

	"github.com/gin-gonic/gin"
)

// TenantHandler handles HTTP requests for tenant management.
type TenantHandler struct {
	store store.TenantStore
}

// NewTenantHandler creates a new TenantHandler.
func NewTenantHandler(store store.TenantStore) *TenantHandler {
	return &TenantHandler{store: store}
}

// Create handles POST /admin/tenants
func (h *TenantHandler) Create(c *gin.Context) {
	var req models.CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Set defaults
	if req.MaxAPIKeys == 0 {
		req.MaxAPIKeys = 10
	}
	if req.MaxModels == 0 {
		req.MaxModels = 5
	}
	if req.MaxConcurrentSessions == 0 {
		req.MaxConcurrentSessions = 10
	}
	
	// Create tenant model
	tenant := &models.Tenant{
		TenantID:              req.TenantID,
		Name:                  req.Name,
		CompanyName:           req.CompanyName,
		ContactEmail:          strPtr(req.ContactEmail),
		ContactPhone:          strPtr(req.ContactPhone),
		MaxAPIKeys:            req.MaxAPIKeys,
		MaxModels:             req.MaxModels,
		MonthlyQuota:          req.MonthlyQuota,
		MaxConcurrentSessions: req.MaxConcurrentSessions,
		ExpiresAt:             req.ExpiresAt,
		Features:              req.Features,
		Status:                "active",
	}
	
	// Insert into database
	if err := h.store.Create(c.Request.Context(), tenant); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tenant"})
		return
	}
	
	c.JSON(http.StatusCreated, tenant)
}

// List handles GET /admin/tenants
func (h *TenantHandler) List(c *gin.Context) {
	var filter models.ListTenantFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	tenants, err := h.store.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tenants"})
		return
	}
	
	total, err := h.store.Total(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get total count"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"data":      tenants,
		"total":     total,
		"page":      filter.Page,
		"page_size": filter.PageSize,
	})
}

// GetByID handles GET /admin/tenants/:tenant_id
func (h *TenantHandler) GetByID(c *gin.Context) {
	tenantID := c.Param("tenant_id")
	
	tenant, err := h.store.GetByTenantID(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}
	
	c.JSON(http.StatusOK, tenant)
}

// Update handles PUT /admin/tenants/:tenant_id
func (h *TenantHandler) Update(c *gin.Context) {
	tenantID := c.Param("tenant_id")
	
	var req models.UpdateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	if err := h.store.Update(c.Request.Context(), tenantID, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tenant"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "tenant updated successfully"})
}

// Suspend handles POST /admin/tenants/:tenant_id/suspend
func (h *TenantHandler) Suspend(c *gin.Context) {
	tenantID := c.Param("tenant_id")
	
	if err := h.store.Suspend(c.Request.Context(), tenantID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to suspend tenant"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "tenant suspended successfully"})
}

// GetUsage handles GET /admin/tenants/:tenant_id/usage
func (h *TenantHandler) GetUsage(c *gin.Context) {
	tenantID := c.Param("tenant_id")
	
	// Parse date range (default to last 30 days)
	startDate, _ := time.Parse("2006-01-02", c.Query("start_date"))
	endDate, _ := time.Parse("2006-01-02", c.Query("end_date"))
	
	if endDate.IsZero() {
		endDate = time.Now()
	}
	if startDate.IsZero() {
		startDate = endDate.Add(-30 * 24 * time.Hour)
	}
	
	usage, err := h.store.GetUsage(c.Request.Context(), tenantID, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get usage"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"data":      usage,
		"tenant_id": tenantID,
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
	})
}

// Helper function to create string pointer
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
