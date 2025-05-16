package api

import (
	"fmt"
	"log"
	"net/http"

	"ai-agent-character-demo/backend/internal/models"
	"ai-agent-character-demo/backend/internal/service"
	"ai-agent-character-demo/backend/pkg/jwt"
	"ai-agent-character-demo/backend/pkg/logger"

	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication-related requests
type AuthHandler struct {
	service    *service.UserService
	jwtService *jwt.Service
	logger     *logger.Logger
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(service *service.UserService, jwtService *jwt.Service, logger *logger.Logger) *AuthHandler {
	return &AuthHandler{
		service:    service,
		jwtService: jwtService,
		logger:     logger,
	}
}

// Signup handles user registration
func (h *AuthHandler) Signup(c *gin.Context) {
	var req models.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Error binding JSON for signup", "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	user, token, err := h.service.CreateUser(&req)
	if err != nil {
		switch err {
		case service.ErrUserAlreadyExists:
			c.JSON(http.StatusConflict, gin.H{"error": "A user with this email already exists"})
		case service.ErrInvalidRole:
			c.JSON(http.StatusBadRequest, gin.H{"error": "The provided role is invalid"})
		default:
			h.logger.Error("Error creating user", "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user account"})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"user":  user.ToResponse(),
		"token": token,
	})
}

// Login handles user authentication
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("Error binding JSON for login", "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	user, token, err := h.service.Login(&req)
	if err != nil {
		switch err {
		case service.ErrInvalidCredentials:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		default:
			h.logger.Error("Error during login", "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "An error occurred during login"})
		}
		return
	}

	h.logger.Info("User logged in successfully",
		"userID", user.ID,
		"email", user.Email,
		"role", user.Role,
	)

	c.JSON(http.StatusOK, gin.H{
		"user":  user.ToResponse(),
		"token": token,
	})
}

// Me returns the current authenticated user
func (h *AuthHandler) Me(c *gin.Context) {
	// Print the Authorization header for debugging
	authHeader := c.GetHeader("Authorization")
	log.Printf("[DEBUG] /auth/me Authorization header: %s", authHeader)
	// Get user ID from JWT claims
	userID, exists := c.Get("userId")
	log.Printf("[DEBUG] /auth/me userID in context: %v exists: %v", userID, exists)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	user, err := h.service.GetUserByID(userID.(uint))
	if err != nil {
		switch err {
		case service.ErrUserNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		default:
			h.logger.Error("Error getting user", "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve user"})
		}
		return
	}

	c.JSON(http.StatusOK, user.ToResponse())
}

// UpdateUserRole allows admins to update a user's role
func (h *AuthHandler) UpdateUserRole(c *gin.Context) {
	// Only admins can update roles
	claims, exists := c.Get("claims")
	if !exists || !claims.(*jwt.JWTClaims).HasRole(jwt.RoleAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	var req struct {
		Role string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Convert userID to uint and update role
	var userIDUint uint
	if _, err := fmt.Sscanf(userID, "%d", &userIDUint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID must be a number"})
		return
	}

	if err := h.service.UpdateUserRole(userIDUint, jwt.Role(req.Role)); err != nil {
		switch err {
		case service.ErrUserNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		case service.ErrInvalidRole:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role. Must be user, admin, or guest"})
		default:
			h.logger.Error("Error updating user role", "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user role"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User role updated successfully",
	})
}
