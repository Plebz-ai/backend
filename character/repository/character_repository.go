package repository

import (
	"ai-agent-character-demo/backend/character/models"

	"gorm.io/gorm"
)

type CharacterRepository interface {
	Create(character *models.Character) error
	GetByID(id uint) (*models.Character, error)
	GetAll() ([]models.Character, error)
}

type GormCharacterRepository struct {
	db *gorm.DB
}

func NewGormCharacterRepository(db *gorm.DB) *GormCharacterRepository {
	return &GormCharacterRepository{db: db}
}

func (r *GormCharacterRepository) Create(character *models.Character) error {
	return r.db.Create(character).Error
}

func (r *GormCharacterRepository) GetByID(id uint) (*models.Character, error) {
	var character models.Character
	err := r.db.First(&character, id).Error
	if err != nil {
		return nil, err
	}
	return &character, nil
}

func (r *GormCharacterRepository) GetAll() ([]models.Character, error) {
	var characters []models.Character
	err := r.db.Find(&characters).Error
	if characters == nil {
		characters = []models.Character{}
	}
	return characters, err
}
