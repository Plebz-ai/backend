package models

import (
	"time"

	"gorm.io/gorm"
)

type Character struct {
	gorm.Model    `json:"-"`
	ID            uint      `json:"id" gorm:"primarykey"`
	Name          string    `json:"name" gorm:"not null"`
	Description   string    `json:"description" gorm:"not null"`
	Personality   string    `json:"personality" gorm:"not null"`
	Background    string    `json:"background"`
	Category      string    `json:"category"`
	Traits        []string  `json:"traits" gorm:"type:text[]"`
	Goals         []string  `json:"goals" gorm:"type:text[]"`
	Fears         []string  `json:"fears" gorm:"type:text[]"`
	Relationships []string  `json:"relationships" gorm:"type:text[]"`
	VoiceType     string    `json:"voice_type" gorm:"not null"`
	VoiceGender   string    `json:"voice_gender"`
	VoiceStyle    string    `json:"voice_style"`
	AvatarURL     string    `json:"avatar_url"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	IsCustom      bool      `json:"is_custom" gorm:"default:false"` // Only true for custom characters
}

type CreateCharacterRequest struct {
	Name          string   `json:"name" binding:"required"`
	Description   string   `json:"description" binding:"required"`
	Personality   string   `json:"personality" binding:"required"`
	Background    string   `json:"background"`
	Category      string   `json:"category"`
	Traits        []string `json:"traits"`
	Goals         []string `json:"goals"`
	Fears         []string `json:"fears"`
	Relationships []string `json:"relationships"`
	VoiceType     string   `json:"voice_type" binding:"required"`
	VoiceGender   string   `json:"voice_gender"`
	VoiceStyle    string   `json:"voice_style"`
	AvatarURL     string   `json:"avatar_url"`
	IsCustom      bool     `json:"is_custom"`
}
