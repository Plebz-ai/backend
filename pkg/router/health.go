package router

import (
	"os"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

// setupHealthRoutes registers health check endpoints
func (r *Router) setupHealthRoutes() {
	healthHandler := func(c *gin.Context) {
		// Check database connection
		dbStatus := "ok"
		if err := r.Container.DB.Exec("SELECT 1").Error; err != nil {
			dbStatus = err.Error()
			r.Logger.Error("Database health check failed", "error", err)
		}

		// Get count of active connections
		activeConnections := len(r.Hub.GetActiveConnections())

		// Get memory stats
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		// Prepare response
		c.JSON(200, gin.H{
			"status":    "ok",
			"version":   os.Getenv("APP_VERSION"),
			"timestamp": time.Now().Format(time.RFC3339),
			"components": gin.H{
				"database": dbStatus,
				"websocket": gin.H{
					"status":             "ok",
					"active_connections": activeConnections,
				},
			},
			"memory": gin.H{
				"alloc_mb":  memStats.Alloc / 1024 / 1024,
				"sys_mb":    memStats.Sys / 1024 / 1024,
				"gc_cycles": memStats.NumGC,
			},
		})
	}

	// Register both health endpoint paths for compatibility
	r.Engine.GET("/health", healthHandler)
	r.Engine.GET("/api/health", healthHandler)
}
