package main

import (
	"ai-agent-character-demo/backend/audio/api"
	"ai-agent-character-demo/backend/audio/repository"
	"ai-agent-character-demo/backend/audio/service"

	"gorm.io/gorm"
)

func NewAudioHandlerWithDI(db *gorm.DB) *api.AudioHandler {
	repo := repository.NewGormAudioRepository(db)
	svc := service.NewAudioService(repo)
	handler := api.NewAudioHandler(svc)
	return handler
}
