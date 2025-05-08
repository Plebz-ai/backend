package errors

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"ai-agent-character-demo/backend/pkg/logger"

	"github.com/gin-gonic/gin"
)

// SimpleRecovery returns a middleware that recovers from any panics with minimal logging
// This is a simpler alternative to RecoveryWithLogger in middleware.go
func SimpleRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Get stack trace
				stack := string(debug.Stack())

				// Log the error with stack trace
				log := logger.GetGlobal()
				log.Error("Panic recovered",
					"error", fmt.Sprintf("%v", err),
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
					"stack", stack,
				)

				// Create a standard response
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Internal server error",
					"code":  "SERVER_PANIC",
				})
			}
		}()
		c.Next()
	}
}
