package repository

import (
	"ai-agent-character-demo/backend/audio/models"
	"gorm.io/gorm"
)

type AudioRepository interface {
	Create(chunk *models.AudioChunk) error
	GetByID(id uint) (*models.AudioChunk, error)
	GetBySession(sessionID string) ([]models.AudioChunk, error)
}

type GormAudioRepository struct {
	db *gorm.DB
}

func NewGormAudioRepository(db *gorm.DB) *GormAudioRepository {
	return &GormAudioRepository{db: db}
}

func (r *GormAudioRepository) Create(chunk *models.AudioChunk) error {
	return r.db.Create(chunk).Error
}

func (r *GormAudioRepository) GetByID(id uint) (*models.AudioChunk, error) {
	var chunk models.AudioChunk
	err := r.db.First(&chunk, id).Error
	if err != nil {
		return nil, err
	}
	return &chunk, nil
}

func (r *GormAudioRepository) GetBySession(sessionID string) ([]models.AudioChunk, error) {
	var chunks []models.AudioChunk
	err := r.db.Where("session_id = ?", sessionID).Find(&chunks).Error
	return chunks, err
} 