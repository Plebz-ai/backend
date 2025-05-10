package api

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ai-agent-character-demo/backend/internal/models"
	"ai-agent-character-demo/backend/internal/service"

	"github.com/gin-gonic/gin"
)

// NOTE: The context key for user ID is always 'userId' (lowercase 'd'), matching the auth middleware.
// Do not use 'userID' (uppercase 'D').

type CharacterHandler struct {
	service *service.CharacterService
}

func NewCharacterHandler(service *service.CharacterService) *CharacterHandler {
	return &CharacterHandler{service: service}
}

func (h *CharacterHandler) CreateCharacter(c *gin.Context) {
	// Check content type to determine if it's a multipart form
	contentType := c.GetHeader("Content-Type")
	isMultipart := strings.Contains(contentType, "multipart/form-data")

	var req models.CreateCharacterRequest

	if isMultipart {
		// Handle multipart form data (file upload)
		// Parse form data
		if err := c.Request.ParseMultipartForm(10 << 20); err != nil { // 10 MB max
			log.Printf("Error parsing multipart form: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Get form values
		req.Name = c.PostForm("name")
		req.Description = c.PostForm("description")
		req.Personality = c.PostForm("personality")
		req.VoiceType = c.PostForm("voice_type")
		// Read is_custom from form (if present)
		isCustomStr := c.PostForm("is_custom")
		if isCustomStr == "true" || isCustomStr == "1" {
			req.IsCustom = true
		} else {
			req.IsCustom = false
		}

		// First try to process base64 encoded avatar if available
		base64Image := c.PostForm("avatar_base64")
		avatarFilename := c.PostForm("avatar_filename")

		if base64Image != "" && avatarFilename != "" {
			// Create a unique filename with timestamp
			timestamp := time.Now().UnixNano()
			filename := fmt.Sprintf("%d_%s", timestamp, filepath.Base(avatarFilename))
			savePath := filepath.Join("uploads", filename)

			// Extract the base64 data - strip the prefix like "data:image/jpeg;base64,"
			commaIndex := strings.Index(base64Image, ",")
			if commaIndex != -1 {
				base64Image = base64Image[commaIndex+1:]
			}

			// Decode the base64 image
			imageData, err := base64.StdEncoding.DecodeString(base64Image)
			if err != nil {
				log.Printf("Error decoding base64 image: %v", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid base64 image data"})
				return
			}

			// Create the uploads directory if it doesn't exist
			if err := os.MkdirAll("uploads", 0755); err != nil {
				log.Printf("Error creating uploads directory: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create uploads directory"})
				return
			}

			// Save the image to a file
			if err := os.WriteFile(savePath, imageData, 0644); err != nil {
				log.Printf("Error saving image file: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save image"})
				return
			}

			// Set the avatar URL
			host := c.Request.Host
			scheme := "http"
			if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
				scheme = "https"
			}
			req.AvatarURL = fmt.Sprintf("%s://%s/uploads/%s", scheme, host, filename)
		} else {
			// Fall back to regular file upload if base64 isn't available
			file, header, err := c.Request.FormFile("avatar")
			if err == nil {
				// File was included
				defer file.Close()

				// Check if it's an image
				if !strings.HasPrefix(header.Header.Get("Content-Type"), "image/") {
					c.JSON(http.StatusBadRequest, gin.H{"error": "uploaded file is not an image"})
					return
				}

				// Create a unique filename with timestamp to avoid conflicts
				timestamp := time.Now().UnixNano()
				filename := fmt.Sprintf("%d_%s", timestamp, filepath.Base(header.Filename))
				savePath := filepath.Join("uploads", filename)

				// Save the file
				if err := c.SaveUploadedFile(header, savePath); err != nil {
					log.Printf("Error saving file: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
					return
				}

				// Set the avatar URL using the server host
				// In production, you would use your domain name
				host := c.Request.Host
				scheme := "http"
				if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
					scheme = "https"
				}
				req.AvatarURL = fmt.Sprintf("%s://%s/uploads/%s", scheme, host, filename)
			}
		}
	} else {
		// Handle JSON request
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Printf("Error binding JSON: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// Ensure IsCustom is set (default false if not present)
		if !req.IsCustom {
			req.IsCustom = false
		}
	}

	// Validate required fields
	if req.Name == "" || req.Description == "" || req.Personality == "" || req.VoiceType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing required fields"})
		return
	}

	log.Printf("Creating character: %+v", req)
	character, err := h.service.CreateCharacter(&req)
	if err != nil {
		log.Printf("Error creating character: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, character)
}

func (h *CharacterHandler) GetCharacter(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id format"})
		return
	}

	character, err := h.service.GetCharacter(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, character)
}

func (h *CharacterHandler) ListCharacters(c *gin.Context) {
	// Get user ID from the JWT token context
	userIdInterface, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Convert to uint
	userId, ok := userIdInterface.(uint)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID format"})
		return
	}

	// Get characters with conversations involving this user
	characters, err := h.service.ListCharactersWithConversations(userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, characters)
}
