package api

import (
	"net/http"
	"strconv"

	"ai-agent-character-demo/backend/conversation/models"
	"ai-agent-character-demo/backend/conversation/service"

	// "ai-agent-demo/backend/conversation/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
)

type MessageHandler struct {
	service *service.MessageService
}

func NewMessageHandler(service *service.MessageService) *MessageHandler {
	return &MessageHandler{service: service}
}

func (h *MessageHandler) CreateMessage(c *gin.Context) {
	var message models.Message
	if err := c.ShouldBindJSON(&message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Extract userID from JWT claims in context
	var userID uint
	claims, exists := c.Get("user")
	if exists {
		if jwtClaims, ok := claims.(jwt.MapClaims); ok {
			userID = uint(jwtClaims["userId"].(float64))
			c.Set("userId", userID)
		}
	}
	if err := h.service.CreateMessage(&message, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, message)
}

func (h *MessageHandler) GetMessageByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message ID"})
		return
	}
	message, err := h.service.GetMessageByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Message not found"})
		return
	}
	c.JSON(http.StatusOK, message)
}

func (h *MessageHandler) GetMessagesBySession(c *gin.Context) {
	sessionID := c.Param("session_id")
	messages, err := h.service.GetMessagesBySession(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, messages)
}
