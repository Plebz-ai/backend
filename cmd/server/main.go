package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-agent-character-demo/backend/internal/models"
	"ai-agent-character-demo/backend/pkg/config"
	"ai-agent-character-demo/backend/pkg/di"
	"ai-agent-character-demo/backend/pkg/logger"
	"ai-agent-character-demo/backend/pkg/router"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize structured logger
	logConfig := logger.DefaultConfig()
	// Set log level from environment if available
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		logConfig.Level = level
	}
	// Set log format from environment if available
	logConfig.JSON = os.Getenv("LOG_FORMAT") != "text"

	log := logger.New(logConfig)
	logger.SetGlobal(log)

	log.Info("Starting application", "version", os.Getenv("APP_VERSION"))

	// Initialize database
	db, err := config.NewDB()
	if err != nil {
		log.LogError(err, "Failed to initialize database")
		os.Exit(1)
	}

	// Auto-migrate the schema
	if err := db.AutoMigrate(&models.Character{}, &models.Message{}, &models.User{}, &models.AudioChunk{}); err != nil {
		log.LogError(err, "Failed to migrate database")
		os.Exit(1)
	}

	// Create indexes for better query performance
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_messages_char_session ON messages(character_id, session_id)").Error; err != nil {
		log.LogError(err, "Failed to create message index", "index", "idx_messages_char_session")
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_audio_session ON audio_chunks(session_id)").Error; err != nil {
		log.LogError(err, "Failed to create audio index", "index", "idx_audio_session")
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_audio_status ON audio_chunks(processing_status)").Error; err != nil {
		log.LogError(err, "Failed to create audio status index", "index", "idx_audio_status")
	}

	// Initialize dependency injection container
	diConfig := di.DefaultConfig()
	diConfig.LoggerConfig = logConfig
	diConfig.JWTSecret = os.Getenv("JWT_SECRET")
	if expiry := os.Getenv("JWT_EXPIRY_HOURS"); expiry != "" {
		if val, err := time.ParseDuration(expiry + "h"); err == nil {
			diConfig.JWTExpiryHours = int(val.Hours())
		}
	}

	container, err := di.New(db, diConfig)
	if err != nil {
		log.LogError(err, "Failed to initialize dependency container")
		os.Exit(1)
	}

	// Initialize and setup router
	r := router.New(container)
	r.SetupRoutes()

	// Add OpenAPI validation if schema file is available
	schemaPath := os.Getenv("OPENAPI_SCHEMA_PATH")
	if schemaPath != "" {
		r.AddOpenAPIValidation(schemaPath)
	}

	// Get port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r.Engine,
	}

	// Start the server in a goroutine
	go func() {
		log.Info("Server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.LogError(err, "Server failed to start")
			os.Exit(1)
		}
	}()

	// Setup graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive a signal
	<-quit
	log.Info("Shutting down server...")

	// Create a deadline to wait for
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown the server
	if err := srv.Shutdown(ctx); err != nil {
		log.LogError(err, "Server forced to shutdown")
	}

	log.Info("Server exited gracefully")
}
