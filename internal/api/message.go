package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"ai-agent-character-demo/backend/internal/service"
	"ai-agent-character-demo/backend/pkg/jwt"
	ws "ai-agent-character-demo/backend/pkg/ws"
)

// MessageController handles message-related API endpoints
type MessageController struct {
	messageService   *service.MessageService
	characterService *service.CharacterService
	aiService        *service.AIServiceAdapter
	jwtService       *jwt.Service
	mlApiKey         string
}

// NewMessageController creates a new message controller
func NewMessageController(
	messageService *service.MessageService,
	characterService *service.CharacterService,
	aiService *service.AIServiceAdapter,
	jwtService *jwt.Service,
) *MessageController {
	// Get ML API key from environment or use a default for development
	mlApiKey := os.Getenv("ML_API_KEY")
	if mlApiKey == "" {
		mlApiKey = "ml-api-secret-key" // Default for development only
	}

	return &MessageController{
		messageService:   messageService,
		characterService: characterService,
		aiService:        aiService,
		jwtService:       jwtService,
		mlApiKey:         mlApiKey,
	}
}

// RegisterRoutes registers the routes for the message controller
func (c *MessageController) RegisterRoutes(router *gin.Engine) {
	msgGroup := router.Group("/api/messages")
	msgGroup.Use(c.authMiddleware()) // Use the controller's authMiddleware instead of undefined AuthMiddleware
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

// RegisterRoutesV1 registers versioned routes for the message controller
func (c *MessageController) RegisterRoutesV1(router *gin.RouterGroup) {
	// Regular user message endpoints
	messageGroup := router.Group("/messages")
	messageGroup.Use(c.authMiddleware())
	{
		messageGroup.GET("", c.validateListMessagesRequest(), c.GetMessages)
		messageGroup.GET("/session/:sessionId", c.validateSessionRequest(), c.GetSessionMessages)
		messageGroup.POST("", c.validateCreateMessageRequest(), c.SaveMessage)
		messageGroup.POST("/feedback", c.SaveFeedback)
	}

	// ML API endpoints (with separate authentication)
	mlGroup := router.Group("/ml/messages")
	mlGroup.Use(c.mlAuthMiddleware())
	{
		mlGroup.GET("", c.validateListMessagesRequest(), c.GetMessagesForML)
		mlGroup.POST("/process", c.validateProcessMessageRequest(), c.ProcessMessage)
	}
}

// authMiddleware ensures user authentication
func (c *MessageController) authMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.GetHeader("Authorization")
		if token == "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header is required",
				"code":  "AUTH_REQUIRED",
			})
			return
		}

		// Strip "Bearer " prefix if present
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}

		// Validate token
		claims, err := c.jwtService.ValidateToken(token)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid token",
				"code":  "INVALID_TOKEN",
			})
			return
		}

		// Add claims to context
		ctx.Set("userId", claims.UserID)
		ctx.Next()
	}
}

// validateListMessagesRequest validates the request to list messages
func (c *MessageController) validateListMessagesRequest() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// Validate character ID
		charIDStr := ctx.Query("characterId")
		if charIDStr == "" {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "Character ID is required",
				"code":  "MISSING_CHARACTER_ID",
			})
			return
		}

		// Convert and validate character ID format
		charID, err := strconv.ParseUint(charIDStr, 10, 64)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "Invalid character ID format",
				"code":  "INVALID_CHARACTER_ID",
			})
			return
		}

		// Optional session ID filter
		sessionID := ctx.Query("sessionId")

		// Optional limit parameter
		limitStr := ctx.DefaultQuery("limit", "50")
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 || limit > 200 {
			limit = 50 // Default limit
		}

		// Store validated parameters in context
		ctx.Set("characterId", uint(charID))
		if sessionID != "" {
			ctx.Set("sessionId", sessionID)
		}
		ctx.Set("limit", limit)

		log.Printf("[%s] Validated ListMessagesRequest: CharacterID=%d, SessionID=%s, Limit=%d", ctx.FullPath(), uint(charID), sessionID, limit)

		ctx.Next()
	}
}

// validateSessionRequest validates the session parameter
func (c *MessageController) validateSessionRequest() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		sessionID := ctx.Param("sessionId")
		if sessionID == "" {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "Session ID is required",
				"code":  "MISSING_SESSION_ID",
			})
			return
		}

		// Validate character ID from query parameter
		charIDStr := ctx.Query("characterId")
		if charIDStr == "" {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "Character ID is required",
				"code":  "MISSING_CHARACTER_ID",
			})
			return
		}

		charID, err := strconv.ParseUint(charIDStr, 10, 64)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "Invalid character ID format",
				"code":  "INVALID_CHARACTER_ID",
			})
			return
		}

		// Store validated parameters
		ctx.Set("sessionId", sessionID)
		ctx.Set("characterId", uint(charID))

		ctx.Next()
	}
}

// validateCreateMessageRequest validates the message creation request
func (c *MessageController) validateCreateMessageRequest() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req struct {
			SessionID   string `json:"sessionId" binding:"required"`
			CharacterID uint   `json:"characterId" binding:"required"`
			Content     string `json:"content" binding:"required"`
			Sender      string `json:"sender" binding:"required,oneof=user character system"`
		}

		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"code":    "INVALID_REQUEST",
				"details": "Request must include sessionId, characterId, content, and sender fields",
			})
			return
		}

		if len(req.Content) > 8000 {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "Message content too long (max 8000 characters)",
				"code":  "MESSAGE_TOO_LONG",
			})
			return
		}

		// Store validated request in context
		ctx.Set("messageRequest", req)
		ctx.Next()
	}
}

// validateProcessMessageRequest validates the message processing request
func (c *MessageController) validateProcessMessageRequest() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req struct {
			SessionID   string `json:"sessionId" binding:"required"`
			CharacterID uint   `json:"characterId" binding:"required"`
			Message     string `json:"message" binding:"required"`
		}

		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request format",
				"code":    "INVALID_REQUEST",
				"details": "Request must include sessionId, characterId, and message fields",
			})
			return
		}

		// Store validated request in context
		ctx.Set("processRequest", req)
		ctx.Next()
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

		// Validate API key
		if token != c.mlApiKey {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// GetSessionMessages retrieves all messages for a session
func (c *MessageController) GetSessionMessages(ctx *gin.Context) {
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

	messages, err := c.messageService.GetSessionMessages(uint(charID), sessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error retrieving messages: %v", err)})
		return
	}

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
	var request struct {
		SessionID   string `json:"sessionId" binding:"required"`
		CharacterID uint   `json:"characterId" binding:"required"`
		Content     string `json:"content" binding:"required"`
	}

	if err := ctx.BindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	userMessage := &ws.ChatMessage{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Sender:    "user",
		Content:   request.Content,
		Timestamp: time.Now(),
	}

	err := c.messageService.SaveMessage(request.CharacterID, request.SessionID, userMessage)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error saving message: %v", err)})
		return
	}

	character, err := c.characterService.GetCharacter(request.CharacterID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error fetching character: %v", err)})
		return
	}

	wsCharacter := &ws.Character{
		ID:          character.ID,
		Name:        character.Name,
		Description: character.Description,
		Personality: character.Personality,
		VoiceType:   character.VoiceType,
	}

	dbMessages, err := c.messageService.GetSessionMessages(request.CharacterID, request.SessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error loading previous messages: %v", err)})
		return
	}

	wsMessages := make([]ws.ChatMessage, len(dbMessages))
	for i, msg := range dbMessages {
		wsMessages[i] = ws.ChatMessage{
			ID:        msg.ExternalID,
			Sender:    msg.Sender,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
	}

	aiResponse, err := c.aiService.GenerateResponse(wsCharacter, request.Content, wsMessages)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error generating AI response: %v", err)})
		return
	}

	characterMessage := &ws.ChatMessage{
		ID:        fmt.Sprintf("resp-%d", time.Now().UnixNano()),
		Sender:    "character",
		Content:   aiResponse,
		Timestamp: time.Now(),
	}

	err = c.messageService.SaveMessage(request.CharacterID, request.SessionID, characterMessage)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error saving character response: %v", err)})
		return
	}

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
	messageID := ctx.Param("id")
	if messageID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Message ID is required"})
		return
	}

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

	messages, err := c.messageService.GetSessionMessages(uint(charID), sessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error retrieving messages: %v", err)})
		return
	}

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

	ctx.JSON(http.StatusNotImplemented, gin.H{"error": "This endpoint is not yet implemented"})
}

// GetMessages retrieves a list of messages for a character or session
func (c *MessageController) GetMessages(ctx *gin.Context) {
	charID, exists := ctx.Get("characterId")
	if !exists {
		log.Printf("[%s] GetMessages Error: Missing character ID in context", ctx.FullPath())
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Missing character ID"})
		return
	}

	sessionID, hasSession := ctx.Get("sessionId")
	queryLimit, limitExists := ctx.Get("limit")
	queryOffset, offsetExists := ctx.Get("offset")

	charIDUint := charID.(uint)
	var sessionIDStr string
	if hasSession {
		sessionIDStr = sessionID.(string)
	}

	limitInt := 30 // Default
	if limitExists {
		limitInt = queryLimit.(int)
	}
	offsetInt := 0 // Default
	if offsetExists {
		offsetInt = queryOffset.(int)
	}

	log.Printf("[%s] GetMessages Handler: CharacterID=%d, SessionID=%s (Exists: %t), Limit=%d, Offset=%d", ctx.FullPath(), charIDUint, sessionIDStr, hasSession, limitInt, offsetInt)

	if hasSession {
		sessionMessages, totalCount, err := c.messageService.GetSessionMessagesPaginated(charID.(uint), sessionID.(string), queryLimit.(int), queryOffset.(int))
		if err != nil {
			log.Printf("[%s] GetMessages Error: Error retrieving session messages for CharacterID=%d, SessionID=%s, Limit=%d, Offset=%d - %v", ctx.FullPath(), charIDUint, sessionIDStr, limitInt, offsetInt, err)
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Error retrieving session messages: %v", err),
			})
			return
		}
		log.Printf("[%s] Successfully retrieved %d messages (Total: %d) for CharacterID=%d, SessionID=%s", ctx.FullPath(), len(sessionMessages), totalCount, charIDUint, sessionIDStr)
		formattedMessages := make([]map[string]interface{}, len(sessionMessages))
		for i, msg := range sessionMessages {
			formattedMessages[i] = map[string]interface{}{
				"id":        msg.ExternalID,
				"sender":    msg.Sender,
				"content":   msg.Content,
				"timestamp": msg.Timestamp,
			}
		}
		ctx.JSON(http.StatusOK, gin.H{
			"characterId": charID,
			"sessionId":   sessionID,
			"messages":    formattedMessages,
			"count":       totalCount,
			"limit":       queryLimit,
			"offset":      queryOffset,
		})
	} else {
		ctx.JSON(http.StatusNotImplemented, gin.H{
			"error": "Retrieving all messages for a character without session ID is not implemented",
		})
		return
	}
}

// SaveMessage saves a new message to the database
func (c *MessageController) SaveMessage(ctx *gin.Context) {
	reqInterface, exists := ctx.Get("messageRequest")
	if !exists {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Missing message data"})
		return
	}

	req := reqInterface.(struct {
		SessionID   string `json:"sessionId" binding:"required"`
		CharacterID uint   `json:"characterId" binding:"required"`
		Content     string `json:"content" binding:"required"`
		Sender      string `json:"sender" binding:"required,oneof=user character system"`
	})

	chatMessage := &ws.ChatMessage{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Sender:    req.Sender,
		Content:   req.Content,
		Timestamp: time.Now(),
	}

	err := c.messageService.SaveMessage(req.CharacterID, req.SessionID, chatMessage)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Error saving message: %v", err),
		})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"id":          chatMessage.ID,
		"sender":      chatMessage.Sender,
		"content":     chatMessage.Content,
		"timestamp":   chatMessage.Timestamp,
		"characterId": req.CharacterID,
		"sessionId":   req.SessionID,
	})
}

// GetMessagesForML retrieves messages for ML processing
func (c *MessageController) GetMessagesForML(ctx *gin.Context) {
	charID, exists := ctx.Get("characterId")
	if !exists {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Missing character ID"})
		return
	}

	sessionID, hasSession := ctx.Get("sessionId")

	if hasSession {
		sessionMessages, err := c.messageService.GetSessionMessages(charID.(uint), sessionID.(string))
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Error retrieving session messages: %v", err),
			})
			return
		}

		formattedMessages := make([]map[string]interface{}, len(sessionMessages))
		for i, msg := range sessionMessages {
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
			"characterId": charID,
			"sessionId":   sessionID,
			"messages":    formattedMessages,
			"count":       len(formattedMessages),
		})
	} else {
		ctx.JSON(http.StatusNotImplemented, gin.H{
			"error": "Retrieving all messages for ML processing is not implemented",
		})
		return
	}
}

// ProcessMessage processes a message with AI and returns a response
func (c *MessageController) ProcessMessage(ctx *gin.Context) {
	reqInterface, exists := ctx.Get("processRequest")
	if !exists {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Missing message data"})
		return
	}

	req := reqInterface.(struct {
		SessionID   string `json:"sessionId" binding:"required"`
		CharacterID uint   `json:"characterId" binding:"required"`
		Message     string `json:"message" binding:"required"`
	})

	character, err := c.characterService.GetCharacter(req.CharacterID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Error fetching character: %v", err),
		})
		return
	}

	wsCharacter := &ws.Character{
		ID:          character.ID,
		Name:        character.Name,
		Description: character.Description,
		Personality: character.Personality,
		VoiceType:   character.VoiceType,
	}

	dbMessages, err := c.messageService.GetSessionMessages(req.CharacterID, req.SessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Error loading message history: %v", err),
		})
		return
	}

	wsMessages := make([]ws.ChatMessage, len(dbMessages))
	for i, msg := range dbMessages {
		wsMessages[i] = ws.ChatMessage{
			ID:        msg.ExternalID,
			Sender:    msg.Sender,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
	}

	response, err := c.aiService.GenerateResponse(wsCharacter, req.Message, wsMessages)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Error generating AI response: %v", err),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"response":    response,
		"characterId": req.CharacterID,
		"sessionId":   req.SessionID,
	})
}

// Feedback model
type FeedbackRequest struct {
	MessageID    string `json:"messageId" binding:"required"`
	UserID       uint   `json:"userId" binding:"required"`
	FeedbackType string `json:"feedbackType" binding:"required,oneof=up down flag"`
	Timestamp    int64  `json:"timestamp"`
}

// SaveFeedback handler
func (c *MessageController) SaveFeedback(ctx *gin.Context) {
	var req FeedbackRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid feedback request"})
		return
	}
	err := c.messageService.SaveFeedback(req.MessageID, req.UserID, req.FeedbackType, req.Timestamp)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"status": "ok"})
}
