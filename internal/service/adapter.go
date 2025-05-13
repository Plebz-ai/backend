package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"ai-agent-character-demo/backend/ai"
	"ai-agent-character-demo/backend/internal/models"
	ws "ai-agent-character-demo/backend/pkg/ws"

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
func (a *CharacterServiceAdapter) GetCharacter(id uint, userID string) (*ws.Character, error) {
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
	}, nil
}

// AIServiceAdapter adapts an external AI service to the ws.AIService interface
type AIServiceAdapter struct {
	generateResponseFn func(character *ws.Character, userMessage string, history []ws.ChatMessage) (string, error)
	textToSpeechFn     func(ctx context.Context, text string, voiceType string) ([]byte, error)
	speechToTextFn     func(ctx context.Context, sessionID string, audioData []byte) (string, string, error)
}

// NewAIServiceAdapter creates a new adapter for an AI service
func NewAIServiceAdapter(
	generateResponseFn func(character *ws.Character, userMessage string, history []ws.ChatMessage) (string, error),
	textToSpeechFn func(ctx context.Context, text string, voiceType string) ([]byte, error),
	speechToTextFn func(ctx context.Context, sessionID string, audioData []byte) (string, string, error),
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
func (a *AIServiceAdapter) SpeechToText(ctx context.Context, sessionID string, audioData []byte) (string, string, error) {
	return a.speechToTextFn(ctx, sessionID, audioData)
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

// AdapterService handles the connection between the audio service and AI layer
type AdapterService struct {
	audioService *AudioService
	charService  *CharacterService
	userService  *UserService
	aiBridge     *ai.AIBridge
	sessions     map[string]*SessionInfo
}

// SessionInfo tracks information about an active session
type SessionInfo struct {
	UserID      string
	CharacterID uint
	AvatarURL   string
	StartTime   time.Time
	LastActive  time.Time
}

// NewAdapterService creates a new adapter service
func NewAdapterService(audioService *AudioService, charService *CharacterService, userService *UserService) (*AdapterService, error) {
	aiBridge, err := ai.NewAIBridge()
	if err != nil {
		return nil, fmt.Errorf("failed to create AI bridge: %v", err)
	}

	return &AdapterService{
		audioService: audioService,
		charService:  charService,
		userService:  userService,
		aiBridge:     aiBridge,
		sessions:     make(map[string]*SessionInfo),
	}, nil
}

// StartSession initializes a new session with the AI
func (s *AdapterService) StartSession(sessionID, userID string, characterID uint, avatarURL string) error {
	// Convert userID from string to uint for the user service
	var userIDUint uint
	_, err := fmt.Sscanf(userID, "%d", &userIDUint)
	if err != nil {
		return fmt.Errorf("invalid user ID format: %v", err)
	}

	// Check if the user exists
	user, err := s.userService.GetUserByID(userIDUint)
	if err != nil {
		return fmt.Errorf("invalid user: %v", err)
	}

	// Check if the character exists
	character, err := s.charService.GetCharacter(characterID)
	if err != nil {
		return fmt.Errorf("invalid character: %v", err)
	}

	// Initialize session context in the AI bridge
	s.aiBridge.RegisterSession(sessionID, characterID, userID, avatarURL)

	// Record session info
	s.sessions[sessionID] = &SessionInfo{
		UserID:      userID,
		CharacterID: characterID,
		AvatarURL:   avatarURL,
		StartTime:   time.Now(),
		LastActive:  time.Now(),
	}

	log.Printf("Started session %s for user %s with character %s", sessionID, user.Name, character.Name)
	return nil
}

// EndSession cleans up a session
func (s *AdapterService) EndSession(sessionID string) error {
	// Check if session exists
	if _, exists := s.sessions[sessionID]; !exists {
		return errors.New("session not found")
	}

	// Remove from our tracking
	delete(s.sessions, sessionID)

	log.Printf("Ended session %s", sessionID)
	return nil
}

// ProcessAudioChunk handles an audio chunk through the AI pipeline
func (s *AdapterService) ProcessAudioChunk(ctx context.Context, chunkID string) (string, []byte, error) {
	// Retrieve the chunk
	chunk, err := s.audioService.GetAudioChunk(chunkID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to retrieve audio chunk: %v", err)
	}

	// Update status to processing
	if err := s.audioService.UpdateProcessingStatus(chunkID, "processing"); err != nil {
		log.Printf("Warning: Failed to update processing status: %v", err)
	}

	// Check if session exists and update last active time
	sessionInfo, exists := s.sessions[chunk.SessionID]
	if !exists {
		// Try to create the session if it doesn't exist
		if err := s.StartSession(chunk.SessionID, chunk.UserID, chunk.CharID, ""); err != nil {
			return "", nil, fmt.Errorf("session not found and could not be created: %v", err)
		}
		sessionInfo = s.sessions[chunk.SessionID]
	}
	sessionInfo.LastActive = time.Now()

	// 1. Speech-to-text
	transcript, _, err := s.aiBridge.SpeechToText(ctx, chunk.SessionID, chunk.AudioData)
	if err != nil {
		s.audioService.UpdateProcessingStatus(chunkID, "failed")
		return "", nil, fmt.Errorf("speech-to-text processing failed: %v", err)
	}

	// 2. Generate response (chat)
	character, err := s.charService.GetCharacter(chunk.CharID)
	if err != nil {
		s.audioService.UpdateProcessingStatus(chunkID, "failed")
		return "", nil, fmt.Errorf("failed to get character: %v", err)
	}
	wsChar := &ws.Character{
		ID:          character.ID,
		Name:        character.Name,
		Description: character.Description,
		Personality: character.Personality,
		VoiceType:   character.VoiceType,
	}
	// Optionally, fetch conversation history if needed
	var history []ws.ChatMessage
	textResponse, err := s.aiBridge.GenerateTextResponse(wsChar, transcript, history)
	if err != nil {
		s.audioService.UpdateProcessingStatus(chunkID, "failed")
		return "", nil, fmt.Errorf("response generation failed: %v", err)
	}

	// 3. Text-to-speech
	audioResponse, err := s.aiBridge.TextToSpeech(ctx, textResponse, wsChar.VoiceType)
	if err != nil {
		log.Printf("Warning: Failed to generate speech for response: %v", err)
	}

	// Update status to completed
	if err := s.audioService.UpdateProcessingStatus(chunkID, "completed"); err != nil {
		log.Printf("Warning: Failed to update processing status: %v", err)
	}

	return textResponse, audioResponse, nil
}

// GetSessionInfo returns information about a session
func (s *AdapterService) GetSessionInfo(sessionID string) (*SessionInfo, error) {
	sessionInfo, exists := s.sessions[sessionID]
	if !exists {
		return nil, errors.New("session not found")
	}
	return sessionInfo, nil
}

// UpdateAvatarForSession is now a no-op (avatar streaming not supported)
func (s *AdapterService) UpdateAvatarForSession(sessionID string, avatarURL string) error {
	return nil
}

// ListActiveSessions returns a list of all active sessions
func (s *AdapterService) ListActiveSessions() []*SessionInfo {
	result := make([]*SessionInfo, 0, len(s.sessions))
	for _, info := range s.sessions {
		result = append(result, info)
	}
	return result
}

// CleanupInactiveSessions removes sessions that have been inactive for too long
func (s *AdapterService) CleanupInactiveSessions(maxInactiveTime time.Duration) int {
	cutoff := time.Now().Add(-maxInactiveTime)
	count := 0

	for sessionID, info := range s.sessions {
		if info.LastActive.Before(cutoff) {
			s.EndSession(sessionID)
			count++
		}
	}

	return count
}

// StartCleanupRoutine initiates a background routine to clean up inactive sessions
func (s *AdapterService) StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			count := s.CleanupInactiveSessions(2 * time.Hour)
			if count > 0 {
				log.Printf("Cleaned up %d inactive sessions", count)
			}
		}
	}()
}

// ProcessAudioData processes audio data directly without storing it as a chunk
func (s *AdapterService) ProcessAudioData(ctx context.Context, sessionID string, audioData []byte) (string, []byte, error) {
	// Check if session exists and update last active time
	sessionInfo, exists := s.sessions[sessionID]
	if !exists {
		return "", nil, errors.New("session not found")
	}
	sessionInfo.LastActive = time.Now()

	// 1. Speech-to-text
	transcript, _, err := s.aiBridge.SpeechToText(ctx, sessionID, audioData)
	if err != nil {
		return "", nil, fmt.Errorf("speech-to-text processing failed: %v", err)
	}

	// 2. Generate response (chat)
	character, err := s.charService.GetCharacter(sessionInfo.CharacterID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get character: %v", err)
	}
	wsChar := &ws.Character{
		ID:          character.ID,
		Name:        character.Name,
		Description: character.Description,
		Personality: character.Personality,
		VoiceType:   character.VoiceType,
	}
	var history []ws.ChatMessage
	textResponse, err := s.aiBridge.GenerateTextResponse(wsChar, transcript, history)
	if err != nil {
		return "", nil, fmt.Errorf("response generation failed: %v", err)
	}

	// 3. Text-to-speech
	audioResponse, err := s.aiBridge.TextToSpeech(ctx, textResponse, wsChar.VoiceType)
	if err != nil {
		log.Printf("Warning: Failed to generate speech for response: %v", err)
	}

	return textResponse, audioResponse, nil
}
