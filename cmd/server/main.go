package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-agent-character-demo/backend/ai"
	"ai-agent-character-demo/backend/internal/api"
	"ai-agent-character-demo/backend/internal/models"
	"ai-agent-character-demo/backend/internal/service"
	"ai-agent-character-demo/backend/internal/ws"
	"ai-agent-character-demo/backend/pkg/config"
	"ai-agent-character-demo/backend/pkg/jwt"

	"github.com/gorilla/websocket"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for simplicity; adjust for production
	},
}

// Helper function to get minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			log.Println("WebSocket read error:", err)
			break
		}

		// Echo the message back for now; replace with actual processing
		if err := conn.WriteMessage(messageType, p); err != nil {
			log.Println("WebSocket write error:", err)
			break
		}
	}
}

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

	// Create indexes for better query performance
	// These are safe operations that will help with scaling
	db.Exec("CREATE INDEX IF NOT EXISTS idx_messages_char_session ON messages(character_id, session_id)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_audio_session ON audio_chunks(session_id)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_audio_status ON audio_chunks(processing_status)")

	// Initialize Gin router
	r := gin.Default()

	// CORS middleware
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		// Make sure to allow the frontend port 3001
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

	// Create AI Bridge for real AI Layer integration
	aiBridge, err := ai.NewAIBridge()
	if err != nil {
		log.Fatalf("Failed to create AI Bridge: %v", err)
	}

	// Initialize AI Layer sessions
	sessionRegistry := make(map[string]bool)
	sessionMutex := sync.Mutex{}

	// Create AI Service adapter using real AI Bridge
	aiServiceAdapter := service.NewAIServiceAdapter(
		func(character *ws.Character, userMessage string, history []ws.ChatMessage) (string, error) {
			// Use the character's data to generate a response
			return "Hello! I'm " + character.Name + ". Thanks for your message: \"" + userMessage + "\"", nil
		},
		func(ctx context.Context, text string, voiceType string) ([]byte, error) {
			// Real TTS implementation using AI Bridge
			return aiBridge.TextToSpeech(ctx, text, voiceType)
		},
		func(ctx context.Context, sessionID string, audioData []byte) (string, string, error) {
			// Ensure the session is registered with the AI Bridge
			sessionMutex.Lock()
			if !sessionRegistry[sessionID] {
				log.Printf("Registering session %s with AI Bridge", sessionID)
				// Extract character ID from the session ID format (session-{charID}-{clientID}-{timestamp})
				parts := strings.Split(sessionID, "-")
				var charID uint = 1 // Default character ID
				if len(parts) > 1 {
					if id, err := strconv.ParseUint(parts[1], 10, 32); err == nil {
						charID = uint(id)
						log.Printf("Extracted character ID %d from session ID %s", charID, sessionID)
					}
				}

				// Register the session with AI Bridge
				aiBridge.RegisterSession(sessionID, charID, "user-"+sessionID, "")
				sessionRegistry[sessionID] = true
			}
			sessionMutex.Unlock()

			// Process the audio chunk with the correct session ID
			log.Printf("Processing audio for session %s (%d bytes)", sessionID, len(audioData))
			transcript, aiResponse, err := aiBridge.ProcessAudioChunk(ctx, sessionID, audioData)
			if err != nil {
				return "", "", err
			}

			// If we got an AI response from the LLM_Layer, return both transcript and response
			if aiResponse != "" {
				log.Printf("Using AI response from LLM_Layer: %s", aiResponse[:min(50, len(aiResponse))])
			}

			// Return both the transcript and AI response
			return transcript, aiResponse, nil
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

	http.HandleFunc("/ws", handleWebSocket)
	log.Println("WebSocket server started on /ws")

	// Health check endpoints - Add both paths for compatibility
	healthHandler := func(c *gin.Context) {
		// Check database connection
		dbStatus := "ok"
		if err := db.Exec("SELECT 1").Error; err != nil {
			dbStatus = fmt.Sprintf("error: %v", err)
		}

		// Get count of active connections
		activeConnections := len(hub.GetActiveConnections())

		// Get memory stats
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		c.JSON(200, gin.H{
			"status":    "ok",
			"version":   os.Getenv("APP_VERSION"),
			"timestamp": time.Now().Format(time.RFC3339),
			"components": gin.H{
				"database": dbStatus,
				"websocket": gin.H{
					"status":             "ok",
					"active_connections": activeConnections,
				},
			},
			"memory": gin.H{
				"alloc_mb":  memStats.Alloc / 1024 / 1024,
				"sys_mb":    memStats.Sys / 1024 / 1024,
				"gc_cycles": memStats.NumGC,
			},
		})
	}

	// Register both health endpoint paths
	r.GET("/health", healthHandler)
	r.GET("/api/health", healthHandler) // Add this path that's being requested

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
