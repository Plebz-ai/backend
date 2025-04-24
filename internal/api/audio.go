package api

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"ai-agent-character-demo/backend/internal/service"
	"ai-agent-character-demo/backend/pkg/jwt"
)

// AudioController handles audio-related API endpoints
type AudioController struct {
	audioService *service.AudioService
	jwtService   *jwt.Service
}

// NewAudioController creates a new audio controller
func NewAudioController(audioService *service.AudioService, jwtService *jwt.Service) *AudioController {
	return &AudioController{
		audioService: audioService,
		jwtService:   jwtService,
	}
}

// RegisterRoutes registers the routes for the audio controller
func (c *AudioController) RegisterRoutes(router *gin.Engine) {
	audioGroup := router.Group("/api/audio")
	audioGroup.Use(c.authMiddleware())
	{
		audioGroup.POST("/upload", c.UploadAudio)
		audioGroup.GET("/chunk/:id", c.GetAudioChunk)
		audioGroup.GET("/chunks/session/:sessionId", c.GetSessionAudioChunks)
		audioGroup.DELETE("/chunk/:id", c.DeleteAudioChunk)
	}

	// ML Engineer API endpoints (separate authentication)
	mlGroup := router.Group("/api/ml")
	mlGroup.Use(c.mlAuthMiddleware())
	{
		mlGroup.GET("/audio/chunk/:id", c.GetAudioChunkForML)
		mlGroup.GET("/audio/chunks/pending", c.GetPendingAudioChunks)
		mlGroup.GET("/audio/chunks/all", c.GetAllAudioChunks)
		mlGroup.PUT("/audio/chunk/:id/status", c.UpdateAudioChunkStatus)
	}

	// New endpoint for streaming audio chunks
	mlGroup.POST("/audio/chunks", c.StreamAudio)
}

// authMiddleware ensures user authentication
func (c *AudioController) authMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.GetHeader("Authorization")
		if token == "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			return
		}

		// Strip "Bearer " prefix if present
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}

		// Validate token
		claims, err := c.jwtService.ValidateToken(token)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		// Add claims to context
		ctx.Set("userId", claims.UserID)
		ctx.Next()
	}
}

// mlAuthMiddleware provides special authentication for ML API
func (c *AudioController) mlAuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		apiKey := ctx.GetHeader("X-ML-API-Key")
		if apiKey == "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API key is required"})
			return
		}

		// TODO: Replace with actual API key validation
		// For now, using a simple hardcoded key for demo
		if apiKey != "ml-api-key-12345" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			return
		}

		ctx.Next()
	}
}

// UploadAudio handles audio file uploads
func (c *AudioController) UploadAudio(ctx *gin.Context) {
	userId, exists := ctx.Get("userId")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in token"})
		return
	}

	// Get session and character IDs
	sessionID := ctx.PostForm("sessionId")
	if sessionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	charIDStr := ctx.PostForm("charId")
	if charIDStr == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "charId is required"})
		return
	}

	charID, err := strconv.ParseUint(charIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid character ID"})
		return
	}

	// Parse audio format
	format := ctx.PostForm("format")
	if format == "" {
		format = "webm" // Default format
	}

	// Parse audio metadata
	sampleRateStr := ctx.PostForm("sampleRate")
	sampleRate := 48000 // Default sample rate
	if sampleRateStr != "" {
		parsedRate, err := strconv.ParseInt(sampleRateStr, 10, 64)
		if err == nil {
			sampleRate = int(parsedRate)
		}
	}

	channelsStr := ctx.PostForm("channels")
	channels := 1 // Default mono
	if channelsStr != "" {
		parsedChannels, err := strconv.ParseInt(channelsStr, 10, 64)
		if err == nil {
			channels = int(parsedChannels)
		}
	}

	durationStr := ctx.PostForm("duration")
	duration := 0.0 // Default duration
	if durationStr != "" {
		parsedDuration, err := strconv.ParseFloat(durationStr, 64)
		if err == nil {
			duration = parsedDuration
		}
	}

	// Get TTL duration (optional)
	ttlStr := ctx.PostForm("ttl")
	ttl := 24 * time.Hour // Default 24 hour TTL
	if ttlStr != "" {
		parsedTTL, err := time.ParseDuration(ttlStr)
		if err == nil && parsedTTL > 0 {
			ttl = parsedTTL
		}
	}

	// Get additional metadata (optional)
	metadata := ctx.PostForm("metadata")

	// Get the audio file
	file, err := ctx.FormFile("audioFile")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Error getting audio file: %v", err)})
		return
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error opening file: %v", err)})
		return
	}
	defer src.Close()

	// Read the file content
	audioData, err := io.ReadAll(src)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error reading file: %v", err)})
		return
	}

	// Store the audio chunk
	chunkID, err := c.audioService.StoreAudioChunk(
		fmt.Sprintf("%d", userId.(uint)),
		sessionID,
		uint(charID),
		audioData,
		format,
		duration,
		sampleRate,
		channels,
		metadata,
		ttl,
	)

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error storing audio: %v", err)})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"id":        chunkID,
		"size":      len(audioData),
		"format":    format,
		"duration":  duration,
		"expiresAt": time.Now().Add(ttl),
	})
}

// GetAudioChunk retrieves a single audio chunk by ID
func (c *AudioController) GetAudioChunk(ctx *gin.Context) {
	userId, exists := ctx.Get("userId")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in token"})
		return
	}

	chunkID := ctx.Param("id")
	if chunkID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Chunk ID is required"})
		return
	}

	chunk, err := c.audioService.GetAudioChunk(chunkID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Error retrieving audio chunk: %v", err)})
		return
	}

	// Ensure user has access to this chunk
	if chunk.UserID != fmt.Sprintf("%d", userId.(uint)) {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to access this audio chunk"})
		return
	}

	// Return audio data
	ctx.Header("Content-Type", "audio/"+chunk.Format)
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=audio-%s.%s", chunkID, chunk.Format))
	ctx.Data(http.StatusOK, "audio/"+chunk.Format, chunk.AudioData)
}

// GetSessionAudioChunks retrieves all audio chunks for a session
func (c *AudioController) GetSessionAudioChunks(ctx *gin.Context) {
	userId, exists := ctx.Get("userId")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in token"})
		return
	}

	sessionID := ctx.Param("sessionId")
	if sessionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Session ID is required"})
		return
	}

	chunks, err := c.audioService.GetSessionAudioChunks(sessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error retrieving session audio chunks: %v", err)})
		return
	}

	// Filter chunks by user ID for security
	userChunks := make([]map[string]interface{}, 0)
	for _, chunk := range chunks {
		if chunk.UserID == fmt.Sprintf("%d", userId.(uint)) {
			userChunks = append(userChunks, map[string]interface{}{
				"id":               chunk.ID,
				"format":           chunk.Format,
				"duration":         chunk.Duration,
				"sampleRate":       chunk.SampleRate,
				"channels":         chunk.Channels,
				"createdAt":        chunk.CreatedAt,
				"expiresAt":        chunk.ExpiresAt,
				"processingStatus": chunk.ProcessingStatus,
				"size":             len(chunk.AudioData),
			})
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"sessionId": sessionID,
		"chunks":    userChunks,
		"count":     len(userChunks),
	})
}

// DeleteAudioChunk deletes an audio chunk
func (c *AudioController) DeleteAudioChunk(ctx *gin.Context) {
	userId, exists := ctx.Get("userId")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in token"})
		return
	}

	chunkID := ctx.Param("id")
	if chunkID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Chunk ID is required"})
		return
	}

	// First, get the chunk to verify ownership
	chunk, err := c.audioService.GetAudioChunk(chunkID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Error retrieving audio chunk: %v", err)})
		return
	}

	// Ensure user has access to this chunk
	if chunk.UserID != fmt.Sprintf("%d", userId.(uint)) {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to delete this audio chunk"})
		return
	}

	if err := c.audioService.DeleteAudioChunk(chunkID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error deleting audio chunk: %v", err)})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Audio chunk deleted successfully"})
}

// GetAudioChunkForML retrieves a single audio chunk for ML processing
func (c *AudioController) GetAudioChunkForML(ctx *gin.Context) {
	chunkID := ctx.Param("id")
	if chunkID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Chunk ID is required"})
		return
	}

	chunk, err := c.audioService.GetAudioChunk(chunkID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Error retrieving audio chunk: %v", err)})
		return
	}

	// Return audio data with metadata
	ctx.JSON(http.StatusOK, gin.H{
		"id":               chunk.ID,
		"userId":           chunk.UserID,
		"sessionId":        chunk.SessionID,
		"charId":           chunk.CharID,
		"format":           chunk.Format,
		"duration":         chunk.Duration,
		"sampleRate":       chunk.SampleRate,
		"channels":         chunk.Channels,
		"createdAt":        chunk.CreatedAt,
		"expiresAt":        chunk.ExpiresAt,
		"metadata":         chunk.Metadata,
		"processingStatus": chunk.ProcessingStatus,
		"audioData":        chunk.AudioData,
	})
}

// GetPendingAudioChunks retrieves all pending audio chunks for ML processing
func (c *AudioController) GetPendingAudioChunks(ctx *gin.Context) {
	// Parse pagination parameters
	limitStr := ctx.DefaultQuery("limit", "10")
	offsetStr := ctx.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 10
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Get chunks from service
	chunks, total, err := c.audioService.GetPendingAudioChunks(limit, offset)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Error retrieving pending chunks: %v", err),
		})
		return
	}

	// Convert to response format
	response := make([]map[string]interface{}, 0, len(chunks))
	for _, chunk := range chunks {
		// Skip audio data in listing to reduce payload size
		response = append(response, map[string]interface{}{
			"id":               chunk.ID,
			"userId":           chunk.UserID,
			"sessionId":        chunk.SessionID,
			"charId":           chunk.CharID,
			"format":           chunk.Format,
			"duration":         chunk.Duration,
			"sampleRate":       chunk.SampleRate,
			"channels":         chunk.Channels,
			"createdAt":        chunk.CreatedAt,
			"expiresAt":        chunk.ExpiresAt,
			"metadata":         chunk.Metadata,
			"processingStatus": chunk.ProcessingStatus,
			"size":             len(chunk.AudioData),
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"chunks": response,
		"count":  len(response),
		"total":  total,
		"offset": offset,
		"limit":  limit,
	})
}

// UpdateAudioChunkStatus updates the processing status of an audio chunk
func (c *AudioController) UpdateAudioChunkStatus(ctx *gin.Context) {
	chunkID := ctx.Param("id")
	if chunkID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Chunk ID is required"})
		return
	}

	var statusUpdate struct {
		Status string `json:"status"`
	}

	if err := ctx.BindJSON(&statusUpdate); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if statusUpdate.Status == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Status is required"})
		return
	}

	// Valid statuses: pending, processing, completed, failed
	validStatuses := map[string]bool{
		"pending":    true,
		"processing": true,
		"completed":  true,
		"failed":     true,
	}

	if !validStatuses[statusUpdate.Status] {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status. Must be one of: pending, processing, completed, failed"})
		return
	}

	if err := c.audioService.UpdateProcessingStatus(chunkID, statusUpdate.Status); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error updating status: %v", err)})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Status updated successfully"})
}

// GetAllAudioChunks retrieves all audio chunks regardless of status
func (c *AudioController) GetAllAudioChunks(ctx *gin.Context) {
	limitStr := ctx.DefaultQuery("limit", "10")
	offsetStr := ctx.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit parameter"})
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid offset parameter"})
		return
	}

	chunks, total, err := c.audioService.GetAllAudioChunks(limit, offset)
	if err != nil {
		log.Printf("Error retrieving all audio chunks: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve audio chunks"})
		return
	}

	// Format response with metadata only (no audio data to reduce payload size)
	formattedChunks := make([]gin.H, len(chunks))
	for i, chunk := range chunks {
		formattedChunks[i] = gin.H{
			"id":               chunk.ID,
			"userId":           chunk.UserID,
			"sessionId":        chunk.SessionID,
			"charId":           chunk.CharID,
			"format":           chunk.Format,
			"duration":         chunk.Duration,
			"sampleRate":       chunk.SampleRate,
			"channels":         chunk.Channels,
			"createdAt":        chunk.CreatedAt,
			"expiresAt":        chunk.ExpiresAt,
			"metadata":         chunk.Metadata,
			"processingStatus": chunk.ProcessingStatus,
			"size":             len(chunk.AudioData),
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"chunks": formattedChunks,
		"count":  len(chunks),
		"limit":  limit,
		"offset": offset,
		"total":  total,
	})
}

// StreamAudio handles the request to stream audio chunks
func (c *AudioController) StreamAudio(ctx *gin.Context) {
	// Implementation of the StreamAudio function
	// This is a placeholder and should be implemented based on your specific requirements
	ctx.JSON(http.StatusNotImplemented, gin.H{"error": "StreamAudio function not implemented"})
}
