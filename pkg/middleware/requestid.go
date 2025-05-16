package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Key types for context values
type contextKey string

const (
	// RequestIDKey is the key for request ID values in contexts
	RequestIDKey contextKey = "requestID"
	// UserIDKey is the key for user ID values in contexts
	UserIDKey contextKey = "userID"
	// TraceIDKey is the key for trace ID values in contexts
	TraceIDKey contextKey = "traceID"
)

// RequestIDMiddleware adds a unique request ID to each request
// and sets it in both the context and response headers
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if request already has an ID from upstream service
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Set the request ID in the context
		ctx := context.WithValue(c.Request.Context(), RequestIDKey, requestID)
		c.Request = c.Request.WithContext(ctx)

		// Set the request ID in the response headers
		c.Header("X-Request-ID", requestID)
		c.Set("requestID", requestID)

		// Process the request
		c.Next()
	}
}

// ContextPropagationMiddleware ensures that certain context values are propagated
// throughout the request lifecycle and passed on to downstream services
func ContextPropagationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Add trace context from incoming headers if present
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = uuid.New().String()
		}
		ctx := context.WithValue(c.Request.Context(), TraceIDKey, traceID)
		c.Request = c.Request.WithContext(ctx)
		c.Set("traceID", traceID)
		c.Header("X-Trace-ID", traceID)

		// Add correlation ID for tracking request flows across services
		correlationID := c.GetHeader("X-Correlation-ID")
		if correlationID == "" {
			// Use the request ID as correlation ID if not provided
			correlationID = c.GetString("requestID")
		}
		c.Header("X-Correlation-ID", correlationID)
		c.Set("correlationID", correlationID)

		c.Next()
	}
}

// WithRequestContext adds standard context values to a context for downstream operations
func WithRequestContext(parent context.Context, c *gin.Context) context.Context {
	ctx := parent

	// Add request ID
	if requestID, exists := c.Get("requestID"); exists {
		ctx = context.WithValue(ctx, RequestIDKey, requestID)
	}

	// Add user ID if authenticated
	if userID, exists := c.Get("userID"); exists {
		ctx = context.WithValue(ctx, UserIDKey, userID)
	}

	// Add trace ID
	if traceID, exists := c.Get("traceID"); exists {
		ctx = context.WithValue(ctx, TraceIDKey, traceID)
	}

	return ctx
}

// GetRequestID extracts the request ID from a context
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}

	return ""
}

// GetUserID extracts the user ID from a context
func GetUserID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}

	return ""
}

// GetTraceID extracts the trace ID from a context
func GetTraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}

	return ""
}
