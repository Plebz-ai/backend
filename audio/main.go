package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"ai-agent-character-demo/backend/audio/api"
	"ai-agent-character-demo/backend/pkg/jwt"
)

func main() {
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

	handler := NewAudioHandlerWithDI(db)

	r := gin.Default()
	api.RegisterAudioRoutes(r, handler, jwtService)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8085"
	}
	log.Printf("Audio service listening on :%s", port)
	r.Run(":" + port)
}
