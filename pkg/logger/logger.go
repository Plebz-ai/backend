package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"
)

// LogLevel type alias for log level constants
type LogLevel string

// Log levels
const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// Config contains logger configuration options
type Config struct {
	// Level is the minimum level to log
	Level string
	// JSON enables JSON formatting instead of text
	JSON bool
	// Output is where logs will be written (defaults to os.Stderr)
	Output io.Writer
	// AddSource adds source code information to logs
	AddSource bool
}

// DefaultConfig returns a default logger configuration
func DefaultConfig() Config {
	return Config{
		Level:     "info",
		JSON:      true, // Default to JSON for production
		Output:    os.Stderr,
		AddSource: false,
	}
}

// Logger wraps slog for structured logging
type Logger struct {
	*slog.Logger
	config Config
}

// global is the package-level logger instance
var global *Logger

// New creates a new logger with the given configuration
func New(config Config) *Logger {
	var handler slog.Handler

	// Set log level
	var level slog.Level
	switch LogLevel(config.Level) {
	case LevelDebug:
		level = slog.LevelDebug
	case LevelWarn:
		level = slog.LevelWarn
	case LevelError:
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Configure handler based on format
	if config.JSON {
		handler = slog.NewJSONHandler(config.Output, &slog.HandlerOptions{
			Level:     level,
			AddSource: config.AddSource,
		})
	} else {
		handler = slog.NewTextHandler(config.Output, &slog.HandlerOptions{
			Level:     level,
			AddSource: config.AddSource,
		})
	}

	logger := &Logger{
		Logger: slog.New(handler),
		config: config,
	}

	// Set this as global if no global logger exists yet
	if global == nil {
		global = logger
	}

	return logger
}

// SetGlobal sets the global logger instance
func SetGlobal(logger *Logger) {
	global = logger
}

// GetGlobal returns the global logger instance
func GetGlobal() *Logger {
	return global
}

// LogError logs an error with context information
func (l *Logger) LogError(err error, msg string, args ...any) {
	l.Error(msg, append([]any{"error", err.Error()}, args...)...)
}

// WithRequestID adds a request ID to the logger's context
func (l *Logger) WithRequestID(requestID string) *Logger {
	if requestID == "" {
		return l
	}
	return &Logger{Logger: l.With("request_id", requestID)}
}

// WithUserID adds a user ID to the logger's context
func (l *Logger) WithUserID(userID string) *Logger {
	if userID == "" {
		return l
	}
	return &Logger{Logger: l.With("user_id", userID)}
}

// WithContext adds fields from the context to the logger
func (l *Logger) WithContext(ctx context.Context) *Logger {
	// You can extract values from the context if needed
	return l
}

// LogRequest logs details about an HTTP request
func (l *Logger) LogRequest(method, path string, status int, latency time.Duration) {
	l.Info("request completed",
		"method", method,
		"path", path,
		"status", status,
		"latency_ms", latency.Milliseconds(),
	)
}
