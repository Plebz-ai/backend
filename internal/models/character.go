package models

import (
	"time"

	"gorm.io/gorm"
)

type Character struct {
	gorm.Model  `json:"-"`
	ID          uint      `json:"id" gorm:"primarykey"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description" gorm:"not null"`
	Personality string    `json:"personality" gorm:"not null"`
	VoiceType   string    `json:"voice_type" gorm:"not null"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateCharacterRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description" binding:"required"`
	Personality string `json:"personality" binding:"required"`
	VoiceType   string `json:"voice_type" binding:"required"`
}
