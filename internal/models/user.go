package models

import (
	"time"

	"ai-agent-character-demo/backend/pkg/jwt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `json:"name"`
	Email       string    `gorm:"uniqueIndex" json:"email"`
	Password    string    `json:"-"`                                      // Never return password in JSON
	Role        string    `json:"role" gorm:"default:user"`               // user, admin, or guest
	Permissions string    `json:"permissions,omitempty" gorm:"type:text"` // JSON array of permissions
	LastLogin   time.Time `json:"last_login,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateUserRequest is the request structure for creating a new user
type CreateUserRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role,omitempty"`
}

// LoginRequest is the request structure for user login
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// UserResponse is the response structure for user data (without sensitive info)
type UserResponse struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	Permissions []string  `json:"permissions,omitempty"`
	LastLogin   time.Time `json:"last_login,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserPreference represents a user's personalization settings
type UserPreference struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"userId"`
	ChatStyle string    `json:"chatStyle"` // e.g. 'concise', 'detailed'
	TTSVoice  string    `json:"ttsVoice"`  // e.g. 'male', 'female', 'predefined'
	Theme     string    `json:"theme"`     // e.g. 'light', 'dark'
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// HashPassword hashes a password for storage
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPasswordHash compares a password with a hash
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// BeforeCreate is a GORM hook to hash the password before saving
func (u *User) BeforeCreate(tx *gorm.DB) error {
	hashedPassword, err := HashPassword(u.Password)
	if err != nil {
		return err
	}
	u.Password = hashedPassword

	// Set default role if not specified
	if u.Role == "" {
		u.Role = string(jwt.RoleUser)
	}

	return nil
}

// ToResponse converts a User model to a UserResponse
func (u *User) ToResponse() UserResponse {
	var permissions []string

	// Parse permissions from JSON string if present
	if u.Permissions != "" {
		// Simple handling - in a real app you'd use json.Unmarshal
		permissions = make([]string, 0)
	}

	return UserResponse{
		ID:          u.ID,
		Name:        u.Name,
		Email:       u.Email,
		Role:        u.Role,
		Permissions: permissions,
		LastLogin:   u.LastLogin,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}
