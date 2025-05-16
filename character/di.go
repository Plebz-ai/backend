package main

import (
	"ai-agent-character-demo/backend/character/api"
	"ai-agent-character-demo/backend/character/repository"
	"ai-agent-character-demo/backend/character/service"

	"gorm.io/gorm"
)

func NewCharacterHandlerWithDI(db *gorm.DB) *api.CharacterHandler {
	repo := repository.NewGormCharacterRepository(db)
	svc := service.NewCharacterService(repo)
	handler := api.NewCharacterHandler(svc)
	return handler
}
