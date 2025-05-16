package router

import (
	"time"

	"ai-agent-character-demo/backend/internal/api"
	"ai-agent-character-demo/backend/internal/ws"
	"ai-agent-character-demo/backend/pkg/config"
	"ai-agent-character-demo/backend/pkg/di"
	"ai-agent-character-demo/backend/pkg/errors"
	"ai-agent-character-demo/backend/pkg/jwt"
	"ai-agent-character-demo/backend/pkg/logger"
	"ai-agent-character-demo/backend/pkg/middleware"

	"github.com/gin-gonic/gin"
)

// Track server start time for uptime calculations
var startTime = time.Now()

// Router is the main router for the application
type Router struct {
	Engine    *gin.Engine
	Container *di.Container
	Logger    *logger.Logger
	Hub       *ws.Hub
	Config    *config.Config
}

// New creates a new router with the given container
func New(container *di.Container) *Router {
	// Use the container's logger
	logger.SetGlobal(container.Logger)

	// Load configuration
	cfg := config.New()

	// Configure Gin mode based on environment
	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize Gin router
	engine := gin.New()

	// Use the logger middleware first to capture all requests
	engine.Use(logger.Middleware(container.Logger))

	// Add custom error handler middleware
	engine.Use(errors.ErrorHandler())

	// Add custom recovery middleware with structured logging instead of default
	engine.Use(errors.RecoveryWithLogger())

	// Create rate limiter with default options
	rateLimiter := middleware.NewRateLimiter(container.Logger)

	// Apply rate limiting to all routes
	engine.Use(rateLimiter.Middleware())

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
		Config:    cfg,
	}
}

// SetupRoutes registers all application routes
func (r *Router) SetupRoutes() {
	// Add CORS middleware
	r.Engine.Use(corsMiddleware())

	// Create JWT auth middleware
	jwtAuth := middleware.JWTAuthMiddleware(r.Container.JWTService, r.Logger)

	// Initialize controllers with proper constructor signatures
	authHandler := api.NewAuthHandler(r.Container.UserService, r.Container.JWTService, r.Logger)
	characterHandler := api.NewCharacterHandler(r.Container.CharacterService)
	audioController := api.NewAudioController(r.Container.AudioService, r.Container.JWTService)
	messageController := api.NewMessageController(
		r.Container.MessageService,
		r.Container.CharacterService,
		r.Container.AIServiceAdapter,
		r.Container.JWTService,
	)

	// API version 1 routes
	v1 := r.Engine.Group("/api/v1")

	// Public routes (no auth required)
	publicRoutes := v1.Group("/")
	{
		// Health check endpoint
		publicRoutes.GET("/health", r.healthCheckHandler())

		// Auth routes
		authRoutes := publicRoutes.Group("/auth")
		{
			authRoutes.POST("/signup", authHandler.Signup)
			authRoutes.POST("/login", authHandler.Login)
			authRoutes.GET("/me", jwtAuth, authHandler.Me)
		}
	}

	// Protected routes (require authentication)
	protectedRoutes := v1.Group("/")
	protectedRoutes.Use(jwtAuth)
	{
		// User management routes (admin only)
		adminRoutes := protectedRoutes.Group("/admin")
		adminRoutes.Use(middleware.RequireRole(jwt.RoleAdmin))
		{
			adminRoutes.PUT("/users/:id/role", authHandler.UpdateUserRole)
		}

		// Character routes - protected by auth
		characterRoutes := protectedRoutes.Group("/characters")
		{
			characterRoutes.POST("", middleware.RequirePermission(jwt.PermWriteCharacter), characterHandler.CreateCharacter)
			characterRoutes.GET("", middleware.RequirePermission(jwt.PermReadCharacter), characterHandler.ListCharacters)
			characterRoutes.GET("/:id", middleware.RequirePermission(jwt.PermReadCharacter), characterHandler.GetCharacter)

			// Comment out these routes until the methods are implemented
			// characterRoutes.PUT("/:id", middleware.RequirePermission(jwt.PermWriteCharacter), characterHandler.UpdateCharacter)
			// characterRoutes.DELETE("/:id", middleware.RequirePermission(jwt.PermDeleteCharacter), characterHandler.DeleteCharacter)
		}
	}

	// Register audio routes with versioning
	audioController.RegisterRoutesV1(v1)

	// Register message routes with versioning
	messageController.RegisterRoutesV1(v1)

	// Legacy API routes for backward compatibility
	// These will eventually be phased out
	legacyAuth := r.Engine.Group("/api/auth")
	{
		legacyAuth.POST("/signup", authHandler.Signup)
		legacyAuth.POST("/login", authHandler.Login)
		legacyAuth.GET("/me", jwtAuth, authHandler.Me)
	}

	// Register legacy routes for backward compatibility
	audioController.RegisterRoutes(r.Engine)
	messageController.RegisterRoutes(r.Engine)

	// WebSocket route
	r.Engine.GET("/ws", func(c *gin.Context) {
		ws.ServeWs(r.Hub, c)
	})
}

// healthCheckHandler returns a simple health check handler
func (r *Router) healthCheckHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"version": r.Config.Server.Env,
			"time":    time.Now().Format(time.RFC3339),
		})
	}
}

// Enhance CORS middleware to explicitly allow WebSocket-specific headers
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
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept, Accept-Encoding, X-CSRF-Token, Authorization, Origin, Upgrade, Connection, Cache-Control")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Upgrade, Connection")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
