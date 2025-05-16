package api

import (
	"net/http"
	"strings"

	"ai-agent-character-demo/backend/pkg/jwt"

	"github.com/gin-gonic/gin"
)

func JWTAuthMiddleware(jwtService *jwt.Service, requiredRoles ...jwt.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid Authorization header"})
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		claims, err := jwtService.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}
		// Role check
		if len(requiredRoles) > 0 {
			allowed := false
			for _, role := range requiredRoles {
				if claims.HasRole(role) {
					allowed = true
					break
				}
			}
			if !allowed {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Insufficient role"})
				return
			}
		}
		c.Set("user", claims)
		c.Next()
	}
}

func RequirePermission(permission jwt.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get("user")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "No user claims in context"})
			return
		}
		userClaims, ok := claims.(*jwt.JWTClaims)
		if !ok || !userClaims.HasPermission(permission) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
			return
		}
		c.Next()
	}
} 