package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// AIBridge handles the communication between the backend and the AI Layer
type AIBridge struct {
	httpClient      *http.Client
	aiLayerURL      string
	wsConnections   map[string]*websocket.Conn
	connMutex       sync.Mutex
	openAIService   *AIService
	sessionContexts map[string]*SessionContext
	contextMutex    sync.Mutex
	avatarStreams   map[string]string // Maps session ID to stream ID
	avatarMutex     sync.Mutex
}

// SessionContext stores the context for a given session
type SessionContext struct {
	CharacterID uint
	UserID      string
	Messages    []Message
	LastActive  time.Time
	AvatarURL   string
}

// AILayerResponse represents a response from the AI Layer
type AILayerResponse struct {
	Transcripts string `json:"transcripts"`
	ID          string `json:"id"`
}

// CreateStreamResponse represents a response from the avatar stream creation
type CreateStreamResponse struct {
	StreamID string            `json:"stream_id"`
	Offer    map[string]string `json:"offer"`
}

// NewAIBridge creates a new instance of the AIBridge
func NewAIBridge() (*AIBridge, error) {
	aiLayerURL := os.Getenv("AI_SERVICE_URL")
	if aiLayerURL == "" {
		aiLayerURL = "http://localhost:5000"
	}

	openAIService, err := NewAIService()
	if err != nil {
		return nil, fmt.Errorf("failed to create AI service: %v", err)
	}

	return &AIBridge{
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		aiLayerURL:      aiLayerURL,
		wsConnections:   make(map[string]*websocket.Conn),
		openAIService:   openAIService,
		sessionContexts: make(map[string]*SessionContext),
		avatarStreams:   make(map[string]string),
	}, nil
}

// RegisterSession registers a new session with the bridge
func (b *AIBridge) RegisterSession(sessionID string, characterID uint, userID string, avatarURL string) {
	b.contextMutex.Lock()
	defer b.contextMutex.Unlock()

	b.sessionContexts[sessionID] = &SessionContext{
		CharacterID: characterID,
		UserID:      userID,
		Messages:    []Message{},
		LastActive:  time.Now(),
		AvatarURL:   avatarURL,
	}

	// Initiate avatar stream if avatar URL is provided
	if avatarURL != "" {
		go b.InitAvatarStream(sessionID, avatarURL)
	}
}

// InitAvatarStream initiates an avatar stream for the session
func (b *AIBridge) InitAvatarStream(sessionID string, avatarURL string) error {
	// Create a payload to send to the avatar stream API
	payload := map[string]string{
		"image_id": avatarURL,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal avatar stream payload: %v", err)
	}

	// Send request to create a new avatar stream
	url := fmt.Sprintf("%s/avatar/create_stream", b.aiLayerURL)
	resp, err := b.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create avatar stream: %v", err)
	}
	defer resp.Body.Close()

	// Parse the response
	var streamResp CreateStreamResponse
	if err := json.NewDecoder(resp.Body).Decode(&streamResp); err != nil {
		return fmt.Errorf("failed to decode avatar stream response: %v", err)
	}

	// Store the stream ID for future use
	b.avatarMutex.Lock()
	b.avatarStreams[sessionID] = streamResp.StreamID
	b.avatarMutex.Unlock()

	log.Printf("Avatar stream created for session %s with stream ID %s", sessionID, streamResp.StreamID)
	return nil
}

// ProcessAudioChunk processes an audio chunk through the AI Layer
func (b *AIBridge) ProcessAudioChunk(ctx context.Context, sessionID string, audioData []byte) (string, string, error) {
	// Check if session exists
	b.contextMutex.Lock()
	sessionContext, exists := b.sessionContexts[sessionID]
	if !exists {
		// Auto-create a session context if it doesn't exist
		log.Printf("Creating new session context for %s in AI Bridge", sessionID)
		sessionContext = &SessionContext{
			CharacterID: 1, // Default character ID
			UserID:      "user-" + sessionID,
			Messages:    []Message{},
			LastActive:  time.Now(),
		}
		b.sessionContexts[sessionID] = sessionContext
	} else {
		sessionContext.LastActive = time.Now()
	}
	b.contextMutex.Unlock()

	log.Printf("Processing audio chunk for session %s, audio size: %d bytes", sessionID, len(audioData))

	// Call TranscribeAudio which now returns both transcript and AI response
	transcript, aiResponse, err := b.TranscribeAudio(ctx, sessionID, audioData)
	if err != nil {
		return "", "", fmt.Errorf("failed to process audio: %v", err)
	}

	return transcript, aiResponse, nil
}

// Updated TranscribeAudio to use the new AI Layer endpoints
func (b *AIBridge) TranscribeAudio(ctx context.Context, sessionID string, audioData []byte) (string, string, error) {
	url := fmt.Sprintf("%s/process_audio", b.aiLayerURL)

	// Create a new request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("audio", "audio.wav")
	if err != nil {
		return "", "", fmt.Errorf("failed to create form file: %v", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return "", "", fmt.Errorf("failed to write audio data: %v", err)
	}

	// Add session and character ID as form fields if available
	b.contextMutex.Lock()
	sessionContext, exists := b.sessionContexts[sessionID]
	b.contextMutex.Unlock()

	if exists {
		writer.WriteField("session_id", sessionID)
		writer.WriteField("character_id", fmt.Sprintf("%d", sessionContext.CharacterID))
	}

	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %v", err)
	}

	// Add session and character ID as headers too for redundancy
	if exists {
		req.Header.Set("X-Session-ID", sessionID)
		req.Header.Set("X-Character-ID", fmt.Sprintf("%d", sessionContext.CharacterID))
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("transcription service returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode response: %v", err)
	}

	transcript, ok := result["transcript"].(string)
	if !ok {
		return "", "", fmt.Errorf("transcription service response missing 'transcript' field")
	}

	// Get AI response from the response field
	aiResponse, ok := result["response"].(string)
	if !ok {
		log.Printf("Warning: AI Layer response missing 'response' field, will fall back to internal AI service")
		aiResponse = ""
	} else {
		log.Printf("Received AI response from AI Layer: %s", aiResponse[:min(50, len(aiResponse))])
	}

	return transcript, aiResponse, nil
}

// ProcessTranscript processes a transcript through the backend AI service
func (b *AIBridge) ProcessTranscript(ctx context.Context, sessionID string, transcript string) (string, []byte, error) {
	b.contextMutex.Lock()
	sessionContext, exists := b.sessionContexts[sessionID]
	if !exists {
		b.contextMutex.Unlock()
		return "", nil, fmt.Errorf("session not found: %s", sessionID)
	}
	sessionContext.LastActive = time.Now()
	b.contextMutex.Unlock()

	// Get character info from the database (simplified for example)
	character := &Character{
		ID:          sessionContext.CharacterID,
		Name:        "AI Assistant",
		Description: "A helpful AI assistant",
		Personality: "Friendly, knowledgeable, and responsive",
		VoiceType:   "default",
	}

	// Add user message to context
	userMessage := Message{
		ID:        fmt.Sprintf("user-%d", time.Now().UnixNano()),
		Sender:    "user",
		Content:   transcript,
		Timestamp: time.Now(),
	}

	b.contextMutex.Lock()
	sessionContext.Messages = append(sessionContext.Messages, userMessage)
	conversationHistory := sessionContext.Messages
	b.contextMutex.Unlock()

	// Generate AI response using existing AIService
	textResponse, err := b.openAIService.GenerateResponse(character, transcript, conversationHistory)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate response: %v", err)
	}

	// Add AI response to context
	aiMessage := Message{
		ID:        fmt.Sprintf("ai-%d", time.Now().UnixNano()),
		Sender:    "character",
		Content:   textResponse,
		Timestamp: time.Now(),
	}

	b.contextMutex.Lock()
	sessionContext.Messages = append(sessionContext.Messages, aiMessage)
	b.contextMutex.Unlock()

	// Generate speech from text response
	audioData, err := b.openAIService.TextToSpeech(ctx, textResponse, character.VoiceType)
	if err != nil {
		log.Printf("Failed to generate speech: %v", err)
		// Continue without audio if TTS fails
	}

	// If there's an active avatar stream, send the audio to it
	if audioData != nil {
		err = b.SendAudioToAvatar(sessionID, audioData)
		if err != nil {
			log.Printf("Failed to send audio to avatar: %v", err)
			// Continue even if avatar animation fails
		}
	}

	return textResponse, audioData, nil
}

// Replace the AI-Layer TTS proxy with internal AIService.TextToSpeech
func (b *AIBridge) TextToSpeech(ctx context.Context, text string, voiceType string) ([]byte, error) {
	return b.openAIService.TextToSpeech(ctx, text, voiceType)
}

// SendAudioToAvatar sends the generated audio to the avatar animation service
func (b *AIBridge) SendAudioToAvatar(sessionID string, audioData []byte) error {
	b.avatarMutex.Lock()
	streamID, exists := b.avatarStreams[sessionID]
	b.avatarMutex.Unlock()

	if !exists {
		return fmt.Errorf("no active avatar stream for session: %s", sessionID)
	}

	// Create a new request to send audio to the avatar stream
	url := fmt.Sprintf("%s/avatar/talk_stream?stream_id=%s", b.aiLayerURL, streamID)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(audioData))
	if err != nil {
		return fmt.Errorf("error creating avatar audio request: %v", err)
	}

	req.Header.Set("Content-Type", "audio/wav")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending audio to avatar: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("avatar audio request failed with status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// SendResponseToFrontend sends the AI response to the frontend via WebSocket
func (b *AIBridge) SendResponseToFrontend(sessionID string, textResponse string, audioData []byte) error {
	// This would integrate with your existing WebSocket server
	// For this example, we'll just log the response
	log.Printf("Response for session %s: %s", sessionID, textResponse)
	if len(audioData) > 0 {
		log.Printf("Generated audio of %d bytes", len(audioData))
	}
	return nil
}

// getOrCreateWSConnection gets an existing WebSocket connection or creates a new one
func (b *AIBridge) getOrCreateWSConnection(sessionID string) (*websocket.Conn, error) {
	b.connMutex.Lock()
	defer b.connMutex.Unlock()

	conn, exists := b.wsConnections[sessionID]
	if exists {
		return conn, nil
	}

	// Create a new WebSocket connection
	wsURL := fmt.Sprintf("ws://%s/ws/asr", b.aiLayerURL[7:]) // Remove http:// and replace with ws://
	log.Printf("Attempting to connect to WebSocket at: %s", wsURL)

	// Test if the HTTP endpoint is accessible first
	httpURL := fmt.Sprintf("%s/healthz", b.aiLayerURL)
	httpResp, err := http.Get(httpURL)
	if err != nil {
		log.Printf("Warning: Health check failed: %v", err)
	} else {
		log.Printf("Health check status: %d", httpResp.StatusCode)
		httpResp.Body.Close()
	}

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, wsResp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		if wsResp != nil {
			log.Printf("WebSocket connection failed with status: %d", wsResp.StatusCode)
		}
		return nil, fmt.Errorf("failed to connect to AI Layer WebSocket: %v", err)
	}

	b.wsConnections[sessionID] = conn
	log.Printf("Successfully connected to WebSocket at: %s", wsURL)
	return conn, nil
}

// CleanupSession removes a session and its WebSocket connection
func (b *AIBridge) CleanupSession(sessionID string) {
	// Close WebSocket connection
	b.connMutex.Lock()
	if conn, exists := b.wsConnections[sessionID]; exists {
		conn.Close()
		delete(b.wsConnections, sessionID)
	}
	b.connMutex.Unlock()

	// Clean up avatar stream if it exists
	b.avatarMutex.Lock()
	if streamID, exists := b.avatarStreams[sessionID]; exists {
		url := fmt.Sprintf("%s/avatar/stop_stream?stream_id=%s", b.aiLayerURL, streamID)
		_, err := b.httpClient.Post(url, "application/json", nil)
		if err != nil {
			log.Printf("Error stopping avatar stream: %v", err)
		}
		delete(b.avatarStreams, sessionID)
	}
	b.avatarMutex.Unlock()

	// Remove session context
	b.contextMutex.Lock()
	delete(b.sessionContexts, sessionID)
	b.contextMutex.Unlock()
}

// Implement API endpoint handlers
func (b *AIBridge) SetupAPIHandlers(mux *http.ServeMux) {
	// Get pending audio chunks
	mux.HandleFunc("/api/ml/audio/chunks/pending", func(w http.ResponseWriter, r *http.Request) {
		// Implementation would fetch pending audio chunks from your database
		chunks := []map[string]interface{}{
			{
				"id":        "chunk1",
				"sessionId": "session1",
				"status":    "pending",
				"createdAt": time.Now().Format(time.RFC3339),
			},
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"chunks": chunks,
		})
	})

	// Process audio chunk
	mux.HandleFunc("/api/ml/audio/chunks/process", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var requestBody struct {
			ChunkID   string `json:"chunkId"`
			SessionID string `json:"sessionId"`
			AudioData string `json:"audioData"` // base64 encoded
		}

		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		audioBytes, err := base64.StdEncoding.DecodeString(requestBody.AudioData)
		if err != nil {
			http.Error(w, "Invalid audio data", http.StatusBadRequest)
			return
		}

		transcript, aiResponse, err := b.ProcessAudioChunk(r.Context(), requestBody.SessionID, audioBytes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		textResponse, audioData, err := b.ProcessTranscript(r.Context(), requestBody.SessionID, transcript)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Return response
		json.NewEncoder(w).Encode(map[string]interface{}{
			"transcript":   transcript,
			"aiResponse":   aiResponse,
			"textResponse": textResponse,
			"audioData":    base64.StdEncoding.EncodeToString(audioData),
		})
	})

	// Create avatar stream
	mux.HandleFunc("/api/ml/avatar/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var requestBody struct {
			SessionID string `json:"sessionId"`
			AvatarURL string `json:"avatarUrl"`
		}

		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		b.avatarMutex.Lock()
		defer b.avatarMutex.Unlock()

		// Check if session exists
		b.contextMutex.Lock()
		sessionContext, exists := b.sessionContexts[requestBody.SessionID]
		if !exists {
			b.contextMutex.Unlock()
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		sessionContext.AvatarURL = requestBody.AvatarURL
		b.contextMutex.Unlock()

		err := b.InitAvatarStream(requestBody.SessionID, requestBody.AvatarURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Return success response
		json.NewEncoder(w).Encode(map[string]string{
			"status": "success",
		})
	})
}

// Helper function to avoid panic with string substring
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
