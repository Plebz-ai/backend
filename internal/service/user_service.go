package service

import (
	"encoding/json"
	"errors"
	"time"

	"ai-agent-character-demo/backend/internal/models"
	"ai-agent-character-demo/backend/pkg/jwt"

	"gorm.io/gorm"
)

var (
	ErrUserAlreadyExists  = errors.New("user with this email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidRole        = errors.New("invalid role")
)

// UserService handles user-related operations
type UserService struct {
	db         *gorm.DB
	jwtService *jwt.Service
}

// NewUserService creates a new user service
func NewUserService(db *gorm.DB, jwtService *jwt.Service) *UserService {
	return &UserService{
		db:         db,
		jwtService: jwtService,
	}
}

// CreateUser creates a new user
func (s *UserService) CreateUser(req *models.CreateUserRequest) (*models.User, string, error) {
	// Check if user already exists
	var existingUser models.User
	result := s.db.Where("email = ?", req.Email).First(&existingUser)
	if result.RowsAffected > 0 {
		return nil, "", ErrUserAlreadyExists
	}

	// Validate and normalize role (default to user if not specified or invalid)
	role := jwt.Role(req.Role)
	if role == "" {
		role = jwt.RoleUser
	} else if role != jwt.RoleUser && role != jwt.RoleAdmin && role != jwt.RoleGuest {
		return nil, "", ErrInvalidRole
	}

	// Get default permissions for this role
	permissions := jwt.GetRolePermissions(role)
	permissionsJSON, _ := json.Marshal(permissions)

	// Create new user
	user := models.User{
		Name:        req.Name,
		Email:       req.Email,
		Password:    req.Password,
		Role:        string(role),
		Permissions: string(permissionsJSON),
	}

	// Save user to database
	if err := s.db.Create(&user).Error; err != nil {
		return nil, "", err
	}

	// Generate JWT token
	token, err := s.jwtService.GenerateToken(user.ID, user.Email, role, permissions)
	if err != nil {
		return nil, "", err
	}

	return &user, token, nil
}

// Login authenticates a user and returns a JWT token
func (s *UserService) Login(req *models.LoginRequest) (*models.User, string, error) {
	// Find user by email
	var user models.User
	result := s.db.Where("email = ?", req.Email).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", result.Error
	}

	// Check password
	if !models.CheckPasswordHash(req.Password, user.Password) {
		return nil, "", ErrInvalidCredentials
	}

	// Update last login time
	s.db.Model(&user).Update("last_login", time.Now())

	// Generate JWT token with role
	role := jwt.Role(user.Role)

	// Get permissions for this role
	var permissions []jwt.Permission
	if user.Permissions != "" {
		_ = json.Unmarshal([]byte(user.Permissions), &permissions)
	} else {
		permissions = jwt.GetRolePermissions(role)
	}

	token, err := s.jwtService.GenerateToken(user.ID, user.Email, role, permissions)
	if err != nil {
		return nil, "", err
	}

	return &user, token, nil
}

// GetUserByID retrieves a user by ID
func (s *UserService) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	result := s.db.First(&user, id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, result.Error
	}
	return &user, nil
}

// GetUserByEmail retrieves a user by email
func (s *UserService) GetUserByEmail(email string) (*models.User, error) {
	var user models.User
	result := s.db.Where("email = ?", email).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, result.Error
	}
	return &user, nil
}

// UpdateUserRole updates a user's role and permissions
func (s *UserService) UpdateUserRole(userID uint, role jwt.Role) error {
	// Validate role
	if role != jwt.RoleUser && role != jwt.RoleAdmin && role != jwt.RoleGuest {
		return ErrInvalidRole
	}

	// Get default permissions for this role
	permissions := jwt.GetRolePermissions(role)
	permissionsJSON, _ := json.Marshal(permissions)

	// Update user
	result := s.db.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"role":        string(role),
			"permissions": string(permissionsJSON),
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}
