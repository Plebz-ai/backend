package errors

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"ai-agent-character-demo/backend/pkg/logger"

	"github.com/gin-gonic/gin"
)

// ErrorHandler returns a middleware that catches and formats application errors
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Check if there are any errors
		if len(c.Errors) > 0 {
			// Get the first error
			err := c.Errors[0].Err

			// Convert to AppError if it's not already
			appErr := FromError(err)

			// Log the error
			logErr := c.MustGet("logger").(*logger.Logger)
			logErr.Error("Request error",
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
				"status_code", appErr.StatusCode,
				"error_code", appErr.Code,
				"message", appErr.Message,
				"details", appErr.Details,
			)

			// Respond with the error
			c.AbortWithStatusJSON(appErr.StatusCode, gin.H{
				"error": gin.H{
					"code":    appErr.Code,
					"message": appErr.Message,
					"details": appErr.Details,
				},
			})
		}
	}
}

// RecoveryWithLogger returns a middleware that recovers from any panics
// and logs the error with the request ID and user ID if available
func RecoveryWithLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				// Get the stack trace
				stack := string(debug.Stack())

				// Get the logger from the context
				l, exists := c.Get("logger")
				var log *logger.Logger
				if !exists {
					log = logger.GetGlobal()
				} else {
					log = l.(*logger.Logger)
				}

				// Log the panic with stack trace
				log.Error("Panic recovered",
					"error", r,
					"stack", stack,
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
				)

				// Respond with a 500 error
				var details interface{} = nil
				if gin.Mode() == gin.DebugMode {
					details = fmt.Sprintf("Panic: %v\n%s", r, stack)
				}

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{
						"code":    "SERVER_ERROR",
						"message": "The server encountered an unexpected error",
						"details": details,
					},
				})
			}
		}()

		c.Next()
	}
}
