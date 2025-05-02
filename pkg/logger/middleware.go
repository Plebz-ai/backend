package logger

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Middleware returns a Gin middleware function that logs requests
func Middleware(logger *Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate a request ID if one doesn't exist
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
			c.Header("X-Request-ID", requestID)
		}

		// Get user ID if available
		userID, _ := c.Get("userId")
		var userIDStr string
		if userID != nil {
			userIDStr = fmt.Sprintf("%v", userID)
		}

		// Create a request-scoped logger
		reqLogger := logger.WithRequestID(requestID)
		if userIDStr != "" {
			reqLogger = reqLogger.WithUserID(userIDStr)
		}

		// Store the logger in the context
		c.Set("logger", reqLogger)

		// Record start time
		start := time.Now()

		// Process request
		c.Next()

		// Log request details
		latency := time.Since(start)
		status := c.Writer.Status()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Log the request
		reqLogger.LogRequest(method, path, status, latency)

		// Log errors if any
		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				reqLogger.LogError(err.Err, "request error",
					"method", method,
					"path", path,
					"error_type", err.Type,
				)
			}
		}
	}
}
