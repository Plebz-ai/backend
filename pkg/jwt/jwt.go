package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Role represents user roles in the system
type Role string

// Permission represents specific permissions in the system
type Permission string

// Role constants
const (
	RoleAdmin  Role = "admin"
	RoleUser   Role = "user"
	RoleGuest  Role = "guest"
	RoleSystem Role = "system"
)

// Permission constants
const (
	PermReadCharacter   Permission = "read:character"
	PermWriteCharacter  Permission = "write:character"
	PermDeleteCharacter Permission = "delete:character"
	PermManageUsers     Permission = "manage:users"
	PermAccessAnalytics Permission = "access:analytics"
	PermManageSystem    Permission = "manage:system"
	PermAccessAudio     Permission = "access:audio"
)

// Common errors
var (
	ErrInvalidToken      = errors.New("invalid token")
	ErrExpiredToken      = errors.New("token has expired")
	ErrInvalidSigningKey = errors.New("invalid signing key")
	ErrTokenEmpty        = errors.New("token is empty")
	ErrInvalidClaims     = errors.New("invalid token claims")
)

// JWTClaims holds JWT claims data
type JWTClaims struct {
	UserID      uint         `json:"userId"`
	Email       string       `json:"email"`
	Role        Role         `json:"role"`
	Permissions []Permission `json:"permissions"`
	jwt.RegisteredClaims
}

// Service provides JWT operations
type Service struct {
	secretKey     []byte
	refreshKey    []byte
	tokenExpiry   time.Duration
	refreshExpiry time.Duration
}

// NewService creates a new JWT service
func NewService(secretKey string, expiry time.Duration) *Service {
	if secretKey == "" {
		secretKey = getSecretKey()
	}

	if expiry == 0 {
		expiry = 24 * time.Hour // Default to 24 hours
	}

	// Use same key for refresh tokens if not specifically configured
	refreshKey := secretKey + "-refresh"
	refreshExpiry := expiry * 7 // Default refresh expiry is 7x the token expiry

	return &Service{
		secretKey:     []byte(secretKey),
		refreshKey:    []byte(refreshKey),
		tokenExpiry:   expiry,
		refreshExpiry: refreshExpiry,
	}
}

// getSecretKey is a utility function that can be used when no secret key is provided
func getSecretKey() string {
	// Default secret key for development environments - do not use in production
	return "character-app-development-secret-key"
}

// GenerateToken creates a new JWT token
func (s *Service) GenerateToken(userID uint, email string, role Role, permissions []Permission) (string, error) {
	claims := JWTClaims{
		UserID:      userID,
		Email:       email,
		Role:        role,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.tokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "character-app",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secretKey)
}

// Simplified version for backward compatibility
func (s *Service) GenerateTokenSimple(userID uint, email string) (string, error) {
	return s.GenerateToken(userID, email, RoleUser, GetRolePermissions(RoleUser))
}

// GenerateRefreshToken creates a new JWT refresh token
func (s *Service) GenerateRefreshToken(userID uint) (string, error) {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.refreshExpiry)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Subject:   fmt.Sprintf("%d", userID),
		Issuer:    "character-app",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.refreshKey)
}

// ValidateToken validates a JWT token and returns the claims
func (s *Service) ValidateToken(tokenString string) (*JWTClaims, error) {
	if tokenString == "" {
		return nil, ErrTokenEmpty
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSigningKey, token.Header["alg"])
		}
		return s.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) || errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, ErrInvalidClaims
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token and returns the user ID
func (s *Service) ValidateRefreshToken(tokenString string) (uint, error) {
	if tokenString == "" {
		return 0, ErrTokenEmpty
	}

	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSigningKey, token.Header["alg"])
		}
		return s.refreshKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) || errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return 0, ErrExpiredToken
		}
		return 0, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !token.Valid {
		return 0, ErrInvalidToken
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok {
		return 0, ErrInvalidClaims
	}

	var userID uint
	if _, err := fmt.Sscanf(claims.Subject, "%d", &userID); err != nil {
		return 0, fmt.Errorf("%w: invalid subject format", ErrInvalidClaims)
	}

	return userID, nil
}

// GetRolePermissions returns the default permissions for a role
func GetRolePermissions(role Role) []Permission {
	switch role {
	case RoleAdmin:
		return []Permission{
			PermReadCharacter,
			PermWriteCharacter,
			PermDeleteCharacter,
			PermManageUsers,
			PermAccessAnalytics,
			PermManageSystem,
			PermAccessAudio,
		}
	case RoleUser:
		return []Permission{
			PermReadCharacter,
			PermWriteCharacter,
			PermAccessAudio,
		}
	case RoleGuest:
		return []Permission{
			PermReadCharacter,
		}
	case RoleSystem:
		return []Permission{
			PermManageSystem,
			PermAccessAnalytics,
		}
	default:
		return []Permission{}
	}
}

// HasRole checks if a user has a specific role
func (c *JWTClaims) HasRole(role Role) bool {
	return c.Role == role
}

// HasPermission checks if a user has a specific permission
func (c *JWTClaims) HasPermission(permission Permission) bool {
	for _, p := range c.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// HasAnyPermission checks if a user has any of the given permissions
func (c *JWTClaims) HasAnyPermission(permissions ...Permission) bool {
	for _, requiredPerm := range permissions {
		if c.HasPermission(requiredPerm) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if a user has all of the given permissions
func (c *JWTClaims) HasAllPermissions(permissions ...Permission) bool {
	for _, requiredPerm := range permissions {
		if !c.HasPermission(requiredPerm) {
			return false
		}
	}
	return true
}

// For backward compatibility
// These functions should be avoided in new code
func GenerateToken(userID uint, email string) (string, error) {
	s := NewService("", 0)
	return s.GenerateTokenSimple(userID, email)
}

func ValidateToken(tokenString string) (*JWTClaims, error) {
	s := NewService("", 0)
	return s.ValidateToken(tokenString)
}
