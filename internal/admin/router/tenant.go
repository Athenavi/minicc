package router

import (
	"github.com/gin-gonic/gin"
	
	"minicc/internal/admin/handler"
)

// RegisterTenantRoutes registers all tenant related routes.
func RegisterTenantRoutes(r *gin.Engine, tenantHandler *handler.TenantHandler) {
	// Admin group (requires admin authentication)
	admin := r.Group("/admin")
	
	tenants := admin.Group("/tenants")
	{
		// Create a new tenant
		tenants.POST("", tenantHandler.Create)
		
		// List all tenants with filters
		tenants.GET("", tenantHandler.List)
		
		// Get specific tenant by ID
		tenants.GET("/:tenant_id", tenantHandler.GetByID)
		
		// Update tenant
		tenants.PUT("/:tenant_id", tenantHandler.Update)
		
		// Suspend tenant
		tenants.POST("/:tenant_id/suspend", tenantHandler.Suspend)
		
		// Get tenant usage statistics
		tenants.GET("/:tenant_id/usage", tenantHandler.GetUsage)
	}
}
