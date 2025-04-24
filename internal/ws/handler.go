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
	SpeechToText(ctx context.Context, audioData []byte) (string, error)
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
			continue
		}

		go c.handleMessage(message)
	}
}

func (c *Client) handleMessage(message Message) {
	switch message.Type {
	case "chat":
		c.handleChatMessage(message)
	case "audio":
		c.handleAudioMessage(message)
	case "ping":
		// Handle ping messages
		c.sendMessage("pong", nil)
	default:
		log.Printf("Unknown message type: %s", message.Type)
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
		return
	}

	if err := json.Unmarshal(contentBytes, &chatContent); err != nil {
		log.Printf("Error unmarshaling chat content: %v", err)
		return
	}

	// Ensure this is a user message
	if chatContent.Sender != "user" {
		log.Printf("Received non-user message with sender: %s", chatContent.Sender)
		return
	}

	// Store the user message in conversation history
	userMessage := ChatMessage{
		ID:        chatContent.ID,
		Sender:    "user",
		Content:   chatContent.Content,
		Timestamp: time.Now(),
	}

	c.messagesMu.Lock()
	c.messages = append(c.messages, userMessage)
	messages := c.messages // Copy for AI generation
	c.messagesMu.Unlock()

	// Save the message to persistent storage
	if c.SessionID != "" {
		err := c.Hub.messageService.SaveMessage(c.CharID, c.SessionID, &userMessage)
		if err != nil {
			log.Printf("Error saving message to database: %v", err)
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

	// Generate AI response
	aiResponse, err := c.Hub.aiService.GenerateResponse(character, chatContent.Content, messages)
	if err != nil {
		log.Printf("Error generating AI response: %v", err)
		c.sendErrorMessage("Failed to generate response from the AI character")
		return
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
		return
	}

	if err := json.Unmarshal(contentBytes, &audioContent); err != nil {
		log.Printf("Error unmarshaling audio content: %v", err)
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

	// Store audio chunk for ML processing (if audio service is available)
	if c.Hub.audioService != nil {
		log.Printf("Audio service is available, attempting to store chunk")
		// Try to cast the interface to an AudioService
		if audioService, ok := c.Hub.audioService.(AudioService); ok {
			// Store the audio chunk with metadata
			metadata := fmt.Sprintf(`{"source":"websocket","clientId":"%s","sessionId":"%s","characterId":"%d"}`,
				c.ID, c.SessionID, c.CharID)

			ttl := 24 * time.Hour // Default TTL

			chunkId, err := audioService.StoreAudioChunk(
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
			} else {
				log.Printf("Audio chunk %s stored successfully for session %s with ID: %s", chunkID, c.SessionID, chunkId)
			}
		} else {
			log.Printf("Error: audioService could not be cast to AudioService interface")
		}
	} else {
		log.Printf("Warning: Hub.audioService is nil, audio chunk will not be stored")
	}

	// Process speech to text
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("Sending %d bytes to speech-to-text service", len(audioData))
	text, err := c.Hub.aiService.SpeechToText(ctx, audioData)
	if err != nil {
		log.Printf("Error converting speech to text: %v", err)
		c.sendErrorMessage("Failed to process speech")
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
		log.Printf("Sending WebSocket message: type=%s, jsonLength=%d, preview=%s",
			messageType, len(messageJSON), string(messageJSON[:min(len(messageJSON), 100)]))
	}

	c.Send <- messageJSON
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

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
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

			// Send first message
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

			// Send any queued messages separately instead of combining them
			n := len(c.Send)

			// Silently process any additional queued messages without logging
			for i := 0; i < n; i++ {
				extraMsg := <-c.Send

				// Send each message as a separate WebSocket frame
				if err := c.Conn.WriteMessage(websocket.TextMessage, extraMsg); err != nil {
					return
				}
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
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
	log.Printf("New WebSocket connection established for client %s, character %d, session %s",
		clientID, charIDUint, sessionID)

	// Start the pumps in separate goroutines
	go client.WritePump()
	go client.ReadPump()
}

// SetAudioService sets the audio service for the hub
func (h *Hub) SetAudioService(audioService interface{}) {
	h.audioService = audioService
}
