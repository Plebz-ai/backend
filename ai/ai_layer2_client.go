package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ai-agent-character-demo/backend/pkg/config"
	"ai-agent-character-demo/backend/pkg/ws"
)

// AI_Layer2Client is a client for the AI_Layer2 microservice
// Supports LLM1 (context), LLM2 (response), STT, and TTS endpoints
// Endpoints and credentials are read from config

type AI_Layer2Client struct {
	client  *http.Client
	baseURL string
	apiKey  string // If needed for auth
}

func NewAI_Layer2Client() (*AI_Layer2Client, error) {
	cfg := config.Get()
	baseURL := cfg.Services.AIServiceURL
	if baseURL == "" {
		baseURL = "http://localhost:5000" // fallback
	}
	apiKey := "" // Add if needed: os.Getenv("AI_LAYER2_API_KEY")
	return &AI_Layer2Client{
		client:  &http.Client{Timeout: 60 * time.Second},
		baseURL: baseURL,
		apiKey:  apiKey,
	}, nil
}

// GenerateContext calls LLM1 to generate persona context (cached per session)
type ContextRequest struct {
	CharacterID uint             `json:"character_id"`
	UserID      string           `json:"user_id"`
	History     []ws.ChatMessage `json:"history"`
}
type ContextResponse struct {
	Context string `json:"context"`
}

func (c *AI_Layer2Client) GenerateContext(ctx context.Context, req ContextRequest) (string, error) {
	data, _ := json.Marshal(req)
	endpoint := fmt.Sprintf("%s/llm1/context", c.baseURL)
	request, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(data))
	request.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("context gen failed: %s", resp.Status)
	}
	var respData ContextResponse
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &respData); err != nil {
		return "", err
	}
	return respData.Context, nil
}

// GenerateResponse calls LLM2 to generate persona response
type ResponseRequest struct {
	CharacterID uint             `json:"character_id"`
	UserID      string           `json:"user_id"`
	Context     string           `json:"context"`
	Message     string           `json:"message"`
	History     []ws.ChatMessage `json:"history"`
}
type ResponseResponse struct {
	Text string `json:"text"`
}

func (c *AI_Layer2Client) GenerateResponse(ctx context.Context, req ResponseRequest) (string, error) {
	data, _ := json.Marshal(req)
	endpoint := fmt.Sprintf("%s/llm2/response", c.baseURL)
	request, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(data))
	request.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("response gen failed: %s", resp.Status)
	}
	var respData ResponseResponse
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &respData); err != nil {
		return "", err
	}
	return respData.Text, nil
}

// SpeechToText calls STT endpoint
func (c *AI_Layer2Client) SpeechToText(ctx context.Context, sessionID string, audioData []byte) (string, error) {
	reqBody := STTRequest{AudioData: base64.StdEncoding.EncodeToString(audioData)}
	data, _ := json.Marshal(reqBody)
	endpoint := fmt.Sprintf("%s/stt", c.baseURL)
	request, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(data))
	request.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("stt failed: %s", resp.Status)
	}
	var respData STTResponse
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &respData); err != nil {
		return "", err
	}
	return respData.Transcript, nil
}

// TextToSpeech calls TTS endpoint
func (c *AI_Layer2Client) TextToSpeech(ctx context.Context, text string, voiceType string) ([]byte, error) {
	reqBody := TTSRequest{Text: text, VoiceName: voiceType}
	data, _ := json.Marshal(reqBody)
	endpoint := fmt.Sprintf("%s/tts", c.baseURL)
	request, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(data))
	request.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tts failed: %s", resp.Status)
	}
	var respData TTSResponse
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &respData); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(respData.AudioData)
}
