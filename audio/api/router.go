package api

import (
	"ai-agent-character-demo/backend/pkg/jwt"

	"github.com/gin-gonic/gin"
)

func RegisterAudioRoutes(r *gin.Engine, handler *AudioHandler, jwtService *jwt.Service) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	audioGroup := r.Group("/audio")
	audioGroup.Use(JWTAuthMiddleware(jwtService))
	{
		audioGroup.POST("", JWTAuthMiddleware(jwtService, jwt.RoleUser), handler.CreateAudioChunk)
		audioGroup.GET("/:id", handler.GetAudioChunkByID)
		audioGroup.GET("/session/:session_id", handler.GetAudioChunksBySession)
	}
}
