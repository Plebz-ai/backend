package service

import (
	"ai-agent-character-demo/backend/audio/models"
	"ai-agent-character-demo/backend/audio/repository"
)

type AudioService struct {
	repo repository.AudioRepository
}

func NewAudioService(repo repository.AudioRepository) *AudioService {
	return &AudioService{repo: repo}
}

func (s *AudioService) CreateAudioChunk(chunk *models.AudioChunk) error {
	// Add business logic, validation, etc. here
	return s.repo.Create(chunk)
}

func (s *AudioService) GetAudioChunkByID(id uint) (*models.AudioChunk, error) {
	return s.repo.GetByID(id)
}

func (s *AudioService) GetAudioChunksBySession(sessionID string) ([]models.AudioChunk, error) {
	return s.repo.GetBySession(sessionID)
}
