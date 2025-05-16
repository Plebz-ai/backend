package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"ai-agent-character-demo/backend/shared/observability"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"ai-agent-character-demo/backend/pkg/jwt"
	"ai-agent-character-demo/backend/user/api"
)

func main() {
	// Observability: Tracing and Metrics
	shutdownTracing := observability.SetupTracing("user-service")
	defer shutdownTracing()
	_ = observability.SetupPrometheusMetrics()

	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		log.Fatal("DATABASE_DSN environment variable not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	jwtExpiry := 24 * time.Hour
	if expStr := os.Getenv("JWT_EXPIRY_HOURS"); expStr != "" {
		if exp, err := strconv.Atoi(expStr); err == nil {
			jwtExpiry = time.Duration(exp) * time.Hour
		}
	}
	jwtService := jwt.NewService(jwtSecret, jwtExpiry)

	handler := NewUserHandlerWithDI(db)

	r := gin.Default()
	api.RegisterUserRoutes(r, handler, jwtService)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}
	log.Printf("User service listening on :%s", port)
	r.Run(":" + port)
}
