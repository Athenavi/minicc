package router

import (
	"github.com/gin-gonic/gin"
	
	"minicc/internal/admin/handler"
)

// RegisterRedisRoutes registers all Redis related routes.
func RegisterRedisRoutes(r *gin.Engine, redisHandler *handler.RedisHandler) {
	admin := r.Group("/admin")
	
	redis := admin.Group("/redis")
	{
		redis.GET("/status", redisHandler.GetStatus)
		redis.GET("/pool", redisHandler.GetPool)
		redis.DELETE("/cache", redisHandler.FlushCache)
		redis.POST("/flush-all", redisHandler.FlushAll)
		redis.GET("/slow-log", redisHandler.GetSlowLog)
		redis.PUT("/config", redisHandler.UpdateConfig)
	}
}
