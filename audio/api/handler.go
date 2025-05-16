package api

import (
	"net/http"
	"strconv"

	"ai-agent-character-demo/backend/audio/models"
	"ai-agent-character-demo/backend/audio/service"

	"github.com/gin-gonic/gin"
)

type AudioHandler struct {
	service *service.AudioService
}

func NewAudioHandler(service *service.AudioService) *AudioHandler {
	return &AudioHandler{service: service}
}

func (h *AudioHandler) CreateAudioChunk(c *gin.Context) {
	var chunk models.AudioChunk
	if err := c.ShouldBindJSON(&chunk); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.CreateAudioChunk(&chunk); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, chunk)
}

func (h *AudioHandler) GetAudioChunkByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid audio chunk ID"})
		return
	}
	chunk, err := h.service.GetAudioChunkByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Audio chunk not found"})
		return
	}
	c.JSON(http.StatusOK, chunk)
}

func (h *AudioHandler) GetAudioChunksBySession(c *gin.Context) {
	sessionID := c.Param("session_id")
	chunks, err := h.service.GetAudioChunksBySession(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, chunks)
}
