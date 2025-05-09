package di

import (
	"ai-agent-character-demo/backend/ai"
	"ai-agent-character-demo/backend/internal/service"
	"ai-agent-character-demo/backend/internal/ws"
	"ai-agent-character-demo/backend/pkg/jwt"
	"ai-agent-character-demo/backend/pkg/logger" // Aliased to avoid conflicts
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Container holds all the dependencies for the application
type Container struct {
	DB                      *gorm.DB
	Logger                  *logger.Logger
	JWTService              *jwt.Service
	UserService             *service.UserService
	CharacterService        *service.CharacterService
	MessageService          *service.MessageService
	AudioService            *service.AudioService
	AIBridge                *ai.AIBridge
	AdapterService          *service.AdapterService
	CharacterServiceAdapter *service.CharacterServiceAdapter
	MessageServiceAdapter   *service.MessageServiceAdapter
	AIServiceAdapter        *service.AIServiceAdapter
}

// Config holds the configuration for the container
type Config struct {
	DBConfig           *gorm.Config
	LoggerConfig       logger.Config
	JWTSecret          string
	JWTExpiryHours     int
	AudioServiceConfig service.AudioServiceConfig
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		DBConfig:       &gorm.Config{},
		LoggerConfig:   logger.DefaultConfig(),
		JWTSecret:      "",
		JWTExpiryHours: 0, // Use default
		AudioServiceConfig: service.AudioServiceConfig{
			MaxChunksPerSession: 5000,
			DefaultTTL:          24 * 60 * 60 * 1000000000, // 24 hours in nanoseconds
		},
	}
}

// New creates a new dependency injection container
func New(db *gorm.DB, config *Config) (*Container, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Initialize the logger
	log := logger.New(config.LoggerConfig)

	// Initialize JWT service with config parameters
	jwtService := jwt.NewService(config.JWTSecret, time.Duration(config.JWTExpiryHours)*time.Hour)

	// Initialize core services
	userService := service.NewUserService(db, jwtService)
	characterService := service.NewCharacterService(db)
	messageService := service.NewMessageService(db)
	audioService := service.NewAudioServiceWithConfig(db, config.AudioServiceConfig)

	// Initialize AI Bridge
	aiBridge, err := ai.NewAIBridge()
	if err != nil {
		return nil, fmt.Errorf("failed to create AI Bridge: %w", err)
	}

	// Initialize adapter service
	adapterService, err := service.NewAdapterService(audioService, characterService, userService)
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter service: %w", err)
	}

	// Initialize service adapters
	characterServiceAdapter := service.NewCharacterServiceAdapter(characterService)
	messageServiceAdapter := service.NewMessageServiceAdapter(messageService)
	// Create AIServiceAdapter with function adapters
	aiServiceAdapter := service.NewAIServiceAdapter(
		// GenerateResponse adapter
		func(character *ws.Character, userMessage string, history []ws.ChatMessage) (string, error) {
			// Pass internal/ws types directly (no conversion needed)
			return aiBridge.GenerateTextResponse(character, userMessage, history)
		},
		// TextToSpeech adapter
		func(ctx context.Context, text string, voiceType string) ([]byte, error) {
			return aiBridge.TextToSpeech(ctx, text, voiceType)
		},
		// SpeechToText adapter
		func(ctx context.Context, sessionID string, audioData []byte) (string, string, error) {
			return aiBridge.SpeechToText(ctx, sessionID, audioData)
		},
	)

	return &Container{
		DB:                      db,
		Logger:                  log,
		JWTService:              jwtService,
		UserService:             userService,
		CharacterService:        characterService,
		MessageService:          messageService,
		AudioService:            audioService,
		AIBridge:                aiBridge,
		AdapterService:          adapterService,
		CharacterServiceAdapter: characterServiceAdapter,
		MessageServiceAdapter:   messageServiceAdapter,
		AIServiceAdapter:        aiServiceAdapter,
	}, nil
}
