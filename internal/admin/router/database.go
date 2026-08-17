package router

import (
	"github.com/gin-gonic/gin"
	
	"minicc/internal/admin/handler"
)

// RegisterDBRoutes registers all database related routes.
func RegisterDBRoutes(r *gin.Engine, dbHandler *handler.DBHandler) {
	admin := r.Group("/admin")
	
	database := admin.Group("/database")
	{
		database.GET("/status", dbHandler.GetStatus)
		
		backups := database.Group("/backups")
		{
			backups.POST("", dbHandler.CreateBackup)
			backups.GET("", dbHandler.ListBackups)
			backups.POST("/:id/restore", dbHandler.RestoreBackup)
		}
		
		database.POST("/query", dbHandler.ExecuteQuery)
		database.POST("/optimize/:action", dbHandler.Optimize)
	}
}
