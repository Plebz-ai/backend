package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"
)

// Logger is a wrapper around slog to provide structured logging
type Logger struct {
	*slog.Logger
}

// Config holds the configuration for the logger
type Config struct {
	// Level defines the minimum log level to output (debug, info, warn, error)
	Level string
	// JSON specifies whether log output should be formatted as JSON
	JSON bool
	// Output is where the log output is written
	Output io.Writer
}

// DefaultConfig returns a default configuration for the logger
func DefaultConfig() Config {
	return Config{
		Level:  "info",
		JSON:   true,
		Output: os.Stdout,
	}
}

// New creates a new structured logger
func New(cfg Config) *Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var handler slog.Handler
	if cfg.JSON {
		handler = slog.NewJSONHandler(cfg.Output, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		handler = slog.NewTextHandler(cfg.Output, &slog.HandlerOptions{
			Level: level,
		})
	}

	logger := slog.New(handler)
	return &Logger{Logger: logger}
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
	// You can extract values from the context here if needed
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

// LogError logs an error with the given message and additional fields
func (l *Logger) LogError(err error, msg string, args ...any) {
	if err != nil {
		args = append(args, "error", err.Error())
	}
	l.Error(msg, args...)
}

// Global instance for easy access
var global *Logger

// Initialize the global logger with default configuration
func init() {
	global = New(DefaultConfig())
}

// Global returns the global logger instance
func Global() *Logger {
	return global
}

// SetGlobal sets the global logger instance
func SetGlobal(l *Logger) {
	global = l
}
