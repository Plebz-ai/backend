package ws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	ws "ai-agent-character-demo/backend/pkg/ws"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512 * 1024 // 512KB
)

var upgrader = websocket.Upgrader{
	// Allow all origins for local/demo
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  1024,
}

type Client struct {
	ID         string
	Conn       *websocket.Conn
	Send       chan []byte
	CharID     uint
	Hub        *Hub
	UserID     string // Optional for authentication
	messagesMu sync.Mutex
	messages   []ws.ChatMessage // Store conversation history
	SessionID  string           // Optional session ID for persistent conversations
	closed     bool             // Add closed flag
	mu         sync.Mutex       // Add mutex for closed flag
}

type Message struct {
	Type    string      `json:"type"`
	Content interface{} `json:"content"`
}

// CharacterService defines the interface for character operations
type CharacterService interface {
	GetCharacter(id uint, userID string) (*ws.Character, error)
}

// AIService defines the interface for AI operations
type AIService interface {
	GenerateResponse(character *ws.Character, userMessage string, conversationHistory []ws.ChatMessage) (string, error)
	TextToSpeech(ctx context.Context, text string, voiceType string) ([]byte, error)
	SpeechToText(ctx context.Context, sessionID string, audioData []byte) (string, string, error)
}

// MessageService defines the interface for message persistence operations
type MessageService interface {
	SaveMessage(characterID uint, sessionID string, message *ws.ChatMessage) error
	GetSessionMessages(characterID uint, sessionID string) ([]ws.ChatMessage, error)
}

// AudioService interface for audio storage
type AudioService interface {
	StoreAudioChunk(userID string, sessionID string, charID uint, audioData []byte, format string, duration float64, sampleRate int, channels int, metadata string, ttl time.Duration) (string, error)
}

type Hub struct {
	clients          map[*Client]bool
	broadcast        chan []byte
	register         chan *Client
	unregister       chan *Client
	characterService CharacterService
	aiService        AIService
	messageService   MessageService
	audioService     interface{}
	mu               sync.Mutex
	undelivered      map[string][]Message // Buffer for undelivered messages
}

func NewHub(characterService CharacterService, aiService AIService, messageService MessageService) *Hub {
	return &Hub{
		clients:          make(map[*Client]bool),
		broadcast:        make(chan []byte),
		register:         make(chan *Client),
		unregister:       make(chan *Client),
		characterService: characterService,
		aiService:        aiService,
		messageService:   messageService,
		audioService:     nil, // Will be set later if available
		undelivered:      make(map[string][]Message),
	}
}

// SetAudioService sets the audio service after hub initialization
func (h *Hub) SetAudioService(audioService interface{}) {
	h.audioService = audioService
}

// GetActiveConnections returns the number of active WebSocket connections
func (h *Hub) GetActiveConnections() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	clientIDs := make([]string, 0, len(h.clients))
	for client := range h.clients {
		clientIDs = append(clientIDs, client.ID)
	}

	return clientIDs
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("Client registered: %s for character %d", client.ID, client.CharID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.mu.Lock()
				if !client.closed {
					client.closed = true
					close(client.Send)
				}
				client.mu.Unlock()
				log.Printf("Client unregistered: %s", client.ID)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
					log.Printf("Client removed due to blocked channel: %s", client.ID)
				}
			}
			h.mu.Unlock()
		}
	}
}

// Improve error logging and cleanup in ReadPump
func (c *Client) ReadPump() {
	defer func() {
		log.Printf("[ReadPump] Unregistering client: %s, session: %s", c.ID, c.SessionID)
		c.Hub.unregister <- c
		c.mu.Lock()
		if !c.closed {
			c.closed = true
			close(c.Send)
		}
		c.mu.Unlock()
		c.Conn.Close()
		log.Printf("ReadPump ended for client: %s, session: %s", c.ID, c.SessionID)
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, messageData, err := c.Conn.ReadMessage()
		if err != nil {
			log.Printf("[ReadPump] Exiting for client %s, session %s: %v", c.ID, c.SessionID, err)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[ReadPump] Unexpected WebSocket close error for client %s, session %s: %v", c.ID, c.SessionID, err)
			}
			break
		}

		// Parse the message data into a Message struct
		var msg Message
		if err := json.Unmarshal(messageData, &msg); err != nil {
			log.Printf("[ReadPump] Error unmarshaling message: %v", err)
			c.sendErrorMessage("Invalid message format")
			continue
		}

		// Handle message in a separate goroutine
		go c.handleMessage(msg)
	}
	log.Printf("[ReadPump] Loop exited for client %s, session %s", c.ID, c.SessionID)
}

func (c *Client) handleMessage(message Message) {
	log.Printf("[DEBUG] handleMessage called: type=%s, content=%+v", message.Type, message.Content)
	// Add recovery to prevent crashes from panics in message handlers
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in handleMessage: %v", r)
			c.sendErrorMessage("Internal server error occurred")
		}
	}()

	switch message.Type {
	case "chat":
		c.handleChatMessage(message)
	case "audio":
		c.handleAudioMessage(message)
	case "ping":
		// Handle ping messages
		c.sendMessage("pong", nil)
	case "start_stream":
		c.handleStartStreamMessage(message)
	case "stream_config":
		c.handleStreamConfigMessage(message)
	default:
		log.Printf("Unknown message type: %s", message.Type)
		c.sendErrorMessage(fmt.Sprintf("Unknown message type: %s", message.Type))
	}
}

// Add detailed logging for WebSocket message handling
func (c *Client) handleChatMessage(message Message) {
	log.Printf("Received chat message: %+v", message)

	var chatContent struct {
		ID        string `json:"id"`
		Sender    string `json:"sender"`
		Content   string `json:"content"`
		Timestamp int64  `json:"timestamp"`
	}

	contentBytes, err := json.Marshal(message.Content)
	if err != nil {
		log.Printf("Error marshaling content: %v", err)
		c.sendErrorMessage("Error processing message content")
		return
	}

	if err := json.Unmarshal(contentBytes, &chatContent); err != nil {
		log.Printf("Error unmarshaling chat content: %v", err)
		c.sendErrorMessage("Invalid chat message format")
		return
	}

	log.Printf("Chat content unmarshaled successfully: %+v", chatContent)

	if chatContent.Sender != "user" {
		log.Printf("Invalid sender: %s", chatContent.Sender)
		c.sendErrorMessage("Only user messages can be sent")
		return
	}

	if chatContent.Content == "" {
		log.Printf("Empty message content")
		c.sendErrorMessage("Message content cannot be empty")
		return
	}

	if chatContent.ID == "" {
		chatContent.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}

	userMessage := ws.ChatMessage{
		ID:        chatContent.ID,
		Sender:    "user",
		Content:   chatContent.Content,
		Timestamp: time.Now(),
	}

	log.Printf("Storing user message: %+v", userMessage)

	c.messagesMu.Lock()
	c.messages = append(c.messages, userMessage)
	messages := c.messages
	c.messagesMu.Unlock()

	if c.SessionID != "" {
		err := c.Hub.messageService.SaveMessage(c.CharID, c.SessionID, &userMessage)
		if err != nil {
			log.Printf("Error saving message to database: %v", err)
		}
	}

	c.sendMessage("ack", map[string]string{
		"messageId": userMessage.ID,
		"status":    "received",
	})

	log.Printf("Acknowledged user message: %s", userMessage.ID)

	go func() {
		// Get character first
		character, err := c.Hub.characterService.GetCharacter(c.CharID, c.UserID)
		if err != nil {
			log.Printf("Error fetching character: %v", err)
			c.sendErrorMessage("Failed to fetch character information")
			return
		}

		aiResponse, err := c.Hub.aiService.GenerateResponse(character, chatContent.Content, messages)
		if err != nil {
			log.Printf("Error generating AI response: %v", err)
			c.sendErrorMessage("Failed to generate response from the AI character")
			return
		}

		log.Printf("Generated AI response: %s", aiResponse)

		characterMessage := ws.ChatMessage{
			ID:        fmt.Sprintf("resp-%d", time.Now().UnixNano()),
			Sender:    "character",
			Content:   aiResponse,
			Timestamp: time.Now(),
		}

		c.messagesMu.Lock()
		c.messages = append(c.messages, characterMessage)
		c.messagesMu.Unlock()

		sent := c.sendMessageWithAck("chat", characterMessage)
		if !sent {
			// Buffer the message for later delivery
			bufferKey := c.SessionID + ":" + c.ID
			c.Hub.mu.Lock()
			c.Hub.undelivered[bufferKey] = append(c.Hub.undelivered[bufferKey], Message{Type: "chat", Content: characterMessage})
			c.Hub.mu.Unlock()
			log.Printf("[BUFFER] Buffered undelivered message for %s", bufferKey)
		}
	}()
}

func (c *Client) handleAudioMessage(message Message) {
	// Extract the audio content
	log.Printf("Handling audio message from client %s", c.ID)

	var audioContent struct {
		Data interface{} `json:"data"`
	}

	contentBytes, err := json.Marshal(message.Content)
	if err != nil {
		log.Printf("Error marshaling audio content: %v", err)
		c.sendErrorMessage("Error processing audio content")
		return
	}

	if err := json.Unmarshal(contentBytes, &audioContent); err != nil {
		log.Printf("Error unmarshaling audio content: %v", err)
		c.sendErrorMessage("Invalid audio message format")
		return
	}

	// Generate a unique ID for this audio chunk - used for logging
	chunkID := fmt.Sprintf("audio-%d", time.Now().UnixNano())

	// Log data type information
	log.Printf("Audio data type: %T", audioContent.Data)

	// Notify client that speech is being processed
	c.sendMessage("processing", map[string]interface{}{
		"status": "processing_speech",
	})

	// Process audio data based on its type
	var audioData []byte

	switch data := audioContent.Data.(type) {
	case string:
		// Handle base64 encoded string
		log.Printf("Received base64 encoded string, length: %d", len(data))
		var decodeErr error
		audioData, decodeErr = base64.StdEncoding.DecodeString(data)
		if decodeErr != nil {
			log.Printf("Error decoding base64 audio data: %v", decodeErr)
			c.sendErrorMessage("Failed to process audio data")
			return
		}
		log.Printf("Successfully decoded base64 data to %d bytes", len(audioData))
	case []interface{}:
		// Handle array of numbers (legacy format)
		log.Printf("Received array of numbers, length: %d", len(data))
		audioData = make([]byte, len(data))
		for i, v := range data {
			if num, ok := v.(float64); ok {
				audioData[i] = byte(num)
			}
		}
		log.Printf("Successfully converted array to %d bytes", len(audioData))
	default:
		log.Printf("Unexpected audio data format: %T", audioContent.Data)
		c.sendErrorMessage("Unsupported audio data format")
		return
	}

	// Acknowledge receipt of audio chunk
	c.sendMessage("ack", map[string]string{
		"messageId": chunkID,
		"status":    "received",
	})

	// Store audio chunk for ML processing (if audio service is available)
	var storedChunkId string
	if c.Hub.audioService != nil {
		log.Printf("Audio service is available, attempting to store chunk")
		// Try to cast the interface to an AudioService
		if audioService, ok := c.Hub.audioService.(AudioService); ok {
			// Store the audio chunk with metadata
			metadata := fmt.Sprintf(`{"source":"websocket","clientId":"%s","sessionId":"%s","characterId":"%d"}`,
				c.ID, c.SessionID, c.CharID)

			ttl := 24 * time.Hour // Default TTL

			var storeErr error
			storedChunkId, storeErr = audioService.StoreAudioChunk(
				c.UserID,
				c.SessionID,
				c.CharID,
				audioData,
				"webm",
				0.0,   // Duration unknown
				48000, // Default sample rate
				1,     // Mono audio
				metadata,
				ttl,
			)

			if storeErr != nil {
				log.Printf("Error storing audio chunk %s: %v", chunkID, storeErr)
				// Continue even if storage fails
			} else {
				log.Printf("Audio chunk %s stored successfully for session %s with ID: %s", chunkID, c.SessionID, storedChunkId)
			}
		} else {
			log.Printf("Error: audioService could not be cast to AudioService interface")
		}
	} else {
		log.Printf("Warning: Hub.audioService is nil, audio chunk will not be stored")
	}

	// Process speech to text with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use a channel to handle the STT response with timeout
	type sttResult struct {
		transcript string
		aiResponse string
		err        error
	}
	resultChan := make(chan sttResult, 1)

	// IMPORTANT: Use the client's session ID for STT processing, not a random one
	log.Printf("Using session ID %s for STT processing", c.SessionID)

	go func() {
		transcript, aiResponse, sttErr := c.Hub.aiService.SpeechToText(ctx, c.SessionID, audioData)
		resultChan <- sttResult{transcript: transcript, aiResponse: aiResponse, err: sttErr}
	}()

	// Wait for response or timeout
	var transcript string
	var aiResponse string
	select {
	case <-ctx.Done():
		log.Printf("Speech-to-text processing timed out for client %s", c.ID)
		c.sendErrorMessage("Speech processing timed out")
		return
	case result := <-resultChan:
		if result.err != nil {
			log.Printf("Error converting speech to text: %v", result.err)
			c.sendErrorMessage("Failed to process speech")
			return
		}
		transcript = result.transcript
		aiResponse = result.aiResponse
		log.Printf("Got transcript: '%s' and AI response: '%s'",
			transcript,
			aiResponse[:min(50, len(aiResponse))])
	}

	// If transcript is empty, notify the user but don't proceed further
	if transcript == "" {
		c.sendMessage("speech_text", map[string]interface{}{
			"text":   "",
			"id":     chunkID,
			"status": "no_speech_detected",
		})
		return
	}

	// Create a message from the speech
	userMessage := ws.ChatMessage{
		ID:        fmt.Sprintf("speech-%d", time.Now().UnixNano()),
		Sender:    "user",
		Content:   transcript,
		Timestamp: time.Now(),
	}

	// Send the transcribed text back to the client
	c.sendMessage("speech_text", map[string]interface{}{
		"text": transcript,
		"id:":  userMessage.ID,
	})

	// Store the user message in conversation history
	c.messagesMu.Lock()
	c.messages = append(c.messages, userMessage)
	c.messagesMu.Unlock()

	// Save the message to persistent storage
	if c.SessionID != "" {
		saveErr := c.Hub.messageService.SaveMessage(c.CharID, c.SessionID, &userMessage)
		if saveErr != nil {
			log.Printf("Error saving message to database: %v", saveErr)
			// Continue processing even if save fails
		}
	}

	// Acknowledge receipt of message
	c.sendMessage("ack", map[string]string{
		"messageId": userMessage.ID,
		"status":    "received",
	})

	// Notify client that character is typing
	c.sendMessage("typing", map[string]interface{}{
		"is_typing": true,
	})

	// Here's the key change: If we have an AI response from LLM_Layer, use that
	// Otherwise fallback to generating a response with the internal AI service
	var characterResponse string
	var audioResponse []byte

	// Check if we have a valid AI response from our LLM_Layer
	if aiResponse != "" {
		log.Printf("Using AI response from LLM_Layer for client %s", c.ID)
		characterResponse = aiResponse

		// Generate TTS for the AI response
		audioCtx, audioCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer audioCancel()

		var ttsErr error
		audioResponse, ttsErr = c.Hub.aiService.TextToSpeech(audioCtx, characterResponse, "default")
		if ttsErr != nil {
			log.Printf("Error generating speech for LLM response: %v", ttsErr)
			// Continue without audio
		}
	} else {
		// Fetch character data
		character, charErr := c.Hub.characterService.GetCharacter(c.CharID, c.UserID)
		if charErr != nil {
			log.Printf("Error fetching character: %v", charErr)
			c.sendErrorMessage("Failed to fetch character information")
			return
		}

		// Generate AI response with timeout
		aiCtx, aiCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer aiCancel()

		// Use a channel to handle the AI response with timeout
		type responseResult struct {
			response string
			err      error
		}
		aiResultChan := make(chan responseResult, 1)

		go func() {
			resp, respErr := c.Hub.aiService.GenerateResponse(character, transcript, c.messages)
			aiResultChan <- responseResult{response: resp, err: respErr}
		}()

		// Wait for response or timeout
		select {
		case <-aiCtx.Done():
			log.Printf("AI response generation timed out for client %s", c.ID)
			c.sendErrorMessage("Response generation timed out")
			return
		case result := <-aiResultChan:
			if result.err != nil {
				log.Printf("Error generating AI response: %v", result.err)
				c.sendErrorMessage("Failed to generate response from the AI character")
				return
			}
			characterResponse = result.response
		}

		// Generate speech asynchronously if configured
		audioCtx, audioCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer audioCancel()

		var ttsErr error
		audioResponse, ttsErr = c.Hub.aiService.TextToSpeech(audioCtx, characterResponse, character.VoiceType)
		if ttsErr != nil {
			log.Printf("Error generating speech: %v", ttsErr)
			// Continue without audio
		}
	}

	// Create the character's response message
	characterMessage := ws.ChatMessage{
		ID:        fmt.Sprintf("resp-%d", time.Now().UnixNano()),
		Sender:    "character",
		Content:   characterResponse,
		Timestamp: time.Now(),
	}

	// Store the character message in conversation history
	c.messagesMu.Lock()
	c.messages = append(c.messages, characterMessage)
	c.messagesMu.Unlock()

	// Save the character's response to persistent storage
	if c.SessionID != "" {
		saveErr := c.Hub.messageService.SaveMessage(c.CharID, c.SessionID, &characterMessage)
		if saveErr != nil {
			log.Printf("Error saving character message to database: %v", saveErr)
			// Continue even if save fails
		}
	}

	// Send the character's response
	c.sendMessage("chat", characterMessage)

	// Send audio if we have it
	if audioResponse != nil {
		// Send the audio data
		c.sendMessage("audio", map[string]interface{}{
			"data":      audioResponse,
			"messageId": characterMessage.ID,
		})
	}
}

func (c *Client) handleStartStreamMessage(message Message) {
	log.Printf("Handling start_stream message from client %s", c.ID)

	// Extract the stream content
	var streamContent struct {
		CharacterID  interface{} `json:"character_id"`
		VideoEnabled bool        `json:"video_enabled"`
		AudioEnabled bool        `json:"audio_enabled"`
	}

	contentBytes, err := json.Marshal(message.Content)
	if err != nil {
		log.Printf("Error marshaling stream content: %v", err)
		c.sendErrorMessage("Error processing stream content")
		return
	}

	if err := json.Unmarshal(contentBytes, &streamContent); err != nil {
		log.Printf("Error unmarshaling stream content: %v", err)
		c.sendErrorMessage("Invalid stream message format")
		return
	}

	// Convert character ID to uint if it's a string or number
	var characterID uint
	switch id := streamContent.CharacterID.(type) {
	case string:
		idUint, err := strconv.ParseUint(id, 10, 32)
		if err != nil {
			log.Printf("Error parsing character ID as string: %v", err)
			c.sendErrorMessage("Invalid character ID format")
			return
		}
		characterID = uint(idUint)
	case float64:
		characterID = uint(id)
	case int:
		characterID = uint(id)
	default:
		// If somehow it's null or something else
		characterID = c.CharID
	}

	// Validate character ID
	if characterID == 0 && c.CharID != 0 {
		characterID = c.CharID
	} else if characterID == 0 {
		c.sendErrorMessage("Character ID is required")
		return
	}

	// Get character info
	_, err = c.Hub.characterService.GetCharacter(characterID, c.UserID)
	if err != nil {
		log.Printf("Error getting character for streaming: %v", err)
		c.sendErrorMessage("Could not find character")
		return
	}

	// Acknowledge the stream request
	c.sendMessage("call_state", map[string]interface{}{
		"state":        "connecting",
		"character_id": characterID,
	})

	// In a real implementation, you would initialize video streaming here
	// For now, just acknowledge the request
	log.Printf("Starting stream for client %s with character %d, video: %v, audio: %v",
		c.ID, characterID, streamContent.VideoEnabled, streamContent.AudioEnabled)

	// Notify client that the stream is ready
	c.sendMessage("call_state", map[string]interface{}{
		"state":        "connected",
		"character_id": characterID,
	})
}

// handleStreamConfigMessage handles stream configuration updates
func (c *Client) handleStreamConfigMessage(message Message) {
	log.Printf("Handling stream_config message from client %s", c.ID)

	var configContent struct {
		VideoEnabled *bool `json:"video_enabled,omitempty"`
		AudioEnabled *bool `json:"audio_enabled,omitempty"`
	}

	contentBytes, err := json.Marshal(message.Content)
	if err != nil {
		log.Printf("Error marshaling stream config content: %v", err)
		c.sendErrorMessage("Error processing stream config")
		return
	}

	if err := json.Unmarshal(contentBytes, &configContent); err != nil {
		log.Printf("Error unmarshaling stream config content: %v", err)
		c.sendErrorMessage("Invalid stream config format")
		return
	}

	// Log the configuration update
	if configContent.VideoEnabled != nil {
		log.Printf("Client %s updated video enabled: %v", c.ID, *configContent.VideoEnabled)
	}

	if configContent.AudioEnabled != nil {
		log.Printf("Client %s updated audio enabled: %v", c.ID, *configContent.AudioEnabled)
	}

	// Acknowledge the configuration update
	c.sendMessage("stream_config_ack", map[string]interface{}{
		"status": "ok",
	})
}

func (c *Client) sendMessage(messageType string, content interface{}) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		log.Printf("[WARN] Tried to send on closed channel for client %s", c.ID)
		return
	}
	c.mu.Unlock()
	message := Message{
		Type:    messageType,
		Content: content,
	}

	messageJSON, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	// Only log non-ping/pong messages to reduce log noise
	if messageType != "ping" && messageType != "pong" {
		logPreview := string(messageJSON)
		if len(logPreview) > 100 {
			logPreview = logPreview[:100] + "..."
		}
		log.Printf("Sending WebSocket message: type=%s, jsonLength=%d, preview=%s",
			messageType, len(messageJSON), logPreview)
	}

	c.Hub.mu.Lock()
	defer c.Hub.mu.Unlock()

	select {
	case c.Send <- messageJSON:
		// Message sent to channel for processing
	default:
		// Send channel is full or closed
		log.Printf("Failed to send message to client %s: channel full or closed", c.ID)
		c.mu.Lock()
		if !c.closed {
			c.closed = true
			close(c.Send)
		}
		c.mu.Unlock()
		c.Hub.unregister <- c
	}
}

// Helper function to avoid panic
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Client) sendErrorMessage(errorText string) {
	c.sendMessage("error", map[string]string{
		"message": errorText,
	})
}

// WritePump pumps messages from the hub to the websocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
		log.Printf("WritePump ended for client: %s", c.ID)
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				log.Printf("[WritePump] Send channel closed for client %s", c.ID)
				// The hub closed the channel.
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			// Send each message as a separate WebSocket frame
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("[WritePump] Error writing message: %v", err)
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("[WritePump] Error sending ping: %v", err)
				return
			}
		}
	}
}

// Improve logging for WebSocket connections
func ServeWs(hub *Hub, c *gin.Context) {
	// Validate input parameters
	charID := c.Query("characterId")
	if charID == "" {
		log.Println("No character ID provided")
		c.JSON(http.StatusBadRequest, gin.H{"error": "characterId is required"})
		return
	}

	clientID := c.Query("clientId")
	if clientID == "" {
		log.Println("No client ID provided")
		c.JSON(http.StatusBadRequest, gin.H{"error": "clientId is required"})
		return
	}

	// Optional session ID for persistent conversations
	sessionID := c.Query("sessionId")
	if sessionID == "" {
		// If no session ID is provided, create one
		sessionID = fmt.Sprintf("session-%s-%s-%d", charID, clientID, time.Now().Unix())
	}

	// Try to parse character ID as uint, fallback to 1 (Elon Musk) if not numeric
	var charIDUint uint64
	var err error
	charIDUint, err = strconv.ParseUint(charID, 10, 64)
	if err != nil {
		log.Printf("Non-numeric character ID '%s', using fallback character 1 (Elon Musk)", charID)
		charIDUint = 1
	}

	// Upgrade the connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Error upgrading connection: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("WebSocket upgrade failed: %v", err)})
		return
	}

	log.Printf("WebSocket connection established: clientID=%s, characterID=%d", clientID, charIDUint)

	// Enable compression
	conn.EnableWriteCompression(true)

	// Create and register the client
	client := &Client{
		ID:        clientID,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		CharID:    uint(charIDUint),
		Hub:       hub,
		SessionID: sessionID,
		messages:  []ws.ChatMessage{},
	}

	// Load previous messages for this session if it exists
	if sessionID != "" {
		previousMessages, err := hub.messageService.GetSessionMessages(client.CharID, sessionID)
		if err != nil {
			log.Printf("Error loading previous messages: %v", err)
		} else if len(previousMessages) > 0 {
			client.messages = previousMessages
			log.Printf("Loaded %d previous messages for session %s", len(previousMessages), sessionID)

			// Send the chat history to the client
			client.sendMessage("chat_history", map[string]interface{}{
				"messages": previousMessages,
			})
			log.Printf("[DEBUG] Sent chat history to client %s, session %s. About to start ReadPump/WritePump.", client.ID, client.SessionID)
		}
	}

	client.Hub.register <- client

	// After registering client and before starting ReadPump/WritePump:
	connectedMsg := map[string]interface{}{"type": "connected"}
	msgBytes, _ := json.Marshal(connectedMsg)
	select {
	case client.Send <- msgBytes:
		log.Printf("[DEBUG] Sent 'connected' message to client %s", client.ID)
	default:
		log.Printf("[ERROR] Could not send 'connected' message to client %s", client.ID)
	}

	// Send any buffered undelivered messages for this session/client
	bufferKey := client.SessionID + ":" + client.ID
	hub.mu.Lock()
	if msgs, ok := hub.undelivered[bufferKey]; ok {
		for _, msg := range msgs {
			client.sendMessage(msg.Type, msg.Content)
		}
		delete(hub.undelivered, bufferKey)
	}
	hub.mu.Unlock()

	// Start the client's message pumps with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC in WritePump for client %s: %v", client.ID, r)
			}
		}()
		client.WritePump()
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC in ReadPump for client %s: %v", client.ID, r)
			}
		}()
		client.ReadPump()
	}()
}

// sendMessageWithAck tries to send a message and returns true if successful, false if channel is closed
func (c *Client) sendMessageWithAck(messageType string, content interface{}) bool {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		log.Printf("[WARN] Tried to send on closed channel for client %s", c.ID)
		return false
	}
	c.mu.Unlock()
	message := Message{
		Type:    messageType,
		Content: content,
	}
	messageJSON, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return false
	}
	select {
	case c.Send <- messageJSON:
		return true
	default:
		log.Printf("Failed to send message to client %s: channel full or closed", c.ID)
		c.mu.Lock()
		if !c.closed {
			c.closed = true
			close(c.Send)
		}
		c.mu.Unlock()
		c.Hub.unregister <- c
		return false
	}
}
