package main

import (
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"ai-agent-character-demo/backend/conversation/api"
	"ai-agent-character-demo/backend/conversation/grpc"
	"ai-agent-character-demo/backend/conversation/ws"
	"ai-agent-character-demo/backend/pkg/jwt"
	"ai-agent-character-demo/backend/shared/observability"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Observability: Tracing and Metrics
	shutdownTracing := observability.SetupTracing("conversation-service")
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

	handler := NewMessageHandlerWithDI(db)

	r := gin.Default()
	api.RegisterMessageRoutes(r, handler, jwtService)

	restPort := os.Getenv("PORT")
	if restPort == "" {
		restPort = "8084"
	}
	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "9094"
	}
	wsPort := os.Getenv("WS_PORT")
	if wsPort == "" {
		wsPort = "10094"
	}

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		log.Printf("Conversation REST API listening on :%s", restPort)
		r.Run(":" + restPort)
		wg.Done()
	}()
	go func() {
		grpc.StartGRPCServer(grpcPort)
		wg.Done()
	}()
	go func() {
		ws.StartWebSocketServer(wsPort)
		wg.Done()
	}()

	wg.Wait()
}
