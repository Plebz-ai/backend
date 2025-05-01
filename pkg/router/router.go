package router

import (
	"ai-agent-character-demo/backend/internal/api"
	"ai-agent-character-demo/backend/internal/ws"
	"ai-agent-character-demo/backend/pkg/di"
	"ai-agent-character-demo/backend/pkg/logger"

	"github.com/gin-gonic/gin"
)

// Router is the main router for the application
type Router struct {
	Engine    *gin.Engine
	Container *di.Container
	Logger    *logger.Logger
	Hub       *ws.Hub
}

// New creates a new router with the given container
func New(container *di.Container) *Router {
	// Use the container's logger
	logger.SetGlobal(container.Logger)

	// Initialize Gin router
	engine := gin.New()

	// Use the logger middleware
	engine.Use(logger.Middleware(container.Logger))

	// Add recovery middleware
	engine.Use(gin.Recovery())

	// Initialize WebSocket hub
	hub := ws.NewHub(
		container.CharacterServiceAdapter,
		container.AIServiceAdapter,
		container.MessageServiceAdapter,
	)

	// Set audio service in the hub for automatic audio storage
	hub.SetAudioService(container.AudioService)

	// Start the hub
	go hub.Run()

	return &Router{
		Engine:    engine,
		Container: container,
		Logger:    container.Logger,
		Hub:       hub,
	}
}

// SetupRoutes registers all application routes
func (r *Router) SetupRoutes() {
	// Add CORS middleware
	r.Engine.Use(corsMiddleware())

	// Initialize controllers
	authHandler := api.NewAuthHandler(r.Container.UserService)
	characterHandler := api.NewCharacterHandler(r.Container.CharacterService)
	audioController := api.NewAudioController(r.Container.AudioService, r.Container.JWTService)
	messageController := api.NewMessageController(
		r.Container.MessageService,
		r.Container.CharacterService,
		r.Container.AIServiceAdapter,
		r.Container.JWTService,
	)

	// Auth routes
	auth := r.Engine.Group("/api/auth")
	{
		auth.POST("/signup", authHandler.Signup)
		auth.POST("/login", authHandler.Login)
		auth.GET("/me", api.AuthMiddleware(), authHandler.Me)
	}

	// Character routes - protected by auth middleware
	characters := r.Engine.Group("/api/characters")
	characters.Use(api.AuthMiddleware())
	{
		characters.POST("", characterHandler.CreateCharacter)
		characters.GET("", characterHandler.ListCharacters)
		characters.GET("/:id", characterHandler.GetCharacter)
	}

	// Register audio routes
	audioController.RegisterRoutes(r.Engine)

	// Register message routes
	messageController.RegisterRoutes(r.Engine)

	// WebSocket route
	r.Engine.GET("/ws", func(c *gin.Context) {
		ws.ServeWs(r.Hub, c)
	})

	// Health check endpoints
	r.setupHealthRoutes()
}

// corsMiddleware creates a middleware function for CORS handling
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		if origin != "*" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}

		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept, Accept-Encoding, X-CSRF-Token, Authorization, Origin")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
