package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"ai-agent-character-demo/backend/pkg/ws"
)

// AIBridge is a simple HTTP client to the external AI service
// All session management is handled in the backend; this bridge is stateless.

type AIBridge struct {
	client  *http.Client
	baseURL string
}

// NewAIBridge creates an HTTP client for the AI service
func NewAIBridge() (*AIBridge, error) {
	baseURL := os.Getenv("AI_SERVICE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:5000"
	}
	return &AIBridge{
		client:  &http.Client{Timeout: 60 * time.Second},
		baseURL: baseURL,
	}, nil
}

// ChatRequest is the payload for text-based chat
type ChatRequest struct {
	CharacterID uint   `json:"character_id"`
	Content     string `json:"content"`
}

// ChatResponse is the response from the AI chat endpoint
type ChatResponse struct {
	Text string `json:"text"`
}

// GenerateTextResponse calls the external AI service to get a response
func (b *AIBridge) GenerateTextResponse(
	character *ws.Character,
	userMessage string,
	history []ws.ChatMessage,
) (string, error) {
	reqBody := ChatRequest{
		CharacterID: character.ID,
		Content:     userMessage,
	}
	data, _ := json.Marshal(reqBody)
	resp, err := b.client.Post(fmt.Sprintf("%s/api/chat", b.baseURL), "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var respData ChatResponse
	body, _ := ioutil.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &respData); err != nil {
		return "", err
	}
	return respData.Text, nil
}

// SpeechToText sends audio bytes to the AI service and returns the transcript
func (b *AIBridge) SpeechToText(ctx context.Context, sessionID string, audioData []byte) (string, string, error) {
	reqBody := STTRequest{
		AudioData: base64.StdEncoding.EncodeToString(audioData),
	}
	data, _ := json.Marshal(reqBody)
	resp, err := b.client.Post(fmt.Sprintf("%s/api/speech-to-text", b.baseURL), "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var respData STTResponse
	body, _ := ioutil.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &respData); err != nil {
		return "", "", err
	}
	// transcript in first return; second string unused
	return respData.Transcript, "", nil
}

// TextToSpeech sends text to the AI service and returns raw audio bytes
func (b *AIBridge) TextToSpeech(ctx context.Context, text string, voiceType string) ([]byte, error) {
	reqBody := TTSRequest{Text: text, VoiceName: voiceType}
	data, _ := json.Marshal(reqBody)
	resp, err := b.client.Post(fmt.Sprintf("%s/api/text-to-speech", b.baseURL), "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var respData TTSResponse
	body, _ := ioutil.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &respData); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(respData.AudioData)
}

// RegisterSession is a no-op (session is managed in Go service)
func (b *AIBridge) RegisterSession(sessionID string, characterID uint, userID string, avatarURL string) {
}

// CleanupSession is a no-op (no persistent resources)
func (b *AIBridge) CleanupSession(sessionID string) {}
