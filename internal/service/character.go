package service

import (
	"errors"
	"fmt"
	"log"
	"time"

	"ai-agent-character-demo/backend/internal/models"
	"ai-agent-character-demo/backend/internal/ws"

	"gorm.io/gorm"
)

type CharacterService struct {
	db *gorm.DB
}

func NewCharacterService(db *gorm.DB) *CharacterService {
	return &CharacterService{
		db: db,
	}
}

func (s *CharacterService) CreateCharacter(req *models.CreateCharacterRequest) (*models.Character, error) {
	if req.Name == "" {
		return nil, errors.New("character name is required")
	}
	if req.Description == "" {
		return nil, errors.New("character description is required")
	}
	if req.Personality == "" {
		return nil, errors.New("character personality is required")
	}
	if req.VoiceType == "" {
		return nil, errors.New("character voice type is required")
	}

	character := &models.Character{
		Name:        req.Name,
		Description: req.Description,
		Personality: req.Personality,
		VoiceType:   req.VoiceType,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result := s.db.Create(character)
	if result.Error != nil {
		return nil, result.Error
	}

	return character, nil
}

func (s *CharacterService) GetCharacter(id uint) (*models.Character, error) {
	var character models.Character
	result := s.db.First(&character, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &character, nil
}

// GetWebSocketCharacter converts a models.Character to a ws.Character for WebSocket use
func (s *CharacterService) GetWebSocketCharacter(id uint) (*ws.Character, error) {
	character, err := s.GetCharacter(id)
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

func (s *CharacterService) ListCharacters() ([]models.Character, error) {
	var characters []models.Character
	result := s.db.Find(&characters)
	if result.Error != nil {
		return nil, result.Error
	}
	return characters, nil
}

// ListCharactersWithConversations returns characters that have conversations with a specific user
func (s *CharacterService) ListCharactersWithConversations(userID uint) ([]models.Character, error) {
	var characters []models.Character

	// Find all unique character IDs from the messages table where the session involves the user
	// Query to find all sessionIDs that contain userID in their name
	// This approach depends on how you're creating session IDs - we need to adapt based on the exact format

	// First, let's get all unique character IDs from messages where sessionID contains userID
	var characterIDs []uint

	// This query attempts to find all characters that have messages in sessions involving the user
	// The query looks for sessions where the session ID contains the user ID as a string
	userIDStr := fmt.Sprintf("%d", userID)
	query := s.db.Table("messages").
		Select("DISTINCT character_id").
		Where("session_id LIKE ?", "%"+userIDStr+"%").
		Pluck("character_id", &characterIDs)

	if query.Error != nil {
		return nil, query.Error
	}

	// If no characters found with the current session pattern, return all characters as a fallback
	if len(characterIDs) == 0 {
		log.Printf("No character conversations found for user %d, returning all characters", userID)
		return s.ListCharacters()
	}

	// Now get the character details
	result := s.db.Where("id IN ?", characterIDs).Find(&characters)
	if result.Error != nil {
		return nil, result.Error
	}

	return characters, nil
}
