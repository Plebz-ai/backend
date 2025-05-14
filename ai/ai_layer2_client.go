package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"ai-agent-character-demo/backend/pkg/config"
	"ai-agent-character-demo/backend/pkg/ws"
	"errors"
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
	UserInput        string                 `json:"user_input"`
	CharacterDetails map[string]interface{} `json:"character_details"`
	SessionID        string                 `json:"session_id,omitempty"`
}
type ContextResponse struct {
	Context string                 `json:"context"`
	Rules   map[string]interface{} `json:"rules"`
}

func (c *AI_Layer2Client) GenerateContext(ctx context.Context, req ContextRequest) (ContextResponse, error) {
	log.Printf("[AI_Layer2Client] Sending LLM1 request: %+v", req)
	if req.UserInput == "" || req.CharacterDetails == nil {
		return ContextResponse{}, errors.New("missing user_input or character_details")
	}
	jsonData, err := json.Marshal(req)
	if err != nil {
		log.Printf("[AI_Layer2Client] Error marshaling LLM1 request: %v", err)
		return ContextResponse{}, err
	}
	url := c.baseURL + "/llm1/generate-context"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("[AI_Layer2Client] Error creating LLM1 request: %v", err)
		return ContextResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		log.Printf("[AI_Layer2Client] LLM1 request failed: %v", err)
		return ContextResponse{}, err
	}
	defer httpResp.Body.Close()
	var llm1Resp ContextResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&llm1Resp); err != nil {
		log.Printf("[AI_Layer2Client] Error decoding LLM1 response: %v", err)
		return ContextResponse{}, err
	}
	if llm1Resp.Context == "fallback-context" {
		return llm1Resp, errors.New("LLM1 failed to generate context")
	}
	log.Printf("[AI_Layer2Client] LLM1 response: %+v", llm1Resp)
	return llm1Resp, nil
}

// GenerateResponse calls LLM2 to generate persona response
type ResponseRequest struct {
	CharacterID uint             `json:"character_id"`
	UserID      string           `json:"user_id"`
	Context     string           `json:"context"`
	Message     string           `json:"message"`
	History     []ws.ChatMessage `json:"history"`
}

func (c *AI_Layer2Client) GenerateResponse(ctx context.Context, req ResponseRequest) (string, error) {
	log.Printf("[AI_Layer2Client] Sending LLM2 request: %+v", req)
	if req.Context == "" || req.Message == "" {
		return "", errors.New("missing context or message")
	}
	jsonData, err := json.Marshal(req)
	if err != nil {
		log.Printf("[AI_Layer2Client] Error marshaling LLM2 request: %v", err)
		return "", err
	}
	url := c.baseURL + "/llm2/generate-response"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("[AI_Layer2Client] Error creating LLM2 request: %v", err)
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		log.Printf("[AI_Layer2Client] LLM2 request failed: %v", err)
		return "", err
	}
	defer httpResp.Body.Close()
	var llm2Resp struct {
		Response string `json:"response"`
		Error    string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&llm2Resp); err != nil {
		log.Printf("[AI_Layer2Client] Error decoding LLM2 response: %v", err)
		return "", err
	}
	if llm2Resp.Error != "" {
		return llm2Resp.Response, errors.New(llm2Resp.Error)
	}
	log.Printf("[AI_Layer2Client] LLM2 response: %+v", llm2Resp)
	return llm2Resp.Response, nil
}

// Stub for TextToSpeech
func (c *AI_Layer2Client) TextToSpeech(ctx context.Context, text string, voiceType string) ([]byte, error) {
	log.Printf("[AI_Layer2Client] TextToSpeech called with text: %s, voiceType: %s", text, voiceType)
	return nil, errors.New("TextToSpeech not implemented")
}

// Stub for SpeechToText
func (c *AI_Layer2Client) SpeechToText(ctx context.Context, sessionID string, audioData []byte) (string, error) {
	log.Printf("[AI_Layer2Client] SpeechToText called with sessionID: %s, audioData length: %d", sessionID, len(audioData))
	return "", errors.New("SpeechToText not implemented")
}
