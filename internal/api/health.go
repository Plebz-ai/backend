package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler handles health check endpoints
// providing a receiver for health route methods
type Handler struct{}

// HealthResponse represents the health check response structure
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

// HealthHandler returns a simple health check handler
func (h *Handler) HealthHandler(c *gin.Context) {
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   "1.0.0", // Replace with actual version if available
	}
	c.JSON(http.StatusOK, response)
}

// RegisterHealthRoutes registers health check related routes
func (h *Handler) RegisterHealthRoutes(router *gin.RouterGroup) {
	router.GET("/health", h.HealthHandler)
}
