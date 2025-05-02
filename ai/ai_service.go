package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

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
}

// AIService handles AI-related operations including text generation,
// text-to-speech, and speech-to-text conversions
type AIService struct {
	openAIKey        string
	elevenLabsKey    string
	httpClient       *http.Client
	useLocalModelAPI bool
	localModelURL    string
}

// NewAIService creates a new instance of the AIService
func NewAIService() (*AIService, error) {
	openAIKey := os.Getenv("OPENAI_API_KEY")
	elevenLabsKey := os.Getenv("ELEVENLABS_API_KEY")
	useLocalModel := os.Getenv("USE_LOCAL_MODEL") == "true"
	localModelURL := os.Getenv("LOCAL_MODEL_URL")

	if !useLocalModel && openAIKey == "" {
		return nil, errors.New("OpenAI API key is required")
	}

	if elevenLabsKey == "" {
		log.Println("Warning: ElevenLabs API key not provided, text-to-speech will be limited")
	}

	if useLocalModel && localModelURL == "" {
		return nil, errors.New("local model URL is required when USE_LOCAL_MODEL is true")
	}

	return &AIService{
		openAIKey:        openAIKey,
		elevenLabsKey:    elevenLabsKey,
		httpClient:       &http.Client{Timeout: 30 * time.Second},
		useLocalModelAPI: useLocalModel,
		localModelURL:    localModelURL,
	}, nil
}

// GenerateResponse generates an AI response based on character info and conversation history
func (s *AIService) GenerateResponse(character *Character, userMessage string, conversationHistory []Message) (string, error) {
	if s.useLocalModelAPI {
		return s.generateResponseLocal(character, userMessage, conversationHistory)
	}
	return s.generateResponseOpenAI(character, userMessage, conversationHistory)
}

type openAIRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (s *AIService) generateResponseOpenAI(character *Character, userMessage string, conversationHistory []Message) (string, error) {
	systemPrompt := fmt.Sprintf(
		"You are %s. %s Your personality traits are: %s. Respond in character, being concise and engaging.",
		character.Name,
		character.Description,
		character.Personality,
	)

	messages := []message{
		{Role: "system", Content: systemPrompt},
	}

	// Add conversation history
	for _, msg := range conversationHistory {
		role := "assistant"
		if msg.Sender == "user" {
			role = "user"
		}
		messages = append(messages, message{
			Role:    role,
			Content: msg.Content,
		})
	}

	// Add the current user message
	messages = append(messages, message{
		Role:    "user",
		Content: userMessage,
	})

	requestBody := openAIRequest{
		Model:    "gpt-4o",
		Messages: messages,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.openAIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making API request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status code %d: %s", resp.StatusCode, string(body))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %v", err)
	}

	if openAIResp.Error != nil {
		return "", fmt.Errorf("API error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return "", errors.New("no response generated")
	}

	return openAIResp.Choices[0].Message.Content, nil
}

func (s *AIService) generateResponseLocal(character *Character, userMessage string, conversationHistory []Message) (string, error) {
	systemPrompt := fmt.Sprintf(
		"You are %s. %s Your personality traits are: %s. Respond in character, being concise and engaging.",
		character.Name,
		character.Description,
		character.Personality,
	)

	type localModelRequest struct {
		SystemPrompt string      `json:"system_prompt"`
		History      []message   `json:"history"`
		Query        string      `json:"query"`
		Character    interface{} `json:"character"`
	}

	history := []message{}
	for _, msg := range conversationHistory {
		role := "assistant"
		if msg.Sender == "user" {
			role = "user"
		}
		history = append(history, message{
			Role:    role,
			Content: msg.Content,
		})
	}

	requestBody := localModelRequest{
		SystemPrompt: systemPrompt,
		History:      history,
		Query:        userMessage,
		Character:    character,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequest("POST", s.localModelURL+"/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making API request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("local API request failed with status code %d: %s", resp.StatusCode, string(body))
	}

	var localResp struct {
		Response string `json:"response"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(body, &localResp); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %v", err)
	}

	if localResp.Error != "" {
		return "", fmt.Errorf("local API error: %s", localResp.Error)
	}

	return localResp.Response, nil
}

// TextToSpeech converts text to audio using ElevenLabs or fallback solution
func (s *AIService) TextToSpeech(ctx context.Context, text string, voiceType string) ([]byte, error) {
	if s.elevenLabsKey == "" {
		return s.fallbackTextToSpeech(ctx, text, voiceType)
	}

	voiceID := s.getVoiceIDForType(voiceType)
	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", voiceID)

	type ttsRequest struct {
		Text          string `json:"text"`
		VoiceSettings struct {
			Stability       float64 `json:"stability"`
			SimilarityBoost float64 `json:"similarity_boost"`
		} `json:"voice_settings"`
	}

	requestBody := ttsRequest{
		Text: text,
		VoiceSettings: struct {
			Stability       float64 `json:"stability"`
			SimilarityBoost float64 `json:"similarity_boost"`
		}{
			Stability:       0.75,
			SimilarityBoost: 0.75,
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling TTS request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating TTS request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", s.elevenLabsKey)
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making TTS API request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TTS API request failed with status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading TTS response body: %v", err)
	}

	return audioData, nil
}

// getVoiceIDForType maps voice type to ElevenLabs voice ID
func (s *AIService) getVoiceIDForType(voiceType string) string {
	// Default voice IDs for different types
	// These should be configured properly in production
	switch voiceType {
	case "natural":
		return "21m00Tcm4TlvDq8ikWAM" // Rachel
	case "robotic":
		return "AZnzlk1XvdvUeBnXmlld" // Domi
	case "animated":
		return "MF3mGyEYCl7XYWbV9V6O" // Bella
	default:
		return "21m00Tcm4TlvDq8ikWAM" // Default to Rachel
	}
}

// fallbackTextToSpeech provides a basic TTS alternative when ElevenLabs is not available
func (s *AIService) fallbackTextToSpeech(_ context.Context, text string, voiceType string) ([]byte, error) {
	// Log the attempt with the parameters to show they're intentionally captured
	log.Printf("Fallback TTS requested for text '%s...' with voice type '%s'",
		text[:min(20, len(text))], voiceType)

	// In a production app, implement an alternative TTS service here
	// For now, we're returning an error but acknowledging the parameters
	return nil, errors.New("text-to-speech unavailable: ElevenLabs API key not configured")
}

// Helper function to get minimum value - used for text truncation
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SpeechToText converts audio to text
func (s *AIService) SpeechToText(ctx context.Context, audioData []byte) (string, error) {
	if s.openAIKey == "" {
		return "", errors.New("speech-to-text unavailable: OpenAI API key not configured")
	}

	// Create multipart form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file part
	part, err := writer.CreateFormFile("file", "audio.webm")
	if err != nil {
		return "", fmt.Errorf("error creating form file: %v", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return "", fmt.Errorf("error writing audio data: %v", err)
	}

	// Add model field
	if err := writer.WriteField("model", "whisper-1"); err != nil {
		return "", fmt.Errorf("error writing form field: %v", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("error closing multipart writer: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/audio/transcriptions", body)
	if err != nil {
		return "", fmt.Errorf("error creating STT request: %v", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+s.openAIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making STT API request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("STT API request failed with status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var sttResponse struct {
		Text string `json:"text"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading STT response body: %v", err)
	}

	if err := json.Unmarshal(bodyBytes, &sttResponse); err != nil {
		return "", fmt.Errorf("error unmarshaling STT response: %v", err)
	}

	return sttResponse.Text, nil
}
