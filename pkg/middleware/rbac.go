package middleware

import (
	"ai-agent-character-demo/backend/pkg/errors"
	"ai-agent-character-demo/backend/pkg/jwt"
	"ai-agent-character-demo/backend/pkg/logger"

	"github.com/gin-gonic/gin"
)

// RequireRole returns a middleware that requires the user to have a specific role
func RequireRole(role jwt.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get claims from context (set by JWTAuthMiddleware)
		claims, exists := c.Get("claims")
		if !exists {
			c.Error(errors.NewUnauthorizedError("AUTH_REQUIRED", "Authentication required"))
			c.Abort()
			return
		}

		// Cast to JWTClaims
		jwtClaims, ok := claims.(*jwt.JWTClaims)
		if !ok {
			c.Error(errors.NewInternalServerError("INVALID_CLAIMS", "Invalid JWT claims format"))
			c.Abort()
			return
		}

		// Check if user has the required role
		if !jwtClaims.HasRole(role) {
			c.Error(errors.NewForbiddenError("INSUFFICIENT_ROLE", "Your role does not allow this operation"))
			c.Abort()
			return
		}

		// User has the required role, continue
		c.Next()
	}
}

// RequirePermission returns a middleware that requires the user to have a specific permission
func RequirePermission(permission jwt.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get claims from context (set by JWTAuthMiddleware)
		claims, exists := c.Get("claims")
		if !exists {
			c.Error(errors.NewUnauthorizedError("AUTH_REQUIRED", "Authentication required"))
			c.Abort()
			return
		}

		// Cast to JWTClaims
		jwtClaims, ok := claims.(*jwt.JWTClaims)
		if !ok {
			c.Error(errors.NewInternalServerError("INVALID_CLAIMS", "Invalid JWT claims format"))
			c.Abort()
			return
		}

		// Check if user has the required permission
		if !jwtClaims.HasPermission(permission) {
			c.Error(errors.NewForbiddenError("INSUFFICIENT_PERMISSION", "You don't have permission to perform this operation"))
			c.Abort()
			return
		}

		// User has the required permission, continue
		c.Next()
	}
}

// RequireAnyRole returns middleware that requires the user to have at least one of the specified roles
func RequireAnyRole(roles ...jwt.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get claims from context (set by JWTAuthMiddleware)
		claims, exists := c.Get("claims")
		if !exists {
			c.Error(errors.NewUnauthorizedError("AUTH_REQUIRED", "Authentication required"))
			c.Abort()
			return
		}

		// Cast to JWTClaims
		jwtClaims, ok := claims.(*jwt.JWTClaims)
		if !ok {
			c.Error(errors.NewInternalServerError("INVALID_CLAIMS", "Invalid JWT claims format"))
			c.Abort()
			return
		}

		// Check if user has at least one of the required roles
		for _, role := range roles {
			if jwtClaims.HasRole(role) {
				// User has at least one required role, continue
				c.Next()
				return
			}
		}

		// User has none of the required roles
		c.Error(errors.NewForbiddenError("INSUFFICIENT_ROLE", "Your role does not allow this operation"))
		c.Abort()
	}
}

// JWTAuthMiddleware checks that the request has a valid JWT and adds claims to the context
func JWTAuthMiddleware(jwtService *jwt.Service, logger *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.Error(errors.NewUnauthorizedError("AUTH_REQUIRED", "Authorization header is required"))
			c.Abort()
			return
		}

		// Strip "Bearer " prefix if present
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}

		// Validate token
		claims, err := jwtService.ValidateToken(token)
		if err != nil {
			logger.Warn("Invalid JWT token", "error", err.Error())
			c.Error(errors.NewUnauthorizedError("INVALID_TOKEN", "Invalid or expired token"))
			c.Abort()
			return
		}

		// Add claims to context
		c.Set("claims", claims)
		c.Set("userId", claims.UserID)
		c.Set("userID", claims.UserID)
		c.Set("userRole", claims.Role)

		c.Next()
	}
}
