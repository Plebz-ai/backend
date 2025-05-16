package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"ai-agent-character-demo/backend/pkg/logger"
)

// Status represents the health status of a component
type Status string

const (
	// StatusUp indicates a component is working correctly
	StatusUp Status = "up"
	// StatusDown indicates a component is not working
	StatusDown Status = "down"
	// StatusDegraded indicates a component is working but with reduced functionality
	StatusDegraded Status = "degraded"
)

// Component represents a system component that can be health-checked
type Component struct {
	Name        string    `json:"name"`
	Status      Status    `json:"status"`
	Description string    `json:"description,omitempty"`
	Error       string    `json:"error,omitempty"`
	LastChecked time.Time `json:"last_checked"`
}

// Check represents a health check function
type Check func() (Status, string, error)

// Checker manages health checks for the system
type Checker struct {
	checks      map[string]Check
	components  map[string]*Component
	checkPeriod time.Duration
	mutex       sync.RWMutex
	log         *logger.Logger
}

// NewChecker creates a new health checker
func NewChecker(log *logger.Logger, checkPeriod time.Duration) *Checker {
	checker := &Checker{
		checks:      make(map[string]Check),
		components:  make(map[string]*Component),
		checkPeriod: checkPeriod,
		log:         log,
	}

	// Register built-in checks
	checker.RegisterCheck("self", func() (Status, string, error) {
		return StatusUp, "Health checker is running", nil
	})

	return checker
}

// RegisterCheck registers a new health check
func (c *Checker) RegisterCheck(name string, check Check) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.checks[name] = check
	c.components[name] = &Component{
		Name:        name,
		Status:      StatusDown,
		Description: "Not checked yet",
		LastChecked: time.Time{},
	}
}

// RunChecks executes all registered health checks
func (c *Checker) RunChecks() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for name, check := range c.checks {
		status, description, err := check()

		component := c.components[name]
		component.Status = status
		component.Description = description
		component.LastChecked = time.Now()

		if err != nil {
			component.Error = err.Error()
			c.log.Error("Health check failed",
				"component", name,
				"status", string(status),
				"error", err.Error(),
			)
		} else {
			component.Error = ""
			c.log.Debug("Health check completed",
				"component", name,
				"status", string(status),
			)
		}
	}
}

// Start begins periodic health checks
func (c *Checker) Start() {
	go func() {
		// Run checks immediately at startup
		c.RunChecks()

		// Then run periodically
		ticker := time.NewTicker(c.checkPeriod)
		defer ticker.Stop()

		for range ticker.C {
			c.RunChecks()
		}
	}()
}

// GetStatus returns the current health status
func (c *Checker) GetStatus() map[string]*Component {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// Create a copy to avoid race conditions
	result := make(map[string]*Component, len(c.components))
	for k, v := range c.components {
		componentCopy := *v
		result[k] = &componentCopy
	}

	return result
}

// IsSystemHealthy returns true if all critical components are up
func (c *Checker) IsSystemHealthy() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// Loop through all components to check if any critical ones are down
	for _, component := range c.components {
		// Check only critical components (customize this logic)
		if component.Status == StatusDown && c.isCriticalComponent(component.Name) {
			return false
		}
	}

	return true
}

// isCriticalComponent determines whether a component is critical for system operation
func (c *Checker) isCriticalComponent(name string) bool {
	// Define which components are critical
	criticalComponents := map[string]bool{
		"database": true,
		// Add other critical components as needed
	}

	return criticalComponents[name]
}

// HTTPHandler returns an HTTP handler for health checks
func (c *Checker) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := c.GetStatus()

		w.Header().Set("Content-Type", "application/json")

		// If system is unhealthy, return 503 Service Unavailable
		if !c.IsSystemHealthy() {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		response := map[string]interface{}{
			"status":     "ok",
			"timestamp":  time.Now(),
			"components": status,
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			c.log.Error("Failed to encode health check response", "error", err.Error())
		}
	}
}

// RegisterDatabaseCheck registers a database health check
func (c *Checker) RegisterDatabaseCheck(checkFunc func() error) {
	c.RegisterCheck("database", func() (Status, string, error) {
		if err := checkFunc(); err != nil {
			return StatusDown, "Database connection failed", err
		}
		return StatusUp, "Database connection is established", nil
	})
}

// RegisterAPICheck registers an API health check
func (c *Checker) RegisterAPICheck(name, endpoint string, client *http.Client) {
	if client == nil {
		client = http.DefaultClient
	}

	c.RegisterCheck(fmt.Sprintf("api-%s", name), func() (Status, string, error) {
		start := time.Now()
		resp, err := client.Get(endpoint)
		elapsed := time.Since(start)

		if err != nil {
			return StatusDown, "API request failed", err
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return StatusDegraded, fmt.Sprintf("API returned status %d", resp.StatusCode),
				fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		return StatusUp, fmt.Sprintf("API is responding (latency: %s)", elapsed), nil
	})
}
