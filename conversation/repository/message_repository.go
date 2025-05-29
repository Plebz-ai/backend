package repository

import (
	"ai-agent-character-demo/backend/conversation/models"

	"gorm.io/gorm"
)

type MessageRepository interface {
	Create(message *models.Message) error
	GetByID(id uint) (*models.Message, error)
	GetBySession(sessionID string) ([]models.Message, error)
	GetBySessionPaginated(sessionID string, limit, offset int) ([]models.Message, error)
}

type GormMessageRepository struct {
	db *gorm.DB
}

func NewGormMessageRepository(db *gorm.DB) *GormMessageRepository {
	return &GormMessageRepository{db: db}
}

func (r *GormMessageRepository) Create(message *models.Message) error {
	return r.db.Create(message).Error
}

func (r *GormMessageRepository) GetByID(id uint) (*models.Message, error) {
	var message models.Message
	err := r.db.First(&message, id).Error
	if err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *GormMessageRepository) GetBySession(sessionID string) ([]models.Message, error) {
	var messages []models.Message
	err := r.db.Where("session_id = ?", sessionID).Find(&messages).Error
	return messages, err
}

func (r *GormMessageRepository) GetBySessionPaginated(sessionID string, limit, offset int) ([]models.Message, error) {
	var messages []models.Message
	err := r.db.Where("session_id = ?", sessionID).
		Order("timestamp ASC").
		Limit(limit).
		Offset(offset).
		Find(&messages).Error
	return messages, err
}
