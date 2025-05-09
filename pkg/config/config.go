package config

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// Server configuration
	Server struct {
		Port    string
		Env     string
		Timeout time.Duration
		BaseURL string
	}

	// Database configuration
	Database struct {
		Host     string
		Port     string
		User     string
		Password string
		Name     string
		SSLMode  string
		MaxConns int
		Timeout  time.Duration
	}

	// JWT configuration
	JWT struct {
		Secret        string
		ExpiryHours   time.Duration
		RefreshSecret string
		RefreshExpiry time.Duration
	}

	// Security configuration
	Security struct {
		RateLimit       float64
		RateLimitBurst  int
		AllowedOrigins  []string
		TrustedProxies  []string
		MaxBodySize     int64
		TimestampWindow time.Duration
	}

	// Logging configuration
	Logging struct {
		Level  string
		Format string
	}

	// Feature flags
	Features struct {
		EnableAudioProcessing   bool
		EnableWebSockets        bool
		EnableHistory           bool
		EnableAnalytics         bool
		MaxMessagesPerSession   int
		MaxSessionsPerUser      int
		MaxCharactersPerUser    int
		MaxAudioChunkSize       int64
		MaxAudioDuration        time.Duration
		MaxAudioTTL             time.Duration
		DefaultAudioTTL         time.Duration
		MaxAudioChunksPerUser   int
		MaxAudioChunksPerDay    int
		AudioChunkCleanupPeriod time.Duration
	}

	// Service endpoints
	Services struct {
		AIServiceURL string
		MLServiceURL string
	}

	// Cache settings
	Cache struct {
		Enabled     bool
		TTL         time.Duration
		MaxSize     int
		PurgeWindow time.Duration
	}
}

var (
	instance *Config
	once     sync.Once
)

// New creates a new Config instance with values from environment variables
// Uses singleton pattern to ensure only one instance exists
func New() *Config {
	once.Do(func() {
		// Load .env file if exists
		godotenv.Load()

		instance = &Config{}

		// Server config
		instance.Server.Port = getEnvString("PORT", "8081")
		instance.Server.Env = getEnvString("APP_ENV", "development")
		instance.Server.Timeout = getEnvDuration("SERVER_TIMEOUT", 30*time.Second)
		instance.Server.BaseURL = getEnvString("BASE_URL", "http://localhost:"+instance.Server.Port)

		// Database config
		instance.Database.Host = getEnvString("DB_HOST", "localhost")
		instance.Database.Port = getEnvString("DB_PORT", "5432")
		instance.Database.User = getEnvString("DB_USER", "postgres")
		instance.Database.Password = getEnvString("DB_PASSWORD", "postgres")
		instance.Database.Name = getEnvString("DB_NAME", "character-demo")
		instance.Database.SSLMode = getEnvString("DB_SSL_MODE", "disable")
		instance.Database.MaxConns = getEnvInt("DB_MAX_CONNS", 20)
		instance.Database.Timeout = getEnvDuration("DB_TIMEOUT", 5*time.Second)

		// JWT config
		instance.JWT.Secret = getEnvString("JWT_SECRET", "default-jwt-secret-do-not-use-in-production")
		instance.JWT.ExpiryHours = getEnvDuration("JWT_EXPIRY", 24*time.Hour)
		instance.JWT.RefreshSecret = getEnvString("JWT_REFRESH_SECRET", "default-refresh-secret-do-not-use-in-production")
		instance.JWT.RefreshExpiry = getEnvDuration("JWT_REFRESH_EXPIRY", 7*24*time.Hour)

		// Security config
		instance.Security.RateLimit = float64(getEnvInt("RATE_LIMIT", 5))
		instance.Security.RateLimitBurst = getEnvInt("RATE_LIMIT_BURST", 10)
		instance.Security.AllowedOrigins = getEnvStringSlice("ALLOWED_ORIGINS", []string{"*"})
		instance.Security.TrustedProxies = getEnvStringSlice("TRUSTED_PROXIES", []string{"127.0.0.1"})
		instance.Security.MaxBodySize = getEnvInt64("MAX_BODY_SIZE", 10<<20) // 10MB
		instance.Security.TimestampWindow = getEnvDuration("TIMESTAMP_WINDOW", 15*time.Minute)

		// Logging config
		instance.Logging.Level = getEnvString("LOG_LEVEL", "info")
		instance.Logging.Format = getEnvString("LOG_FORMAT", "json")

		// Feature flags
		instance.Features.EnableAudioProcessing = getEnvBool("ENABLE_AUDIO_PROCESSING", true)
		instance.Features.EnableWebSockets = getEnvBool("ENABLE_WEBSOCKETS", true)
		instance.Features.EnableHistory = getEnvBool("ENABLE_HISTORY", true)
		instance.Features.EnableAnalytics = getEnvBool("ENABLE_ANALYTICS", true)
		instance.Features.MaxMessagesPerSession = getEnvInt("MAX_MESSAGES_PER_SESSION", 1000)
		instance.Features.MaxSessionsPerUser = getEnvInt("MAX_SESSIONS_PER_USER", 20)
		instance.Features.MaxCharactersPerUser = getEnvInt("MAX_CHARACTERS_PER_USER", 50)
		instance.Features.MaxAudioChunkSize = getEnvInt64("MAX_AUDIO_CHUNK_SIZE", 5<<20) // 5MB
		instance.Features.MaxAudioDuration = getEnvDuration("MAX_AUDIO_DURATION", 5*time.Minute)
		instance.Features.MaxAudioTTL = getEnvDuration("MAX_AUDIO_TTL", 7*24*time.Hour)
		instance.Features.DefaultAudioTTL = getEnvDuration("DEFAULT_AUDIO_TTL", 24*time.Hour)
		instance.Features.MaxAudioChunksPerUser = getEnvInt("MAX_AUDIO_CHUNKS_PER_USER", 100)
		instance.Features.MaxAudioChunksPerDay = getEnvInt("MAX_AUDIO_CHUNKS_PER_DAY", 25)
		instance.Features.AudioChunkCleanupPeriod = getEnvDuration("AUDIO_CHUNK_CLEANUP_PERIOD", 1*time.Hour)

		// Service endpoints
		instance.Services.AIServiceURL = getEnvString("AI_SERVICE_URL", "")
		instance.Services.MLServiceURL = getEnvString("ML_SERVICE_URL", "")

		// Cache settings
		instance.Cache.Enabled = getEnvBool("CACHE_ENABLED", true)
		instance.Cache.TTL = getEnvDuration("CACHE_TTL", 5*time.Minute)
		instance.Cache.MaxSize = getEnvInt("CACHE_MAX_SIZE", 1000)
		instance.Cache.PurgeWindow = getEnvDuration("CACHE_PURGE_WINDOW", 10*time.Minute)
	})

	return instance
}

// Get returns the singleton Config instance
func Get() *Config {
	if instance == nil {
		return New()
	}
	return instance
}

// Helper functions to read environment variables with default values

func getEnvString(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value, exists := os.LookupEnv(key); exists {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvStringSlice(key string, defaultValue []string) []string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}
