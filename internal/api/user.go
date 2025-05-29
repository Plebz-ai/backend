package api

import (
	"ai-agent-character-demo/backend/internal/models"
	"time"

	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
)

// UserController handles user preference endpoints
type UserController struct {
	db *gorm.DB
}

// NewUserController creates a new UserController
func NewUserController(db *gorm.DB) *UserController {
	return &UserController{db: db}
}

// Handler to get user preferences
func (c *UserController) GetUserPreferences(ctx *gin.Context) {
	userId, exists := ctx.Get("userId")
	if !exists {
		ctx.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}
	var pref models.UserPreference
	err := c.db.Where("user_id = ?", userId).First(&pref).Error
	if err != nil {
		ctx.JSON(200, gin.H{"preferences": nil})
		return
	}
	ctx.JSON(200, gin.H{"preferences": pref})
}

// Handler to set user preferences
func (c *UserController) SetUserPreferences(ctx *gin.Context) {
	userId, exists := ctx.Get("userId")
	if !exists {
		ctx.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}
	var req models.UserPreference
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(400, gin.H{"error": "Invalid request"})
		return
	}
	var pref models.UserPreference
	err := c.db.Where("user_id = ?", userId).First(&pref).Error
	now := time.Now()
	if err != nil {
		// Not found, create new
		pref = models.UserPreference{
			UserID:    userId.(uint),
			ChatStyle: req.ChatStyle,
			TTSVoice:  req.TTSVoice,
			Theme:     req.Theme,
			CreatedAt: now,
			UpdatedAt: now,
		}
		err = c.db.Create(&pref).Error
	} else {
		// Update existing
		pref.ChatStyle = req.ChatStyle
		pref.TTSVoice = req.TTSVoice
		pref.Theme = req.Theme
		pref.UpdatedAt = now
		err = c.db.Save(&pref).Error
	}
	if err != nil {
		ctx.JSON(500, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(200, gin.H{"preferences": pref})
}
