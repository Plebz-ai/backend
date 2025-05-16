package api

import (
	"ai-agent-character-demo/backend/pkg/jwt"

	"github.com/gin-gonic/gin"
)

func RegisterMessageRoutes(r *gin.Engine, handler *MessageHandler, jwtService *jwt.Service) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	msgGroup := r.Group("/messages")
	msgGroup.Use(JWTAuthMiddleware(jwtService))
	{
		msgGroup.POST("", JWTAuthMiddleware(jwtService, jwt.RoleUser), handler.CreateMessage)
		msgGroup.GET("/:id", handler.GetMessageByID)
		msgGroup.GET("/session/:session_id", handler.GetMessagesBySession)
	}
}
