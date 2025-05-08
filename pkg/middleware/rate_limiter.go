package middleware

import (
	"sync"
	"time"

	"ai-agent-character-demo/backend/pkg/errors"
	"ai-agent-character-demo/backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiterOptions configures the rate limiter
type RateLimiterOptions struct {
	// Limit defines requests per second
	Limit rate.Limit
	// Burst defines maximum burst size allowed
	Burst int
	// ExpiryDuration defines how long to keep client state in memory
	ExpiryDuration time.Duration
	// KeyFunc extracts the limiting key from a request (e.g. IP, user ID)
	KeyFunc func(*gin.Context) string
}

// DefaultRateLimiterOptions returns sensible defaults
func DefaultRateLimiterOptions() RateLimiterOptions {
	return RateLimiterOptions{
		Limit:          5,         // 5 requests per second
		Burst:          10,        // Burst of 10 requests
		ExpiryDuration: time.Hour, // Clean up limiter entries after 1 hour
		KeyFunc: func(c *gin.Context) string {
			// Default to client IP
			return c.ClientIP()
		},
	}
}

// client represents a rate limiter client
type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter implements rate limiting middleware for Gin
type RateLimiter struct {
	mu      sync.Mutex
	options RateLimiterOptions
	clients map[string]*client
	logger  *logger.Logger
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(logger *logger.Logger, options ...RateLimiterOptions) *RateLimiter {
	opts := DefaultRateLimiterOptions()
	if len(options) > 0 {
		opts = options[0]
	}

	return &RateLimiter{
		options: opts,
		clients: make(map[string]*client),
		logger:  logger,
	}
}

// Middleware returns a Gin middleware for rate limiting
func (r *RateLimiter) Middleware() gin.HandlerFunc {
	// Start cleanup goroutine
	go r.cleanup()

	return func(c *gin.Context) {
		// Get client key
		key := r.options.KeyFunc(c)

		// Get or create limiter for this client
		limiter := r.getLimiter(key)

		// Check if request is allowed
		if !limiter.Allow() {
			r.logger.Warn("Rate limit exceeded",
				"client", key,
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
			)

			c.Error(errors.NewBadRequestError("RATE_LIMIT_EXCEEDED", "Too many requests. Please try again later."))
			c.Header("Retry-After", "1") // Retry after 1 second
			c.Header("X-RateLimit-Limit", "10")
			c.Abort()
			return
		}

		c.Next()
	}
}

// getLimiter returns a rate limiter for the given key
func (r *RateLimiter) getLimiter(key string) *rate.Limiter {
	r.mu.Lock()
	defer r.mu.Unlock()

	v, exists := r.clients[key]
	if !exists {
		limiter := rate.NewLimiter(r.options.Limit, r.options.Burst)
		r.clients[key] = &client{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	// Update last seen
	v.lastSeen = time.Now()
	return v.limiter
}

// cleanup removes old entries from the clients map
func (r *RateLimiter) cleanup() {
	for {
		time.Sleep(time.Minute) // Check every minute

		r.mu.Lock()
		for k, v := range r.clients {
			if time.Since(v.lastSeen) > r.options.ExpiryDuration {
				delete(r.clients, k)
			}
		}
		r.mu.Unlock()
	}
}
