package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"minicc/internal/admin/handler"
	"minicc/internal/admin/router"
	"minicc/internal/admin/store"
)

// SetupAll initializes all admin handlers, stores, and routes.
// Returns a Gin engine instance with all admin routes registered.
func SetupAll(db *http.Client) (*gin.Engine, error) {
	// Create Gin router in release mode
	gin.SetMode(gin.ReleaseMode)
	ginEngine := gin.New()
	
	// Add recovery middleware
	ginEngine.Use(gin.Recovery())
	
	// TODO: Initialize stores with actual database connection
	// For now, this is a placeholder - see integration guide in BACKEND_MANAGEMENT_SYSTEM_FINAL_REPORT.md
	
	/*
	// Example integration:
	
	pgDB := db.Pool // Your existing PostgreSQL connection
	
	// Initialize stores
	tenantStore := store.NewPostgreSQLTenantStore(pgDB)
	apiKeyStore := store.NewPostgreSQLAPIKeyStore(pgDB)
	domainStore := store.NewPostgreSQLDomainStore(pgDB)
	redisStore := store.NewPostgreSQLRedisStore(pgDB)
	dbStore := store.NewPostgreSQLDBStore(pgDB)
	
	// Initialize handlers
	tenantHandler := handler.NewTenantHandler(tenantStore)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeyStore)
	domainHandler := handler.NewDomainHandler(domainStore)
	redisHandler := handler.NewRedisHandler(redisStore)
	dbHandler := handler.NewDBHandler(dbStore)
	
	// Register routes
	router.RegisterTenantRoutes(ginEngine, tenantHandler)
	router.RegisterAPIKeyRoutes(ginEngine, apiKeyHandler)
	router.RegisterDomainRoutes(ginEngine, domainHandler)
	router.RegisterRedisRoutes(ginEngine, redisHandler)
	router.RegisterDBRoutes(ginEngine, dbHandler)
	*/
	
	return ginEngine, nil
}

// RegisterAdminRoutes registers admin routes into an existing Gin router.
// This is the recommended integration method.
func RegisterAdminRoutes(r *gin.RouterGroup, 
	tenantHandler *handler.TenantHandler,
	apiKeyHandler *handler.APIKeyHandler,
	domainHandler *handler.DomainHandler,
	redisHandler *handler.RedisHandler,
	dbHandler *handler.DBHandler,
) {
	// All admin routes are prefixed with /admin
	admin := r.Group("/admin")
	
	// Tenant management
	router.RegisterTenantRoutes(admin, tenantHandler)
	
	// API Key management
	router.RegisterAPIKeyRoutes(admin, apiKeyHandler)
	
	// Domain management
	router.RegisterDomainRoutes(admin, domainHandler)
	
	// Redis management
	router.RegisterRedisRoutes(admin, redisHandler)
	
	// Database management
	router.RegisterDBRoutes(admin, dbHandler)
}
