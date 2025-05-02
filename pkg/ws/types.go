package ws

import (
	"time"
)

// Character represents a character in the WebSocket context
type Character struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Personality string `json:"personality"`
	VoiceType   string `json:"voiceType"`
}

// ChatMessage represents a message in the chat history
type ChatMessage struct {
	ID        string    `json:"id"`
	Sender    string    `json:"sender"` // "user" or "character"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}
