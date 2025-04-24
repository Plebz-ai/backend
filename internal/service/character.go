package service

import (
	"errors"
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
