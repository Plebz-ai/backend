package secrets

import (
	"context"
	"sync"

	"ai-agent-character-demo/backend/pkg/logger"
)

// Manager provides access to secrets from various sources
type Manager interface {
	// GetSecret retrieves a secret by key
	GetSecret(ctx context.Context, key string) (string, error)

	// GetSecretWithDefault retrieves a secret with a default value if not found
	GetSecretWithDefault(ctx context.Context, key, defaultValue string) string
}

var (
	defaultManager Manager
	managerOnce    sync.Once
)

// Init initializes the default secrets manager
func Init(log *logger.Logger) error {
	var err error
	managerOnce.Do(func() {
		manager, initErr := NewVaultManager(log)
		if initErr != nil {
			err = initErr
			return
		}
		defaultManager = manager
	})
	return err
}

// GetSecret retrieves a secret from the default manager
func GetSecret(ctx context.Context, key string) (string, error) {
	if defaultManager == nil {
		return "", ErrManagerNotInitialized
	}
	return defaultManager.GetSecret(ctx, key)
}

// GetSecretWithDefault retrieves a secret with a default value if not found
func GetSecretWithDefault(ctx context.Context, key, defaultValue string) string {
	if defaultManager == nil {
		return defaultValue
	}
	return defaultManager.GetSecretWithDefault(ctx, key, defaultValue)
}

// Set the default secrets manager (primarily used for testing)
func SetManager(manager Manager) {
	defaultManager = manager
}

// Common errors
var (
	ErrManagerNotInitialized = NewError("secrets manager not initialized")
)

// Error represents a secrets management error
type Error string

// Error implements the error interface
func (e Error) Error() string {
	return string(e)
}

// NewError creates a new Error
func NewError(text string) Error {
	return Error(text)
}
