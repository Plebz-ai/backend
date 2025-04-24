package main

import (
	"context"
	"log"
	"os"
	"time"

	"ai-agent-character-demo/backend/internal/api"
	"ai-agent-character-demo/backend/internal/models"
	"ai-agent-character-demo/backend/internal/service"
	"ai-agent-character-demo/backend/internal/ws"
	"ai-agent-character-demo/backend/pkg/config"
	"ai-agent-character-demo/backend/pkg/jwt"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize database
	db, err := config.NewDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Auto-migrate the schema
	if err := db.AutoMigrate(&models.Character{}, &models.Message{}, &models.User{}, &models.AudioChunk{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Initialize Gin router
	r := gin.Default()

	// CORS middleware
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept, Accept-Encoding, X-CSRF-Token, Authorization, Origin")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Register services
	messageService := service.NewMessageService(db)
	characterService := service.NewCharacterService(db)
	userService := service.NewUserService(db)
	audioService := service.NewAudioServiceWithConfig(db, service.AudioServiceConfig{
		MaxChunksPerSession: 5000, // Allow up to 5000 audio chunks per session
		DefaultTTL:          24 * time.Hour,
	})

	// Create JWT service
	jwtService := jwt.NewService("", 0) // Use defaults

	// Initialize service adapters for WebSocket
	characterServiceAdapter := service.NewCharacterServiceAdapter(characterService)
	messageServiceAdapter := service.NewMessageServiceAdapter(messageService)

	// Create AI Service adapter
	aiServiceAdapter := service.NewAIServiceAdapter(
		func(character *ws.Character, userMessage string, history []ws.ChatMessage) (string, error) {
			// Simple mock implementation
			return "Hello! I'm " + character.Name + ". Thanks for your message: \"" + userMessage + "\"", nil
		},
		func(ctx context.Context, text string, voiceType string) ([]byte, error) {
			// Mock implementation - in a real app, this would call a TTS service
			log.Printf("Text-to-speech request for text: %s with voice type: %s", text, voiceType)
			return nil, nil
		},
		func(ctx context.Context, audioData []byte) (string, error) {
			// Mock implementation - in a real app, this would call an STT service
			log.Printf("Speech-to-text request received with %d bytes of audio data", len(audioData))
			return "This is a mock transcription", nil
		},
	)

	// Initialize WebSocket hub
	hub := ws.NewHub(characterServiceAdapter, aiServiceAdapter, messageServiceAdapter)

	// Set audio service in the hub for automatic audio storage
	hub.SetAudioService(audioService)

	go hub.Run()

	// Register controllers and routes
	authHandler := api.NewAuthHandler(userService)
	characterHandler := api.NewCharacterHandler(characterService)
	audioController := api.NewAudioController(audioService, jwtService)
	messageController := api.NewMessageController(messageService, characterService, aiServiceAdapter, jwtService)

	// Auth routes
	auth := r.Group("/api/auth")
	{
		auth.POST("/signup", authHandler.Signup)
		auth.POST("/login", authHandler.Login)
		auth.GET("/me", api.AuthMiddleware(), authHandler.Me)
	}

	// Character routes - protected by auth middleware
	characters := r.Group("/api/characters")
	characters.Use(api.AuthMiddleware())
	{
		characters.POST("", characterHandler.CreateCharacter)
		characters.GET("", characterHandler.ListCharacters)
		characters.GET("/:id", characterHandler.GetCharacter)
	}

	// Register audio routes
	audioController.RegisterRoutes(r)

	// Register message routes
	messageController.RegisterRoutes(r)

	// WebSocket route
	r.GET("/ws", func(c *gin.Context) {
		ws.ServeWs(hub, c)
	})

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// Get port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	// Start the server
	log.Printf("Server starting on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
