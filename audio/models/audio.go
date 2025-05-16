package models

import (
	"time"

	"gorm.io/gorm"
)

// AudioChunk represents a temporary stored audio fragment from a user
type AudioChunk struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	UserID           string    `json:"user_id"`
	SessionID        string    `json:"session_id" gorm:"index"`
	CharID           uint      `json:"char_id"`
	AudioData        []byte    `json:"audio_data"`
	Format           string    `json:"format" gorm:"default:webm"`
	Duration         float64   `json:"duration"`
	SampleRate       int       `json:"sample_rate" gorm:"default:48000"`
	Channels         int       `json:"channels" gorm:"default:1"`
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at" gorm:"index"`
	Metadata         string    `json:"metadata"`
	ProcessingStatus string    `json:"processing_status" gorm:"default:pending"`
}

func (a *AudioChunk) BeforeCreate(tx *gorm.DB) error {
	if a.ExpiresAt.IsZero() {
		a.ExpiresAt = time.Now().Add(24 * time.Hour)
	}
	return nil
}

func (a *AudioChunk) Expired() bool {
	return time.Now().After(a.ExpiresAt)
}

func (AudioChunk) TableName() string {
	return "audio_chunks"
}
