package handler

import (
	"net/http"
	"time"

	"minicc/internal/admin/models"
	"minicc/internal/admin/store"

	"github.com/gin-gonic/gin"
)

// DomainHandler handles HTTP requests for domain management.
type DomainHandler struct {
	store store.DomainStore
}

// NewDomainHandler creates a new DomainHandler.
func NewDomainHandler(store store.DomainStore) *DomainHandler {
	return &DomainHandler{store: store}
}

// Create handles POST /admin/domains
func (h *DomainHandler) Create(c *gin.Context) {
	var req models.CreateDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Create domain model
	domain := &models.Domain{
		Domain:      req.Domain,
		TenantID:    req.TenantID,
		DNSProvider: strPtr(req.DNSProvider),
		CNAMETarget: strPtr(req.CNAMETarget),
		AutoRenew:   req.AutoRenew,
		SSLStatus:   "pending",
		Status:      "verifying",
	}
	
	// Insert into database
	if err := h.store.Create(c.Request.Context(), domain); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create domain"})
		return
	}
	
	c.JSON(http.StatusCreated, domain)
}

// List handles GET /admin/domains
func (h *DomainHandler) List(c *gin.Context) {
	var filter models.ListDomainFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	domains, err := h.store.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list domains"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"data": domains,
	})
}

// GetByID handles GET /admin/domains/:domain_id
func (h *DomainHandler) GetByID(c *gin.Context) {
	domainID := c.Param("domain_id")
	
	domain, err := h.store.GetByID(c.Request.Context(), domainID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}
	
	c.JSON(http.StatusOK, domain)
}

// Update handles PUT /admin/domains/:domain_id
func (h *DomainHandler) Update(c *gin.Context) {
	domainID := c.Param("domain_id")
	
	var req models.UpdateDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	if err := h.store.Update(c.Request.Context(), domainID, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update domain"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "domain updated successfully"})
}

// VerifyDNS handles POST /admin/domains/:domain_id/verify
func (h *DomainHandler) VerifyDNS(c *gin.Context) {
	domainID := c.Param("domain_id")
	
	// TODO: Implement actual DNS verification
	// For now, just mark as verified
	if err := h.store.VerifyDNS(c.Request.Context(), domainID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify domain"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "domain verified successfully"})
}

// RenewSSL handles POST /admin/domains/:domain_id/renew-ssl
func (h *DomainHandler) RenewSSL(c *gin.Context) {
	domainID := c.Param("domain_id")
	
	// TODO: Implement actual SSL certificate renewal
	// For now, just update the expiration date (1 year from now)
	expiresAt := time.Now().Add(365 * 24 * time.Hour)
	if err := h.store.RenewSSL(c.Request.Context(), domainID, expiresAt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to renew SSL"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message":       "SSL certificate renewed",
		"expires_at":    expiresAt,
		"ssl_status":    "active",
	})
}

// Helper function to create string pointer
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
