package main

import (
	"ai-agent-character-demo/backend/ai"
	"ai-agent-character-demo/backend/internal/models"
	"ai-agent-character-demo/backend/internal/service"
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

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Initialize database
	db, err := setupDatabase()
	if err != nil {
		log.Fatalf("Failed to setup database: %v", err)
	}

	// Initialize services
	audioService := service.NewAudioService(db)
	userService := service.NewUserService(db)
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

	// Create a new HTTP server
	mux := http.NewServeMux()

	// Set up the API handlers
	bridge.SetupAPIHandlers(mux)

	// Set up basic routes for audio processing
	setupBasicRoutes(mux, audioService, adapterService)

	// Add additional handlers for your existing API
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Get the port from the environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	// Create the server
	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
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

// setupBasicRoutes sets up the basic API routes without requiring a full controller
func setupBasicRoutes(mux *http.ServeMux, audioService *service.AudioService, adapterService *service.AdapterService) {
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
	// Get database connection details from environment
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")

	if host == "" || port == "" || user == "" || dbname == "" {
		return nil, fmt.Errorf("missing required database configuration")
	}

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
