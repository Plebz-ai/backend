package service

import (
	"ai-agent-character-demo/backend/character/models"
	"ai-agent-character-demo/backend/character/repository"
)

type CharacterService struct {
	repo repository.CharacterRepository
}

func NewCharacterService(repo repository.CharacterRepository) *CharacterService {
	return &CharacterService{repo: repo}
}

func (s *CharacterService) CreateCharacter(character *models.Character) error {
	// Add business logic, validation, etc. here
	return s.repo.Create(character)
}

func (s *CharacterService) GetCharacterByID(id uint) (*models.Character, error) {
	return s.repo.GetByID(id)
}

func (s *CharacterService) GetAllCharacters() ([]models.Character, error) {
	return s.repo.GetAll()
}
