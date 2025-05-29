package main

import (
	"ai-agent-character-demo/backend/ai"
	"ai-agent-character-demo/backend/internal/api"
	"ai-agent-character-demo/backend/internal/models"
	"ai-agent-character-demo/backend/internal/service"
	"ai-agent-character-demo/backend/pkg/jwt"
	"ai-agent-character-demo/backend/pkg/logger"
	ws "ai-agent-character-demo/backend/pkg/ws"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	internalws "ai-agent-character-demo/backend/internal/ws"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	log.Printf("[DEBUG] JWT_SECRET at startup: %s", os.Getenv("JWT_SECRET"))

	// Initialize database
	db, err := setupDatabase()
	if err != nil {
		log.Fatalf("Failed to setup database: %v", err)
	}

	// Initialize JWT service
	jwtSecret := os.Getenv("JWT_SECRET")
	jwtExpiryHours := 24 * time.Hour // Default to 24 hours
	jwtService := jwt.NewService(jwtSecret, jwtExpiryHours)

	// Initialize services
	audioService := service.NewAudioService(db)
	userService := service.NewUserService(db, jwtService)
	characterService := service.NewCharacterService(db)

	// Initialize adapter service that connects to the AI layer
	adapterService, err := service.NewAdapterService(audioService, characterService, userService)
	if err != nil {
		log.Fatalf("Failed to create adapter service: %v", err)
	}

	// Start cleanup routine for adapter service
	adapterService.StartCleanupRoutine()

	// Create a new AIBridge
	bridge, err := ai.NewAIBridge()
	if err != nil {
		log.Fatalf("Failed to create AIBridge: %v", err)
	}

	// Set up logging
	logConfig := logger.DefaultConfig()
	logger := logger.New(logConfig)

	// Initialize Gin router
	ginEngine := gin.New()
	ginEngine.Use(gin.Recovery())
	ginEngine.Use(func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()

		logger.Info(fmt.Sprintf("[%d] %s %s - %v",
			statusCode,
			c.Request.Method,
			path,
			latency,
		))
	})

	// Initialize WebSocket hub with adapters
	characterServiceAdapter := service.NewCharacterServiceAdapter(characterService)
	messageServiceAdapter := service.NewMessageServiceAdapter(service.NewMessageService(db))
	aiServiceAdapter := createAIServiceAdapter(bridge)

	// Create WebSocket hub
	hub := internalws.NewHub(
		characterServiceAdapter,
		aiServiceAdapter,
		messageServiceAdapter,
	)

	// Set audio service in the hub for automatic audio storage
	hub.SetAudioService(audioService)

	// Start the hub
	go hub.Run()

	// Set up WebSocket route
	ginEngine.GET("/ws", func(c *gin.Context) {
		internalws.ServeWs(hub, c)
	})

	// Initialize controllers for legacy and v1 routes
	authHandler := api.NewAuthHandler(userService, jwtService, logger)
	characterHandler := service.NewCharacterService(db)
	characterApiHandler := api.NewCharacterHandler(characterHandler)
	messageService := service.NewMessageService(db)
	messageHandler := api.NewMessageController(messageService, characterHandler, aiServiceAdapter, jwtService)
	audioHandler := api.NewAudioController(audioService, jwtService)
	userController := api.NewUserController(db)

	// Set up /api legacy routes for frontend compatibility
	apiLegacy := ginEngine.Group("/api")
	{
		// Health
		apiLegacy.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok"})
		})

		// Auth
		apiLegacy.POST("/auth/login", authHandler.Login)
		apiLegacy.POST("/auth/signup", authHandler.Signup)
		apiLegacy.GET("/auth/me", func(c *gin.Context) {
			token := c.GetHeader("Authorization")
			if token == "" {
				c.JSON(401, gin.H{"error": "Authorization header is required"})
				return
			}
			if len(token) > 7 && token[:7] == "Bearer " {
				token = token[7:]
			}
			claims, err := jwtService.ValidateToken(token)
			if err != nil {
				c.JSON(401, gin.H{"error": "Invalid token"})
				return
			}
			c.Set("userId", claims.UserID)
			authHandler.Me(c)
		})

		// --- JWT Middleware for legacy protected routes ---
		legacyJWT := func(c *gin.Context) {
			token := c.GetHeader("Authorization")
			if token == "" {
				token = c.GetHeader("authorization") // Support lowercase header
			}
			if token == "" {
				c.JSON(401, gin.H{"error": "Authorization header is required"})
				c.Abort()
				return
			}
			if len(token) > 7 && token[:7] == "Bearer " {
				token = token[7:]
			}
			claims, err := jwtService.ValidateToken(token)
			if err != nil {
				c.JSON(401, gin.H{"error": "Invalid token"})
				c.Abort()
				return
			}
			c.Set("userId", claims.UserID)
			c.Next()
		}

		// Characters (protected)
		apiLegacy.GET("/characters", legacyJWT, characterApiHandler.ListCharacters)
		apiLegacy.GET("/characters/:id", legacyJWT, characterApiHandler.GetCharacter)
		apiLegacy.POST("/characters", legacyJWT, characterApiHandler.CreateCharacter)

		// Messages (protected)
		apiLegacy.GET("/messages/session/:sessionId", legacyJWT, messageHandler.GetSessionMessages)
		apiLegacy.POST("/messages/send", legacyJWT, messageHandler.SendMessage)

		// Audio (protected)
		apiLegacy.POST("/audio/upload", legacyJWT, audioHandler.UploadAudio)
		apiLegacy.GET("/audio/session/:sessionId", legacyJWT, audioHandler.GetSessionAudio)

		// Forgot Password (stub)
		apiLegacy.POST("/forgot-password", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "Password reset link sent (stub)."})
		})

		// Video Call (stub)
		apiLegacy.GET("/video-call/:id", func(c *gin.Context) {
			c.JSON(501, gin.H{"error": "Video call signaling not implemented in backend stub."})
		})

		// Build/Design/Add/Setup/Implement (stubs)
		apiLegacy.GET("/build", func(c *gin.Context) { c.JSON(501, gin.H{"error": "Not implemented (stub)"}) })
		apiLegacy.POST("/build", func(c *gin.Context) { c.JSON(501, gin.H{"error": "Not implemented (stub)"}) })
		apiLegacy.GET("/design", func(c *gin.Context) { c.JSON(501, gin.H{"error": "Not implemented (stub)"}) })
		apiLegacy.POST("/design", func(c *gin.Context) { c.JSON(501, gin.H{"error": "Not implemented (stub)"}) })
		apiLegacy.GET("/add", func(c *gin.Context) { c.JSON(501, gin.H{"error": "Not implemented (stub)"}) })
		apiLegacy.POST("/add", func(c *gin.Context) { c.JSON(501, gin.H{"error": "Not implemented (stub)"}) })
		apiLegacy.GET("/setup", func(c *gin.Context) { c.JSON(501, gin.H{"error": "Not implemented (stub)"}) })
		apiLegacy.POST("/setup", func(c *gin.Context) { c.JSON(501, gin.H{"error": "Not implemented (stub)"}) })
		apiLegacy.GET("/implement", func(c *gin.Context) { c.JSON(501, gin.H{"error": "Not implemented (stub)"}) })
		apiLegacy.POST("/implement", func(c *gin.Context) { c.JSON(501, gin.H{"error": "Not implemented (stub)"}) })

		// User (protected)
		userGroup := apiLegacy.Group("/user")
		userGroup.Use(legacyJWT)
		userGroup.GET("/preferences", userController.GetUserPreferences)
		userGroup.POST("/preferences", userController.SetUserPreferences)
	}

	// Catch-all for unhandled /api/* routes
	ginEngine.NoRoute(func(c *gin.Context) {
		if len(c.Request.URL.Path) >= 5 && c.Request.URL.Path[:5] == "/api/" {
			c.JSON(404, gin.H{"error": "API endpoint not found", "path": c.Request.URL.Path})
			return
		}
	})

	// Get the port from the environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	// Create the server using Gin engine only
	server := &http.Server{
		Addr:    ":" + port,
		Handler: ginEngine,
	}

	// Start the server in a goroutine
	go func() {
		log.Printf("Server listening on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Set up graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Wait for termination signal
	<-stop

	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown the server gracefully
	log.Println("Shutting down server...")
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Failed to shutdown server: %v", err)
	}

	log.Println("Server shutdown complete")
}

// Helper function to create AIServiceAdapter
func createAIServiceAdapter(bridge *ai.AIBridge) *service.AIServiceAdapter {
	return service.NewAIServiceAdapter(
		// GenerateResponse adapter
		func(character *ws.Character, userMessage string, history []ws.ChatMessage) (string, error) {
			return bridge.GenerateTextResponse(character, userMessage, history)
		},
		// TextToSpeech adapter
		func(ctx context.Context, text string, voiceType string) ([]byte, error) {
			return bridge.TextToSpeech(ctx, text, voiceType)
		},
		// SpeechToText adapter
		func(ctx context.Context, sessionID string, audioData []byte) (string, string, error) {
			transcript, _, err := bridge.SpeechToText(ctx, sessionID, audioData)
			return transcript, "", err
		},
	)
}

// setupBasicRoutes sets up the basic API routes without requiring a full controller
func setupBasicRoutes(mux *http.ServeMux, audioService *service.AudioService, adapterService *service.AdapterService) {
	// Serve static files from uploads directory
	uploadsDir := http.Dir("./uploads")
	fileServer := http.FileServer(uploadsDir)
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", fileServer))

	// Ensure uploads directory exists
	os.MkdirAll("./uploads", 0755)

	// Route to process an audio chunk
	mux.HandleFunc("/api/audio/process", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse form data
		err := r.ParseMultipartForm(10 << 20) // 10 MB max
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse form: %v", err), http.StatusBadRequest)
			return
		}

		// Get session ID
		sessionID := r.FormValue("sessionId")
		if sessionID == "" {
			http.Error(w, "Session ID is required", http.StatusBadRequest)
			return
		}

		// Get the audio file
		file, _, err := r.FormFile("audio")
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get audio file: %v", err), http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Read the file
		audioData, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to read audio file: %v", err), http.StatusInternalServerError)
			return
		}

		// Process the audio through the adapter service
		textResponse, audioResponse, err := adapterService.ProcessAudioData(r.Context(), sessionID, audioData)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to process audio: %v", err), http.StatusInternalServerError)
			return
		}

		// Return the response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"text":  textResponse,
			"audio": base64.StdEncoding.EncodeToString(audioResponse),
		})
	})

	// Route to create a session
	mux.HandleFunc("/api/session/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			SessionID   string `json:"sessionId"`
			UserID      string `json:"userId"`
			CharacterID uint   `json:"characterId"`
			AvatarURL   string `json:"avatarUrl"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse request: %v", err), http.StatusBadRequest)
			return
		}

		if err := adapterService.StartSession(req.SessionID, req.UserID, req.CharacterID, req.AvatarURL); err != nil {
			http.Error(w, fmt.Sprintf("Failed to start session: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "success",
		})
	})
}

// setupDatabase initializes the database connection and runs migrations
func setupDatabase() (*gorm.DB, error) {
	// Get database connection details from environment with defaults for local development
	host := getEnvOrDefault("DB_HOST", "localhost")
	port := getEnvOrDefault("DB_PORT", "5432")
	user := getEnvOrDefault("DB_USER", "postgres")
	password := getEnvOrDefault("DB_PASSWORD", "postgres")
	dbname := getEnvOrDefault("DB_NAME", "character_demo")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Run migrations
	if err := db.AutoMigrate(
		&models.User{},
		&models.Character{},
		&models.AudioChunk{},
		&models.Message{},
	); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Println("Database migrations completed successfully")
	return db, nil
}

// Helper function to get environment variable with default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
