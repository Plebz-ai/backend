package api

import (
	"ai-agent-character-demo/backend/pkg/jwt"

	"github.com/gin-gonic/gin"
)

func RegisterCharacterRoutes(r *gin.Engine, handler *CharacterHandler, jwtService *jwt.Service) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	charGroup := r.Group("/characters")
	charGroup.Use(JWTAuthMiddleware(jwtService))
	{
		charGroup.POST("", JWTAuthMiddleware(jwtService, jwt.RoleAdmin), handler.CreateCharacter)
		charGroup.GET("/:id", handler.GetCharacterByID)
		charGroup.GET("", handler.GetAllCharacters)
	}
}
