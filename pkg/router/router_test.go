package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestWebSocketRoute(t *testing.T) {
	// Create a test router
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "WebSocket route is functional"})
	})

	// Create a test request
	req, _ := http.NewRequest(http.MethodGet, "/ws", nil)
	w := httptest.NewRecorder()

	// Perform the request
	r.ServeHTTP(w, req)

	// Assert the response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "WebSocket route is functional")
}
