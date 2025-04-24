package jwt

import (
	"time"
)

// Service is a wrapper for JWT operations
type Service struct {
	secretKey string
	expiry    time.Duration
}

// NewService creates a new JWT service
func NewService(secretKey string, expiry time.Duration) *Service {
	if secretKey == "" {
		secretKey = getSecretKey()
	}

	if expiry == 0 {
		expiry = 24 * time.Hour // Default to 24 hours
	}

	return &Service{
		secretKey: secretKey,
		expiry:    expiry,
	}
}

// GenerateToken generates a JWT token for a user
func (s *Service) GenerateToken(userID uint, email string) (string, error) {
	return GenerateToken(userID, email)
}

// ValidateToken validates a JWT token and returns the claims
func (s *Service) ValidateToken(tokenString string) (*JWTClaims, error) {
	return ValidateToken(tokenString)
}
