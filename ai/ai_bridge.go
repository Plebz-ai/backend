package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	sessionContexts map[string]*SessionContext
	contextMutex    sync.Mutex
	avatarStreams   map[string]string // Maps session ID to stream ID
	avatarMutex     sync.Mutex

	// Azure LLama settings
	azureEndpoint  string
	azureAPIKey    string
	azureModelName string

	// ElevenLabs settings
	elevenLabsKey string

	// DeepGram settings
	deepgramAPIKey string
}

// SessionContext stores the context for a given session
type SessionContext struct {
	CharacterID     uint
	UserID          string
	Messages        []Message
	LastActive      time.Time
	AvatarURL       string
	CustomCharacter bool   // Flag for custom character pipeline
	SystemPrompt    string // Cached system prompt
	MemoryBuffer    string // Memory from previous conversations
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

// AIResponse represents a response from the AI Layer
type AIResponse struct {
	Text     string `json:"text"`
	Emotion  string `json:"emotion"`
	AudioURL string `json:"audio_url,omitempty"`
}

// AIError represents an error from the AI Layer
type AIError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// NewAIBridge creates a new instance of the AIBridge
func NewAIBridge() (*AIBridge, error) {
	aiLayerURL := os.Getenv("AI_SERVICE_URL")
	if aiLayerURL == "" {
		aiLayerURL = "http://localhost:5000"
	}

	// Load Azure LLama configuration
	azureEndpoint := os.Getenv("AZURE_LLAMA_ENDPOINT")
	azureAPIKey := os.Getenv("AZURE_API_KEY")
	azureModelName := os.Getenv("AZURE_MODEL_NAME")
	if azureEndpoint == "" || azureAPIKey == "" {
		return nil, fmt.Errorf("Azure LLama configuration missing: AZURE_LLAMA_ENDPOINT and AZURE_API_KEY are required")
	}
	if azureModelName == "" {
		azureModelName = "llama-2-13b-chat" // Default model name
	}

	// Load ElevenLabs configuration
	elevenLabsKey := os.Getenv("ELEVENLABS_API_KEY")
	if elevenLabsKey == "" {
		return nil, fmt.Errorf("ElevenLabs API key is required")
	}

	// Load DeepGram configuration
	deepgramAPIKey := os.Getenv("DEEPGRAM_API_KEY")
	if deepgramAPIKey == "" {
		return nil, fmt.Errorf("DeepGram API key is required for speech-to-text")
	}

	return &AIBridge{
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		aiLayerURL:      aiLayerURL,
		wsConnections:   make(map[string]*websocket.Conn),
		sessionContexts: make(map[string]*SessionContext),
		avatarStreams:   make(map[string]string),
		azureEndpoint:   azureEndpoint,
		azureAPIKey:     azureAPIKey,
		azureModelName:  azureModelName,
		elevenLabsKey:   elevenLabsKey,
		deepgramAPIKey:  deepgramAPIKey,
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

	// Transcribe audio using DeepGram
	transcript, err := b.DeepGramSpeechToText(ctx, audioData)
	if err != nil {
		return "", "", fmt.Errorf("speech-to-text failed: %v", err)
	}

	// Generate AI response
	aiResponse, err := b.ProcessTranscript(ctx, sessionID, transcript)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate AI response: %v", err)
	}

	return transcript, aiResponse, nil
}

// DeepGramSpeechToText converts speech to text using DeepGram API
func (b *AIBridge) DeepGramSpeechToText(ctx context.Context, audioData []byte) (string, error) {
	url := "https://api.deepgram.com/v1/listen?model=nova-2&language=en&smart_format=true&punctuate=true"

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(audioData))
	if err != nil {
		return "", fmt.Errorf("failed to create DeepGram request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "audio/wav")
	req.Header.Set("Authorization", "Token "+b.deepgramAPIKey)

	// Send request
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("DeepGram API request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("DeepGram API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	var result struct {
		Results struct {
			Channels []struct {
				Alternatives []struct {
					Transcript string `json:"transcript"`
				} `json:"alternatives"`
			} `json:"channels"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode DeepGram response: %v", err)
	}

	// Extract transcript from the response
	if len(result.Results.Channels) > 0 &&
		len(result.Results.Channels[0].Alternatives) > 0 {
		return result.Results.Channels[0].Alternatives[0].Transcript, nil
	}

	return "", fmt.Errorf("no transcript found in DeepGram response")
}

// ProcessTranscript processes a transcript through the LLama model
func (b *AIBridge) ProcessTranscript(ctx context.Context, sessionID string, transcript string) (string, error) {
	b.contextMutex.Lock()
	sessionContext, exists := b.sessionContexts[sessionID]
	if !exists {
		b.contextMutex.Unlock()
		return "", fmt.Errorf("session not found: %s", sessionID)
	}
	sessionContext.LastActive = time.Now()

	// Add user message to context
	userMessage := Message{
		ID:        fmt.Sprintf("user-%d", time.Now().UnixNano()),
		Sender:    "user",
		Content:   transcript,
		Timestamp: time.Now(),
	}
	sessionContext.Messages = append(sessionContext.Messages, userMessage)

	// Get character info from session context
	characterID := sessionContext.CharacterID
	isCustom := sessionContext.CustomCharacter
	b.contextMutex.Unlock()

	// Generate response based on character type
	var textResponse string
	var err error

	if isCustom {
		// Use custom character pipeline with prompt chaining
		textResponse, err = b.generateCustomCharacterResponse(ctx, sessionID, transcript)
	} else {
		// Use predefined character pipeline
		textResponse, err = b.generatePredefinedCharacterResponse(ctx, sessionID, transcript)
	}

	if err != nil {
		return "", fmt.Errorf("failed to generate response: %v", err)
	}

	// Add AI response to context
	b.contextMutex.Lock()
	aiMessage := Message{
		ID:        fmt.Sprintf("ai-%d", time.Now().UnixNano()),
		Sender:    "character",
		Content:   textResponse,
		Timestamp: time.Now(),
	}
	sessionContext.Messages = append(sessionContext.Messages, aiMessage)
	b.contextMutex.Unlock()

	return textResponse, nil
}

// generatePredefinedCharacterResponse generates a response for predefined characters
func (b *AIBridge) generatePredefinedCharacterResponse(ctx context.Context, sessionID string, userInput string) (string, error) {
	b.contextMutex.Lock()
	sessionContext := b.sessionContexts[sessionID]
	characterID := sessionContext.CharacterID
	messages := sessionContext.Messages

	// Get system prompt (cached or generate new one)
	systemPrompt := sessionContext.SystemPrompt
	b.contextMutex.Unlock()

	// If no system prompt cached, fetch character and generate one
	if systemPrompt == "" {
		character, err := b.getCharacterByID(characterID)
		if err != nil {
			return "", fmt.Errorf("failed to get character: %v", err)
		}

		systemPrompt = fmt.Sprintf(
			"You are %s. %s Your personality traits are: %s. Respond in character, being concise and engaging.",
			character.Name,
			character.Description,
			character.Personality,
		)

		// Cache the system prompt
		b.contextMutex.Lock()
		b.sessionContexts[sessionID].SystemPrompt = systemPrompt
		b.contextMutex.Unlock()
	}

	// Prepare conversation history for the LLM
	var llmMessages []map[string]string

	// Add system message
	llmMessages = append(llmMessages, map[string]string{
		"role":    "system",
		"content": systemPrompt,
	})

	// Add conversation history (limited to last 10 messages to stay within context window)
	startIdx := 0
	if len(messages) > 10 {
		startIdx = len(messages) - 10
	}

	for i := startIdx; i < len(messages)-1; i++ { // Exclude the most recent user message
		role := "assistant"
		if messages[i].Sender == "user" {
			role = "user"
		}

		llmMessages = append(llmMessages, map[string]string{
			"role":    role,
			"content": messages[i].Content,
		})
	}

	// Add the current user message
	llmMessages = append(llmMessages, map[string]string{
		"role":    "user",
		"content": userInput,
	})

	// Call Azure-hosted LLama model
	return b.callAzureLLama(llmMessages)
}

// generateCustomCharacterResponse generates a response for custom characters using prompt chaining
func (b *AIBridge) generateCustomCharacterResponse(ctx context.Context, sessionID string, userInput string) (string, error) {
	b.contextMutex.Lock()
	sessionContext := b.sessionContexts[sessionID]
	characterID := sessionContext.CharacterID
	messages := sessionContext.Messages
	systemPrompt := sessionContext.SystemPrompt
	b.contextMutex.Unlock()

	// Get character information
	character, err := b.getCharacterByID(characterID)
	if err != nil {
		return "", fmt.Errorf("failed to get character: %v", err)
	}

	// Step 1: If no enriched system prompt exists, use LLM1 to generate one
	if systemPrompt == "" {
		// Prepare prompt for LLM1
		promptTemplate := `
		You are an expert AI character designer. Your task is to create a rich system prompt for a character with the following attributes:
		
		Name: %s
		Description: %s
		Personality: %s
		
		The system prompt should be detailed and capture the essence of this character. 
		Include personality traits, speaking style, background information, and specific behavioral guidelines.
		Format your response as a system prompt that starts with "You are [character name]."
		`

		llm1Prompt := fmt.Sprintf(
			promptTemplate,
			character.Name,
			character.Description,
			character.Personality,
		)

		// Call LLM1 to generate the enriched system prompt
		llm1Messages := []map[string]string{
			{"role": "system", "content": "You're an expert prompt engineer. Create detailed character system prompts."},
			{"role": "user", "content": llm1Prompt},
		}

		enrichedPrompt, err := b.callAzureLLama(llm1Messages)
		if err != nil {
			return "", fmt.Errorf("failed to generate enriched prompt: %v", err)
		}

		// Cache the enriched system prompt
		b.contextMutex.Lock()
		b.sessionContexts[sessionID].SystemPrompt = enrichedPrompt
		systemPrompt = enrichedPrompt
		b.contextMutex.Unlock()
	}

	// Step 2: Use LLM2 with the enriched prompt to generate the response
	// Prepare conversation history for LLM2
	var llm2Messages []map[string]string

	// Add system message with enriched prompt
	llm2Messages = append(llm2Messages, map[string]string{
		"role":    "system",
		"content": systemPrompt,
	})

	// Add conversation history (limited to last 10 messages to stay within context window)
	startIdx := 0
	if len(messages) > 10 {
		startIdx = len(messages) - 10
	}

	for i := startIdx; i < len(messages)-1; i++ { // Exclude the most recent user message
		role := "assistant"
		if messages[i].Sender == "user" {
			role = "user"
		}

		llm2Messages = append(llm2Messages, map[string]string{
			"role":    role,
			"content": messages[i].Content,
		})
	}

	// Add the current user message
	llm2Messages = append(llm2Messages, map[string]string{
		"role":    "user",
		"content": userInput,
	})

	// Call LLM2 to generate the final response
	return b.callAzureLLama(llm2Messages)
}

// callAzureLLama calls the Azure-hosted LLama model
func (b *AIBridge) callAzureLLama(messages []map[string]string) (string, error) {
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=2023-07-01-preview",
		b.azureEndpoint, b.azureModelName)

	// Prepare request body
	requestBody := map[string]interface{}{
		"messages":          messages,
		"temperature":       0.7,
		"max_tokens":        800,
		"top_p":             0.95,
		"frequency_penalty": 0,
		"presence_penalty":  0,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", b.azureAPIKey)

	// Send request
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request to Azure: %v", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Azure API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode Azure API response: %v", err)
	}

	// Extract the generated text
	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response generated by Azure LLama")
}

// getCharacterByID fetches character information by ID
// In a real implementation, this would query a database
func (b *AIBridge) getCharacterByID(characterID uint) (*Character, error) {
	// This is a placeholder - in a real implementation, you'd query your database
	// For now, we return a hardcoded character based on ID
	switch characterID {
	case 1:
		return &Character{
			ID:          1,
			Name:        "AI Assistant",
			Description: "A helpful AI assistant",
			Personality: "Friendly, knowledgeable, and responsive",
			VoiceType:   "natural",
			Custom:      false,
		}, nil
	case 2:
		return &Character{
			ID:          2,
			Name:        "Adventure Guide",
			Description: "An enthusiastic guide for your adventures",
			Personality: "Energetic, encouraging, and adventure-loving",
			VoiceType:   "animated",
			Custom:      false,
		}, nil
	default:
		return &Character{
			ID:          characterID,
			Name:        fmt.Sprintf("Character %d", characterID),
			Description: "A custom character",
			Personality: "Adaptable and unique",
			VoiceType:   "natural",
			Custom:      true,
		}, nil
	}
}

// TextToSpeech converts text to speech using ElevenLabs
func (b *AIBridge) TextToSpeech(ctx context.Context, text string, voiceType string) ([]byte, error) {
	voiceID := b.getVoiceIDForType(voiceType)
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
	req.Header.Set("xi-api-key", b.elevenLabsKey)
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := b.httpClient.Do(req)
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
func (b *AIBridge) getVoiceIDForType(voiceType string) string {
	// Default voice IDs for different types
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

// SetupAPIHandlers sets up HTTP API handlers for the AI Bridge
func (b *AIBridge) SetupAPIHandlers(mux *http.ServeMux) {
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

		// Generate TTS response
		voiceType := "natural" // Default voice type
		b.contextMutex.Lock()
		if session, exists := b.sessionContexts[requestBody.SessionID]; exists {
			character, _ := b.getCharacterByID(session.CharacterID)
			if character != nil {
				voiceType = character.VoiceType
			}
		}
		b.contextMutex.Unlock()

		audioData, err := b.TextToSpeech(r.Context(), aiResponse, voiceType)
		if err != nil {
			log.Printf("TTS failed: %v", err)
			// Continue without audio if TTS fails
		}

		// Return response
		json.NewEncoder(w).Encode(map[string]interface{}{
			"transcript": transcript,
			"aiResponse": aiResponse,
			"audioData":  base64.StdEncoding.EncodeToString(audioData),
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

	// Handler for creating a custom character
	mux.HandleFunc("/api/ml/characters/custom", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var requestBody struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Personality string `json:"personality"`
			VoiceType   string `json:"voiceType"`
		}

		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// In a real implementation, you would save this to a database
		// and return the created character ID

		// Return a mock response for now
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          999, // Mock ID
			"name":        requestBody.Name,
			"description": requestBody.Description,
			"personality": requestBody.Personality,
			"voiceType":   requestBody.VoiceType,
			"custom":      true,
			"createdAt":   time.Now(),
		})
	})

	// Health check endpoint
	mux.HandleFunc("/api/ml/healthz", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": "1.0.0",
		})
	})
}

// GenerateTextResponse generates a text response for a character based on a user message and conversation history
func (b *AIBridge) GenerateTextResponse(character interface{}, userMessage string, history interface{}) (string, error) {
	// Create a session ID for this one-time request
	sessionID := fmt.Sprintf("temp-%d", time.Now().UnixNano())

	// Extract character info
	char, ok := character.(*Character)
	if !ok {
		return "", fmt.Errorf("invalid character type")
	}

	// Create temporary session context
	b.contextMutex.Lock()
	b.sessionContexts[sessionID] = &SessionContext{
		CharacterID:     char.ID,
		UserID:          "temp-user",
		Messages:        []Message{},
		LastActive:      time.Now(),
		CustomCharacter: char.Custom,
	}

	// Add history messages if available
	if history != nil {
		historyMessages := []Message{}

		// Convert history to internal format based on type
		switch h := history.(type) {
		case []Message:
			historyMessages = h
		case []map[string]interface{}:
			for _, msg := range h {
				historyMessages = append(historyMessages, Message{
					ID:        fmt.Sprintf("%v", msg["ID"]),
					Sender:    fmt.Sprintf("%v", msg["Sender"]),
					Content:   fmt.Sprintf("%v", msg["Content"]),
					Timestamp: time.Now(), // Use current time as fallback
				})
			}
		}

		b.sessionContexts[sessionID].Messages = historyMessages
	}
	b.contextMutex.Unlock()

	// Clean up the session when done
	defer func() {
		b.contextMutex.Lock()
		delete(b.sessionContexts, sessionID)
		b.contextMutex.Unlock()
	}()

	// Process the message through appropriate pipeline
	response, err := b.ProcessTranscript(context.Background(), sessionID, userMessage)
	return response, err
}

// Helper function to avoid panic with string substring
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
