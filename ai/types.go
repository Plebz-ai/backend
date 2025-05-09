package ai

import "time"

// Message represents a chat message
type Message struct {
	ID        string    `json:"id"`
	Sender    string    `json:"sender"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Character represents a character in the system
type Character struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Personality string    `json:"personality"`
	VoiceType   string    `json:"voice_type"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Custom      bool      `json:"custom"`
}

// Helper function to avoid panic with string substring
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
