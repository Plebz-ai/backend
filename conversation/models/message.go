package models

import (
	"time"
)

// Message represents a chat message
type Message struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	ExternalID  string    `json:"external_id" gorm:"index"`
	CharacterID uint      `json:"character_id" gorm:"index"`
	SessionID   string    `json:"session_id" gorm:"index"`
	Sender      string    `json:"sender"`
	Content     string    `json:"content"`
	Timestamp   time.Time `json:"timestamp"`
	CreatedAt   time.Time `json:"created_at"`
}
