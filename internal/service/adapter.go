package service

import (
	"context"
	"time"

	"ai-agent-character-demo/backend/internal/models"
	"ai-agent-character-demo/backend/internal/ws"

	"gorm.io/gorm"
)

// CharacterServiceAdapter adapts the CharacterService to the ws.CharacterService interface
type CharacterServiceAdapter struct {
	service *CharacterService
}

// NewCharacterServiceAdapter creates a new adapter for the CharacterService
func NewCharacterServiceAdapter(service *CharacterService) *CharacterServiceAdapter {
	return &CharacterServiceAdapter{
		service: service,
	}
}

// GetCharacter implements the ws.CharacterService interface
func (a *CharacterServiceAdapter) GetCharacter(id uint) (*ws.Character, error) {
	character, err := a.service.GetCharacter(id)
	if err != nil {
		return nil, err
	}

	return &ws.Character{
		ID:          character.ID,
		Name:        character.Name,
		Description: character.Description,
		Personality: character.Personality,
		VoiceType:   character.VoiceType,
		CreatedAt:   character.CreatedAt,
		UpdatedAt:   character.UpdatedAt,
	}, nil
}

// AIServiceAdapter adapts an external AI service to the ws.AIService interface
type AIServiceAdapter struct {
	generateResponseFn func(character *ws.Character, userMessage string, history []ws.ChatMessage) (string, error)
	textToSpeechFn     func(ctx context.Context, text string, voiceType string) ([]byte, error)
	speechToTextFn     func(ctx context.Context, audioData []byte) (string, error)
}

// NewAIServiceAdapter creates a new adapter for an AI service
func NewAIServiceAdapter(
	generateResponseFn func(character *ws.Character, userMessage string, history []ws.ChatMessage) (string, error),
	textToSpeechFn func(ctx context.Context, text string, voiceType string) ([]byte, error),
	speechToTextFn func(ctx context.Context, audioData []byte) (string, error),
) *AIServiceAdapter {
	return &AIServiceAdapter{
		generateResponseFn: generateResponseFn,
		textToSpeechFn:     textToSpeechFn,
		speechToTextFn:     speechToTextFn,
	}
}

// GenerateResponse implements the ws.AIService interface
func (a *AIServiceAdapter) GenerateResponse(character *ws.Character, userMessage string, history []ws.ChatMessage) (string, error) {
	return a.generateResponseFn(character, userMessage, history)
}

// TextToSpeech implements the ws.AIService interface
func (a *AIServiceAdapter) TextToSpeech(ctx context.Context, text string, voiceType string) ([]byte, error) {
	return a.textToSpeechFn(ctx, text, voiceType)
}

// SpeechToText implements the ws.AIService interface
func (a *AIServiceAdapter) SpeechToText(ctx context.Context, audioData []byte) (string, error) {
	return a.speechToTextFn(ctx, audioData)
}

// MessageService handles message persistence
type MessageService struct {
	db *gorm.DB
}

// NewMessageService creates a new message service
func NewMessageService(db *gorm.DB) *MessageService {
	return &MessageService{
		db: db,
	}
}

// SaveMessage persists a message to the database
func (s *MessageService) SaveMessage(characterID uint, sessionID string, wsMessage *ws.ChatMessage) error {
	message := &models.Message{
		ExternalID:  wsMessage.ID,
		CharacterID: characterID,
		SessionID:   sessionID,
		Sender:      wsMessage.Sender,
		Content:     wsMessage.Content,
		Timestamp:   wsMessage.Timestamp,
		CreatedAt:   time.Now(),
	}

	return s.db.Create(message).Error
}

// GetSessionMessages retrieves messages for a specific session
func (s *MessageService) GetSessionMessages(characterID uint, sessionID string) ([]models.Message, error) {
	var messages []models.Message
	result := s.db.Where("character_id = ? AND session_id = ?", characterID, sessionID).
		Order("timestamp ASC").
		Find(&messages)

	return messages, result.Error
}

// MessageServiceAdapter adapts MessageService to be used with the WebSocket hub
type MessageServiceAdapter struct {
	messageService *MessageService
}

// NewMessageServiceAdapter creates a new message service adapter
func NewMessageServiceAdapter(messageService *MessageService) *MessageServiceAdapter {
	return &MessageServiceAdapter{
		messageService: messageService,
	}
}

// SaveMessage persists a message to the database
func (a *MessageServiceAdapter) SaveMessage(characterID uint, sessionID string, message *ws.ChatMessage) error {
	return a.messageService.SaveMessage(characterID, sessionID, message)
}

// GetSessionMessages retrieves messages for a specific session
func (a *MessageServiceAdapter) GetSessionMessages(characterID uint, sessionID string) ([]ws.ChatMessage, error) {
	dbMessages, err := a.messageService.GetSessionMessages(characterID, sessionID)
	if err != nil {
		return nil, err
	}

	// Convert from DB model to WS model
	wsMessages := make([]ws.ChatMessage, len(dbMessages))
	for i, msg := range dbMessages {
		wsMessages[i] = ws.ChatMessage{
			ID:        msg.ExternalID,
			Sender:    msg.Sender,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
	}

	return wsMessages, nil
}
