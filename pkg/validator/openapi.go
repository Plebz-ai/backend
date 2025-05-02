package validator

import (
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gin-gonic/gin"
)

// OpenAPIValidator validates requests and responses against OpenAPI specification
type OpenAPIValidator struct {
	swagger    *openapi3.T
	router     routers.Router
	schemaPath string
	mutex      sync.RWMutex
}

// NewOpenAPIValidator creates a new OpenAPI validator
func NewOpenAPIValidator(schemaPath string) (*OpenAPIValidator, error) {
	swagger, err := loadOpenAPISchema(schemaPath)
	if err != nil {
		return nil, err
	}

	router, err := gorillamux.NewRouter(swagger)
	if err != nil {
		return nil, fmt.Errorf("error creating OpenAPI router: %w", err)
	}

	return &OpenAPIValidator{
		swagger:    swagger,
		router:     router,
		schemaPath: schemaPath,
	}, nil
}

// loadOpenAPISchema loads the OpenAPI schema from disk
func loadOpenAPISchema(path string) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	swagger, err := loader.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI schema from %s: %w", path, err)
	}

	if err := swagger.Validate(loader.Context); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI schema: %w", err)
	}

	return swagger, nil
}

// ReloadSchema reloads the OpenAPI schema from disk
func (v *OpenAPIValidator) ReloadSchema() error {
	swagger, err := loadOpenAPISchema(v.schemaPath)
	if err != nil {
		return err
	}

	router, err := gorillamux.NewRouter(swagger)
	if err != nil {
		return fmt.Errorf("error creating OpenAPI router: %w", err)
	}

	v.mutex.Lock()
	defer v.mutex.Unlock()

	v.swagger = swagger
	v.router = router
	return nil
}

// Middleware returns a Gin middleware function that validates requests against the OpenAPI schema
func (v *OpenAPIValidator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip validation if schema file doesn't exist (development mode)
		if _, err := os.Stat(v.schemaPath); os.IsNotExist(err) {
			c.Next()
			return
		}

		// Get the OpenAPI route for this request
		route, pathParams, err := v.router.FindRoute(c.Request)
		if err != nil {
			// Route not found in schema, continue without validation
			c.Next()
			return
		}

		// Prepare request for validation
		requestValidationInput := &openapi3filter.RequestValidationInput{
			Request:    c.Request,
			PathParams: pathParams,
			Route:      route,
			Options: &openapi3filter.Options{
				AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
			},
		}

		v.mutex.RLock()
		err = openapi3filter.ValidateRequest(c.Request.Context(), requestValidationInput)
		v.mutex.RUnlock()

		if err != nil {
			// Invalid request according to schema
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Invalid request: %v", err),
			})
			return
		}

		// Continue with request handling
		c.Next()
	}
}

// ValidateResponse validates a response against the OpenAPI schema
func (v *OpenAPIValidator) ValidateResponse(c *gin.Context, statusCode int, responseBody interface{}) error {
	// Skip validation if schema file doesn't exist (development mode)
	if _, err := os.Stat(v.schemaPath); os.IsNotExist(err) {
		return nil
	}

	// Get the OpenAPI route for this request
	_, _, err := v.router.FindRoute(c.Request)
	if err != nil {
		// Route not found in schema, continue without validation
		return nil
	}

	// TODO: Implement response validation logic
	// This would require capturing the response body and validating it against the schema
	// This is a more complex task and requires middleware that wraps the response writer

	return nil
}
