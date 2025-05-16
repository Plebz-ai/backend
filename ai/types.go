package ai

import "time"

// Message represents a chat message
// (If you want to use ChatMessage, import from ws or define here)
// ChatMessage is now defined in ai_service.go for local use

type STTRequest struct {
	AudioData string `json:"audio_data"`
}

type STTResponse struct {
	Transcript string `json:"transcript"`
}

type TTSRequest struct {
	Text      string `json:"text"`
	VoiceName string `json:"voice_name"`
}

type TTSResponse struct {
	AudioData string `json:"audioData"`
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
