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
	AvatarURL   string    `json:"avatar_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	IsCustom    bool      `json:"is_custom" gorm:"default:false"` // Only true for custom characters
}

type CreateCharacterRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description" binding:"required"`
	Personality string `json:"personality" binding:"required"`
	VoiceType   string `json:"voice_type" binding:"required"`
	AvatarURL   string `json:"avatar_url"`
	IsCustom    bool   `json:"is_custom"`
}
