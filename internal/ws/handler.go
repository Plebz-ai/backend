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
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  1024,
}

// ChatMessage represents a message in the conversation
type ChatMessage struct {
	ID        string    `json:"id"`
	Sender    string    `json:"sender"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type Client struct {
	ID         string
	Conn       *websocket.Conn
	Send       chan []byte
	CharID     uint
	Hub        *Hub
	UserID     string // Optional for authentication
	messagesMu sync.Mutex
	messages   []ChatMessage // Store conversation history
	SessionID  string        // Optional session ID for persistent conversations
}

type Message struct {
	Type    string      `json:"type"`
	Content interface{} `json:"content"`
}

// CharacterService defines the interface for character operations
type CharacterService interface {
	GetCharacter(id uint) (*Character, error)
}

// AIService defines the interface for AI operations
type AIService interface {
	GenerateResponse(character *Character, userMessage string, conversationHistory []ChatMessage) (string, error)
	TextToSpeech(ctx context.Context, text string, voiceType string) ([]byte, error)
	SpeechToText(ctx context.Context, sessionID string, audioData []byte) (string, error)
}

// MessageService defines the interface for message persistence operations
type MessageService interface {
	SaveMessage(characterID uint, sessionID string, message *ChatMessage) error
	GetSessionMessages(characterID uint, sessionID string) ([]ChatMessage, error)
}

// AudioService interface for audio storage
type AudioService interface {
	StoreAudioChunk(userID string, sessionID string, charID uint, audioData []byte, format string, duration float64, sampleRate int, channels int, metadata string, ttl time.Duration) (string, error)
}

// Character represents a character in the system
type Character struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Personality string    `json:"personality"`
	VoiceType   string    `json:"voice_type"`
	AvatarURL   string    `json:"avatar_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
				close(client.Send)
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

func (c *Client) ReadPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
		log.Printf("ReadPump ended for client: %s", c.ID)
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
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		var message Message
		if err := json.Unmarshal(messageData, &message); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			c.sendErrorMessage("Invalid message format")
			continue
		}

		// Handle message in a separate goroutine to avoid blocking the read pump
		// This allows the client to continue reading messages while processing
		go c.handleMessage(message)
	}
}

func (c *Client) handleMessage(message Message) {
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

func (c *Client) handleChatMessage(message Message) {
	// Extract the user message content
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

	// Ensure this is a user message
	if chatContent.Sender != "user" {
		log.Printf("Received non-user message with sender: %s", chatContent.Sender)
		c.sendErrorMessage("Only user messages can be sent")
		return
	}

	// Validate content
	if chatContent.Content == "" {
		c.sendErrorMessage("Message content cannot be empty")
		return
	}

	// Generate message ID if not provided
	if chatContent.ID == "" {
		chatContent.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}

	// Store the user message in conversation history
	userMessage := ChatMessage{
		ID:        chatContent.ID,
		Sender:    "user",
		Content:   chatContent.Content,
		Timestamp: time.Now(),
	}

	// Acknowledge receipt of message
	c.sendMessage("ack", map[string]string{
		"messageId": userMessage.ID,
		"status":    "received",
	})

	c.messagesMu.Lock()
	c.messages = append(c.messages, userMessage)
	messages := c.messages // Copy for AI generation
	c.messagesMu.Unlock()

	// Save the message to persistent storage
	if c.SessionID != "" {
		err := c.Hub.messageService.SaveMessage(c.CharID, c.SessionID, &userMessage)
		if err != nil {
			log.Printf("Error saving message to database: %v", err)
			// Continue processing even if save fails
		}
	}

	// Notify client that character is typing
	c.sendMessage("typing", map[string]interface{}{
		"is_typing": true,
	})

	// Fetch character data
	character, err := c.Hub.characterService.GetCharacter(c.CharID)
	if err != nil {
		log.Printf("Error fetching character: %v", err)
		c.sendErrorMessage("Failed to fetch character information")
		return
	}

	// Generate AI response with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use a channel to handle the AI response with timeout
	type responseResult struct {
		response string
		err      error
	}
	resultChan := make(chan responseResult, 1)

	go func() {
		resp, err := c.Hub.aiService.GenerateResponse(character, chatContent.Content, messages)
		resultChan <- responseResult{response: resp, err: err}
	}()

	// Wait for response or timeout
	var aiResponse string
	select {
	case <-ctx.Done():
		log.Printf("AI response generation timed out for client %s", c.ID)
		c.sendErrorMessage("Response generation timed out")
		return
	case result := <-resultChan:
		if result.err != nil {
			log.Printf("Error generating AI response: %v", result.err)
			c.sendErrorMessage("Failed to generate response from the AI character")
			return
		}
		aiResponse = result.response
	}

	// Create the character's response message
	characterMessage := ChatMessage{
		ID:        fmt.Sprintf("resp-%d", time.Now().UnixNano()),
		Sender:    "character",
		Content:   aiResponse,
		Timestamp: time.Now(),
	}

	// Store the character message in conversation history
	c.messagesMu.Lock()
	c.messages = append(c.messages, characterMessage)
	c.messagesMu.Unlock()

	// Save the character's response to persistent storage
	if c.SessionID != "" {
		err := c.Hub.messageService.SaveMessage(c.CharID, c.SessionID, &characterMessage)
		if err != nil {
			log.Printf("Error saving character message to database: %v", err)
			// Continue even if save fails
		}
	}

	// Send the character's response
	c.sendMessage("chat", characterMessage)

	// Generate speech asynchronously if configured
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		audioData, err := c.Hub.aiService.TextToSpeech(ctx, aiResponse, character.VoiceType)
		if err != nil {
			log.Printf("Error generating speech: %v", err)
			return
		}

		if audioData != nil {
			// Send the audio data
			c.sendMessage("audio", map[string]interface{}{
				"data":      audioData,
				"messageId": characterMessage.ID,
			})
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
		var err error
		audioData, err = base64.StdEncoding.DecodeString(data)
		if err != nil {
			log.Printf("Error decoding base64 audio data: %v", err)
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
		"chunkId": chunkID,
		"status":  "received",
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

			var err error
			storedChunkId, err = audioService.StoreAudioChunk(
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

			if err != nil {
				log.Printf("Error storing audio chunk %s: %v", chunkID, err)
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
		text string
		err  error
	}
	resultChan := make(chan sttResult, 1)

	// IMPORTANT: Use the client's session ID for STT processing, not a random one
	log.Printf("Using session ID %s for STT processing", c.SessionID)

	go func() {
		text, err := c.Hub.aiService.SpeechToText(ctx, c.SessionID, audioData)
		resultChan <- sttResult{text: text, err: err}
	}()

	// Wait for response or timeout
	var text string
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
		text = result.text
	}

	// If text is empty, notify the user but don't proceed further
	if text == "" {
		c.sendMessage("speech_text", map[string]interface{}{
			"text":   "",
			"id":     chunkID,
			"status": "no_speech_detected",
		})
		return
	}

	log.Printf("Speech-to-text result: %s", text)

	// Create a message from the speech
	userMessage := ChatMessage{
		ID:        fmt.Sprintf("speech-%d", time.Now().UnixNano()),
		Sender:    "user",
		Content:   text,
		Timestamp: time.Now(),
	}

	// Send the transcribed text back to the client
	c.sendMessage("speech_text", map[string]interface{}{
		"text": text,
		"id":   userMessage.ID,
	})

	// Handle as a chat message
	c.messagesMu.Lock()
	c.messages = append(c.messages, userMessage)
	c.messagesMu.Unlock()

	// Now handle the message as a chat message
	c.handleChatMessage(Message{
		Type: "chat",
		Content: map[string]interface{}{
			"id":        userMessage.ID,
			"sender":    "user",
			"content":   text,
			"timestamp": time.Now().Unix(),
		},
	})
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
	_, err = c.Hub.characterService.GetCharacter(characterID)
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

	// Protect against concurrent writes to the WebSocket
	c.Hub.mu.Lock()
	defer c.Hub.mu.Unlock()

	select {
	case c.Send <- messageJSON:
		// Message sent to channel for processing
	default:
		// Send channel is full or closed
		log.Printf("Failed to send message to client %s: channel full or closed", c.ID)
		// Attempt to close connection and unregister client
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
				// The hub closed the channel.
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Printf("Error getting next writer: %v", err)
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				log.Printf("Error closing writer: %v", err)
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Error sending ping: %v", err)
				return
			}
		}
	}
}

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

	// Parse character ID
	charIDUint, err := strconv.ParseUint(charID, 10, 64)
	if err != nil {
		log.Printf("Invalid character ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid characterId: %v", err)})
		return
	}

	// Upgrade the connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Error upgrading connection: %v", err)
		return
	}

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
		messages:  []ChatMessage{},
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
		}
	}

	client.Hub.register <- client

	// Start the client's message pumps
	go client.WritePump()
	go client.ReadPump()
}
