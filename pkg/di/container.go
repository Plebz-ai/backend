package di

import (
	"ai-agent-character-demo/backend/ai"
	"ai-agent-character-demo/backend/internal/service"
	"ai-agent-character-demo/backend/pkg/jwt"
	"ai-agent-character-demo/backend/pkg/logger" // Aliased to avoid conflicts
	"ai-agent-character-demo/backend/pkg/ws"
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
	AI_Layer2Client         *ai.AI_Layer2Client
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

	// Initialize AI_Layer2Client
	aiLayer2Client, err := ai.NewAI_Layer2Client()
	if err != nil {
		return nil, fmt.Errorf("failed to create AI_Layer2Client: %w", err)
	}

	// Initialize adapter service
	adapterService, err := service.NewAdapterService(audioService, characterService, userService)
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter service: %w", err)
	}

	// Initialize service adapters
	characterServiceAdapter := service.NewCharacterServiceAdapter(characterService)
	messageServiceAdapter := service.NewMessageServiceAdapter(messageService)
	// Create AIServiceAdapter with function adapters (using AI_Layer2Client)
	aiServiceAdapter := service.NewAIServiceAdapter(
		// GenerateResponse adapter (calls LLM1 then LLM2)
		func(character *ws.Character, userMessage string, history []ws.ChatMessage) (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			userID := ""
			// Build character details map for LLM1
			characterDetails := map[string]interface{}{
				"id":          character.ID,
				"name":        character.Name,
				"description": character.Description,
				"personality": character.Personality,
				"voice_type":  character.VoiceType,
			}
			// TODO: Pass session ID if available
			contextResp, err := aiLayer2Client.GenerateContext(ctx, ai.ContextRequest{
				UserInput:        userMessage,
				CharacterDetails: characterDetails,
				SessionID:        "", // Pass session ID if you have it
			})
			if err != nil {
				return "", fmt.Errorf("context gen failed: %w", err)
			}
			resp, err := aiLayer2Client.GenerateResponse(ctx, ai.ResponseRequest{
				CharacterID: character.ID,
				UserID:      userID,
				Context:     contextResp.Context,
				Message:     userMessage,
				History:     history,
			})
			return resp, err
		},
		// TextToSpeech adapter
		func(ctx context.Context, text string, voiceType string) ([]byte, error) {
			return aiLayer2Client.TextToSpeech(ctx, text, voiceType)
		},
		// SpeechToText adapter
		func(ctx context.Context, sessionID string, audioData []byte) (string, string, error) {
			transcript, err := aiLayer2Client.SpeechToText(ctx, sessionID, audioData)
			return transcript, "", err
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
		AI_Layer2Client:         aiLayer2Client,
		AdapterService:          adapterService,
		CharacterServiceAdapter: characterServiceAdapter,
		MessageServiceAdapter:   messageServiceAdapter,
		AIServiceAdapter:        aiServiceAdapter,
	}, nil
}
