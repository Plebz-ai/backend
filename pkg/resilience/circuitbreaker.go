package resilience

import (
	"errors"
	"sync"
	"time"

	"ai-agent-character-demo/backend/pkg/logger"
)

// CircuitBreakerState represents the current state of a circuit breaker
type CircuitBreakerState string

const (
	// StateClosed means the circuit is closed and requests are allowed to pass through
	StateClosed CircuitBreakerState = "closed"
	// StateOpen means the circuit is open and requests are being short-circuited
	StateOpen CircuitBreakerState = "open"
	// StateHalfOpen means the circuit is allowing a limited number of test requests
	StateHalfOpen CircuitBreakerState = "half-open"
)

// CircuitBreaker implements the Circuit Breaker pattern
type CircuitBreaker struct {
	name             string
	state            CircuitBreakerState
	failureThreshold uint
	successThreshold uint
	timeout          time.Duration
	retryTimeout     time.Duration
	mutex            sync.RWMutex
	failureCount     uint
	successCount     uint
	lastFailureTime  time.Time
	nextAttemptTime  time.Time
	log              *logger.Logger
	// Metrics
	totalFailures     uint64
	totalSuccesses    uint64
	consecutiveErrors uint64
	totalRequests     uint64
	openCircuitCount  uint64
}

// CircuitBreakerConfig holds configuration for a circuit breaker
type CircuitBreakerConfig struct {
	Name             string
	FailureThreshold uint
	SuccessThreshold uint
	Timeout          time.Duration
	RetryTimeout     time.Duration
}

// DefaultCircuitBreakerConfig returns a default circuit breaker configuration
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:             name,
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          10 * time.Second,
		RetryTimeout:     60 * time.Second,
	}
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig, log *logger.Logger) *CircuitBreaker {
	return &CircuitBreaker{
		name:             config.Name,
		state:            StateClosed,
		failureThreshold: config.FailureThreshold,
		successThreshold: config.SuccessThreshold,
		timeout:          config.Timeout,
		retryTimeout:     config.RetryTimeout,
		failureCount:     0,
		successCount:     0,
		log:              log,
	}
}

// Execute runs a function through the circuit breaker
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.allowRequest() {
		cb.log.Warn("Circuit breaker preventing request",
			"name", cb.name,
			"state", string(cb.state),
		)
		return errors.New("circuit open")
	}

	cb.incrementRequests()

	// Start timer for the operation
	startTime := time.Now()

	// Execute the request
	err := fn()

	// Track the result
	if err != nil {
		cb.recordFailure()
		cb.log.Warn("Circuit breaker recorded failure",
			"name", cb.name,
			"error", err.Error(),
			"duration", time.Since(startTime).String(),
		)
		return err
	}

	cb.recordSuccess()
	cb.log.Debug("Circuit breaker recorded success",
		"name", cb.name,
		"duration", time.Since(startTime).String(),
	)

	return nil
}

// allowRequest checks if a request should be allowed to proceed
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mutex.RLock()
	state := cb.state
	nextAttemptTime := cb.nextAttemptTime
	cb.mutex.RUnlock()

	switch state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if retry timeout has expired
		now := time.Now()
		if now.After(nextAttemptTime) {
			// Try transitioning to half-open
			cb.mutex.Lock()
			defer cb.mutex.Unlock()

			// Double-check after acquiring lock
			if cb.state == StateOpen && time.Now().After(cb.nextAttemptTime) {
				cb.toHalfOpen()
				return true
			}
		}
		return false

	case StateHalfOpen:
		// Allow limited traffic in half-open state
		cb.mutex.RLock()
		defer cb.mutex.RUnlock()
		return cb.successCount < cb.successThreshold
	}

	return false
}

// recordSuccess records a successful request
func (cb *CircuitBreaker) recordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.totalSuccesses++

	switch cb.state {
	case StateClosed:
		cb.failureCount = 0

	case StateHalfOpen:
		cb.successCount++
		// If we've reached the success threshold, transition to closed
		if cb.successCount >= cb.successThreshold {
			cb.toClosed()
		}
	}
}

// recordFailure records a failed request
func (cb *CircuitBreaker) recordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.totalFailures++
	cb.consecutiveErrors++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		// If we've reached the failure threshold, transition to open
		if cb.failureCount >= cb.failureThreshold {
			cb.toOpen()
		}

	case StateHalfOpen:
		// Any failure in half-open state should transition back to open
		cb.toOpen()
	}
}

// toOpen transitions the circuit breaker to the open state
func (cb *CircuitBreaker) toOpen() {
	cb.state = StateOpen
	cb.openCircuitCount++
	cb.nextAttemptTime = time.Now().Add(cb.retryTimeout)

	cb.log.Info("Circuit breaker opened",
		"name", cb.name,
		"failures", cb.failureCount,
		"nextAttempt", cb.nextAttemptTime.Format(time.RFC3339),
	)
}

// toHalfOpen transitions the circuit breaker to the half-open state
func (cb *CircuitBreaker) toHalfOpen() {
	cb.state = StateHalfOpen
	cb.successCount = 0

	cb.log.Info("Circuit breaker half-open", "name", cb.name)
}

// toClosed transitions the circuit breaker to the closed state
func (cb *CircuitBreaker) toClosed() {
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.consecutiveErrors = 0

	cb.log.Info("Circuit breaker closed", "name", cb.name)
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return cb.state
}

// incrementRequests increments the total request counter
func (cb *CircuitBreaker) incrementRequests() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.totalRequests++
}

// GetMetrics returns the current metrics of the circuit breaker
func (cb *CircuitBreaker) GetMetrics() map[string]interface{} {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return map[string]interface{}{
		"name":               cb.name,
		"state":              string(cb.state),
		"total_requests":     cb.totalRequests,
		"total_failures":     cb.totalFailures,
		"total_successes":    cb.totalSuccesses,
		"consecutive_errors": cb.consecutiveErrors,
		"open_circuit_count": cb.openCircuitCount,
		"last_failure_time":  cb.lastFailureTime,
	}
}
