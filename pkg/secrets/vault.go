package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"ai-agent-character-demo/backend/pkg/logger"

	vault "github.com/hashicorp/vault/api"
)

// Common errors
var (
	ErrSecretNotFound = errors.New("secret not found")
	ErrVaultDisabled  = errors.New("vault integration is disabled")
	ErrNoVaultToken   = errors.New("no vault token provided")
	ErrNoVaultAddress = errors.New("no vault address provided")
)

// VaultConfig holds configuration for Vault client
type VaultConfig struct {
	Address     string
	Token       string
	Namespace   string
	Timeout     time.Duration
	MaxRetries  int
	SecretsPath string
	Enabled     bool
}

// VaultManager manages secrets with HashiCorp Vault
type VaultManager struct {
	client   *vault.Client
	config   VaultConfig
	cache    map[string]string
	mu       sync.RWMutex
	log      *logger.Logger
	cacheTTL time.Duration
}

// NewVaultManager creates a new Vault manager instance
func NewVaultManager(log *logger.Logger) (*VaultManager, error) {
	// Load configuration from environment variables
	config := VaultConfig{
		Address:     os.Getenv("VAULT_ADDR"),
		Token:       os.Getenv("VAULT_TOKEN"),
		Namespace:   os.Getenv("VAULT_NAMESPACE"),
		SecretsPath: os.Getenv("VAULT_SECRETS_PATH"),
		Enabled:     true,
		Timeout:     10 * time.Second,
		MaxRetries:  3,
	}

	// Check if Vault is enabled
	if enabled := os.Getenv("VAULT_ENABLED"); enabled != "" {
		config.Enabled = enabled == "true" || enabled == "1" || enabled == "yes"
	}

	// If Vault is disabled, return a manager without a client
	if !config.Enabled {
		return &VaultManager{
			config:   config,
			cache:    make(map[string]string),
			log:      log,
			cacheTTL: 5 * time.Minute,
		}, nil
	}

	// Validate required configuration
	if config.Address == "" {
		return nil, ErrNoVaultAddress
	}
	if config.Token == "" {
		return nil, ErrNoVaultToken
	}
	if config.SecretsPath == "" {
		// Default secrets path if not specified
		config.SecretsPath = "secret/data/character-app"
	}

	// Create Vault client configuration
	vaultConfig := vault.DefaultConfig()
	vaultConfig.Address = config.Address
	vaultConfig.Timeout = config.Timeout
	vaultConfig.MaxRetries = config.MaxRetries

	// Create Vault client
	client, err := vault.NewClient(vaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Set token and namespace
	client.SetToken(config.Token)
	if config.Namespace != "" {
		client.SetNamespace(config.Namespace)
	}

	manager := &VaultManager{
		client:   client,
		config:   config,
		cache:    make(map[string]string),
		log:      log,
		cacheTTL: 5 * time.Minute,
	}

	// Start cache cleanup goroutine
	go manager.cleanupCache()

	return manager, nil
}

// GetSecret retrieves a secret from Vault, with fallback to environment variable
func (m *VaultManager) GetSecret(ctx context.Context, key string) (string, error) {
	// Check cache first
	m.mu.RLock()
	cachedValue, found := m.cache[key]
	m.mu.RUnlock()

	if found {
		return cachedValue, nil
	}

	// If Vault is disabled, fall back to environment variables only
	if !m.config.Enabled {
		return m.getFromEnvironment(key)
	}

	// Try to get secret from Vault
	value, err := m.getFromVault(ctx, key)
	if err != nil {
		// If not found in Vault, try environment variable
		if errors.Is(err, ErrSecretNotFound) {
			m.log.Warn("Secret not found in Vault, falling back to environment", "key", key)
			return m.getFromEnvironment(key)
		}
		return "", err
	}

	// Cache the value
	m.cacheSecret(key, value)

	return value, nil
}

// GetSecretWithDefault retrieves a secret with a default value if not found
func (m *VaultManager) GetSecretWithDefault(ctx context.Context, key, defaultValue string) string {
	value, err := m.GetSecret(ctx, key)
	if err != nil {
		m.log.Warn("Failed to get secret, using default value",
			"key", key,
			"error", err.Error(),
		)
		return defaultValue
	}
	return value
}

// getFromVault retrieves a secret directly from Vault
func (m *VaultManager) getFromVault(ctx context.Context, key string) (string, error) {
	// Create path where the secret is stored
	// Format: secret/data/app-name/key
	path := m.config.SecretsPath

	// Read secret from Vault
	secret, err := m.client.KVv2("secret").Get(ctx, path)
	if err != nil {
		m.log.Error("Failed to read secret from Vault",
			"path", path,
			"error", err.Error(),
		)
		return "", fmt.Errorf("failed to read secret: %w", err)
	}

	// Extract value from secret data
	if secret == nil || secret.Data == nil {
		return "", ErrSecretNotFound
	}

	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return "", ErrSecretNotFound
	}

	value, ok := data[key].(string)
	if !ok {
		return "", ErrSecretNotFound
	}

	return value, nil
}

// getFromEnvironment retrieves a secret from environment variables
func (m *VaultManager) getFromEnvironment(key string) (string, error) {
	// Convert key format from snake_case or kebab-case to uppercase with underscores
	envKey := strings.ToUpper(strings.Replace(strings.Replace(key, "-", "_", -1), ".", "_", -1))

	value := os.Getenv(envKey)
	if value == "" {
		return "", ErrSecretNotFound
	}

	// Cache the value
	m.cacheSecret(key, value)

	return value, nil
}

// cacheSecret adds a secret to the cache
func (m *VaultManager) cacheSecret(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache[key] = value
}

// cleanupCache periodically clears the secret cache to ensure freshness
func (m *VaultManager) cleanupCache() {
	ticker := time.NewTicker(m.cacheTTL)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		m.cache = make(map[string]string)
		m.mu.Unlock()

		m.log.Debug("Secret cache cleared")
	}
}
