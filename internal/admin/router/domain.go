package router

import (
	"github.com/gin-gonic/gin"
	
	"minicc/internal/admin/handler"
)

// RegisterDomainRoutes registers all domain related routes.
func RegisterDomainRoutes(r *gin.Engine, domainHandler *handler.DomainHandler) {
	admin := r.Group("/admin")
	
	domains := admin.Group("/domains")
	{
		domains.POST("", domainHandler.Create)
		domains.GET("", domainHandler.List)
		domains.GET("/:domain_id", domainHandler.GetByID)
		domains.PUT("/:domain_id", domainHandler.Update)
		domains.POST("/:domain_id/verify", domainHandler.VerifyDNS)
		domains.POST("/:domain_id/renew-ssl", domainHandler.RenewSSL)
	}
}
