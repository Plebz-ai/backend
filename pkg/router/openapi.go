package router

import (
	"ai-agent-character-demo/backend/pkg/validator"
	"os"
	"path/filepath"
)

// AddOpenAPIValidation adds OpenAPI validation middleware to the router
func (r *Router) AddOpenAPIValidation(schemaPath string) {
	// Check if schema file exists
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		r.Logger.Warn("OpenAPI schema file not found, skipping validation", "path", schemaPath)
		return
	}

	// Initialize OpenAPI validator
	v, err := validator.NewOpenAPIValidator(schemaPath)
	if err != nil {
		r.Logger.Error("Failed to initialize OpenAPI validator", "error", err)
		return
	}

	// Add validator middleware
	r.Engine.Use(v.Middleware())
	r.Logger.Info("OpenAPI validation enabled", "schema", schemaPath)

	// Serve OpenAPI schema file
	schemaDir := filepath.Dir(schemaPath)
	schemaFile := filepath.Base(schemaPath)
	r.Engine.Static("/api/docs", schemaDir)
	r.Logger.Info("OpenAPI schema available at", "url", "/api/docs/"+schemaFile)

	// Setup Swagger UI if available
	swaggerUIPath := os.Getenv("SWAGGER_UI_PATH")
	if swaggerUIPath != "" && fileExists(swaggerUIPath) {
		r.Engine.Static("/swagger-ui", swaggerUIPath)
		r.Logger.Info("Swagger UI available at", "url", "/swagger-ui/")
	}
}

// fileExists checks if a file exists and is not a directory
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
