package main

import (
	"ai-agent-character-demo/backend/conversation/api"
	"ai-agent-character-demo/backend/conversation/repository"
	"ai-agent-character-demo/backend/conversation/service"

	"gorm.io/gorm"
)

func NewMessageHandlerWithDI(db *gorm.DB) *api.MessageHandler {
	repo := repository.NewGormMessageRepository(db)
	svc := service.NewMessageService(repo)
	handler := api.NewMessageHandler(svc)
	return handler
}
