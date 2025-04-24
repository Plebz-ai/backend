package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"ai-agent-character-demo/backend/internal/service"
	"ai-agent-character-demo/backend/internal/ws"
	"ai-agent-character-demo/backend/pkg/jwt"
)

// MessageController handles message-related API endpoints
type MessageController struct {
	messageService   *service.MessageService
	characterService *service.CharacterService
	aiService        *service.AIServiceAdapter
	jwtService       *jwt.Service
}

// NewMessageController creates a new message controller
func NewMessageController(
	messageService *service.MessageService,
	characterService *service.CharacterService,
	aiService *service.AIServiceAdapter,
	jwtService *jwt.Service,
) *MessageController {
	return &MessageController{
		messageService:   messageService,
		characterService: characterService,
		aiService:        aiService,
		jwtService:       jwtService,
	}
}

// RegisterRoutes registers the routes for the message controller
func (c *MessageController) RegisterRoutes(router *gin.Engine) {
	msgGroup := router.Group("/api/messages")
	msgGroup.Use(AuthMiddleware())
	{
		msgGroup.GET("/session/:sessionId", c.GetSessionMessages)
		msgGroup.POST("/send", c.SendMessage)
		msgGroup.GET("/:id", c.GetMessage)
	}

	// ML Engineer API endpoints (separate authentication)
	mlGroup := router.Group("/api/ml")
	mlGroup.Use(c.mlAuthMiddleware())
	{
		mlGroup.GET("/messages/session/:sessionId", c.GetSessionMessagesForML)
		mlGroup.GET("/messages/recent", c.GetRecentMessages)
	}
}

// mlAuthMiddleware ensures ML engineer authentication
func (c *MessageController) mlAuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.GetHeader("X-ML-API-Key")
		if token == "" {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "API key is required"})
			ctx.Abort()
			return
		}

		// TODO: Replace with actual API key validation logic
		if token != "ml-api-secret-key" {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// GetSessionMessages retrieves all messages for a session
func (c *MessageController) GetSessionMessages(ctx *gin.Context) {
	// User authentication has already been verified by middleware
	sessionID := ctx.Param("sessionId")
	if sessionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Session ID is required"})
		return
	}

	charIDStr := ctx.Query("characterId")
	if charIDStr == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Character ID is required"})
		return
	}

	charID, err := strconv.ParseUint(charIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid character ID"})
		return
	}

	// Get messages from service
	messages, err := c.messageService.GetSessionMessages(uint(charID), sessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error retrieving messages: %v", err)})
		return
	}

	// Format for response
	formattedMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		formattedMessages[i] = map[string]interface{}{
			"id":        msg.ExternalID,
			"sender":    msg.Sender,
			"content":   msg.Content,
			"timestamp": msg.Timestamp,
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"sessionId":   sessionID,
		"characterId": charID,
		"messages":    formattedMessages,
		"count":       len(formattedMessages),
	})
}

// SendMessage sends a new message and gets a response from the character
func (c *MessageController) SendMessage(ctx *gin.Context) {
	// User authentication has already been verified by middleware
	var request struct {
		SessionID   string `json:"sessionId" binding:"required"`
		CharacterID uint   `json:"characterId" binding:"required"`
		Content     string `json:"content" binding:"required"`
	}

	if err := ctx.BindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Create user message
	userMessage := &ws.ChatMessage{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Sender:    "user",
		Content:   request.Content,
		Timestamp: time.Now(),
	}

	// Save user message
	err := c.messageService.SaveMessage(request.CharacterID, request.SessionID, userMessage)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error saving message: %v", err)})
		return
	}

	// Get character info
	character, err := c.characterService.GetCharacter(request.CharacterID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error fetching character: %v", err)})
		return
	}

	// Convert character to WebSocket format
	wsCharacter := &ws.Character{
		ID:          character.ID,
		Name:        character.Name,
		Description: character.Description,
		Personality: character.Personality,
		VoiceType:   character.VoiceType,
		CreatedAt:   character.CreatedAt,
		UpdatedAt:   character.UpdatedAt,
	}

	// Get conversation history
	dbMessages, err := c.messageService.GetSessionMessages(request.CharacterID, request.SessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error loading previous messages: %v", err)})
		return
	}

	// Convert messages to WebSocket format
	wsMessages := make([]ws.ChatMessage, len(dbMessages))
	for i, msg := range dbMessages {
		wsMessages[i] = ws.ChatMessage{
			ID:        msg.ExternalID,
			Sender:    msg.Sender,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
	}

	// Generate AI response
	aiResponse, err := c.aiService.GenerateResponse(wsCharacter, request.Content, wsMessages)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error generating AI response: %v", err)})
		return
	}

	// Create character message
	characterMessage := &ws.ChatMessage{
		ID:        fmt.Sprintf("resp-%d", time.Now().UnixNano()),
		Sender:    "character",
		Content:   aiResponse,
		Timestamp: time.Now(),
	}

	// Save character message
	err = c.messageService.SaveMessage(request.CharacterID, request.SessionID, characterMessage)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error saving character response: %v", err)})
		return
	}

	// Return both messages
	ctx.JSON(http.StatusOK, gin.H{
		"userMessage": map[string]interface{}{
			"id":        userMessage.ID,
			"sender":    userMessage.Sender,
			"content":   userMessage.Content,
			"timestamp": userMessage.Timestamp,
		},
		"characterMessage": map[string]interface{}{
			"id":        characterMessage.ID,
			"sender":    characterMessage.Sender,
			"content":   characterMessage.Content,
			"timestamp": characterMessage.Timestamp,
		},
	})
}

// GetMessage retrieves a single message by ID
func (c *MessageController) GetMessage(ctx *gin.Context) {
	// User authentication has already been verified by middleware
	messageID := ctx.Param("id")
	if messageID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Message ID is required"})
		return
	}

	// Implement logic to get a specific message
	// This would require adding a new method to the message service
	ctx.JSON(http.StatusNotImplemented, gin.H{"error": "This endpoint is not yet implemented"})
}

// GetSessionMessagesForML retrieves all messages for a session (ML access)
func (c *MessageController) GetSessionMessagesForML(ctx *gin.Context) {
	sessionID := ctx.Param("sessionId")
	if sessionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Session ID is required"})
		return
	}

	charIDStr := ctx.Query("characterId")
	if charIDStr == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Character ID is required"})
		return
	}

	charID, err := strconv.ParseUint(charIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid character ID"})
		return
	}

	// Get messages from service
	messages, err := c.messageService.GetSessionMessages(uint(charID), sessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error retrieving messages: %v", err)})
		return
	}

	// Format for response
	formattedMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		formattedMessages[i] = map[string]interface{}{
			"id":          msg.ID,
			"externalId":  msg.ExternalID,
			"sender":      msg.Sender,
			"content":     msg.Content,
			"timestamp":   msg.Timestamp,
			"createdAt":   msg.CreatedAt,
			"characterId": msg.CharacterID,
			"sessionId":   msg.SessionID,
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"sessionId":   sessionID,
		"characterId": charID,
		"messages":    formattedMessages,
		"count":       len(formattedMessages),
	})
}

// GetRecentMessages retrieves recent messages across all sessions (ML access)
func (c *MessageController) GetRecentMessages(ctx *gin.Context) {
	limitStr := ctx.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 100
	}

	// This would require adding a new method to the message service
	ctx.JSON(http.StatusNotImplemented, gin.H{"error": "This endpoint is not yet implemented"})
}
