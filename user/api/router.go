package api

import (
	"github.com/gin-gonic/gin"
	"ai-agent-character-demo/backend/pkg/jwt"
)

func RegisterUserRoutes(r *gin.Engine, handler *UserHandler, jwtService *jwt.Service) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	userGroup := r.Group("/users")
	userGroup.Use(JWTAuthMiddleware(jwtService))
	{
		userGroup.POST("", JWTAuthMiddleware(jwtService, jwt.RoleAdmin), handler.CreateUser)
		userGroup.GET("/email/:email", handler.GetUserByEmail)
		userGroup.GET("/:id", handler.GetUserByID)
	}
}
