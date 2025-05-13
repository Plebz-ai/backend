package main

import (
	"ai-agent-character-demo/backend/user/api"
	"ai-agent-character-demo/backend/user/repository"
	"ai-agent-character-demo/backend/user/service"

	"gorm.io/gorm"
)

func NewUserHandlerWithDI(db *gorm.DB) *api.UserHandler {
	repo := repository.NewGormUserRepository(db)
	svc := service.NewUserService(repo)
	handler := api.NewUserHandler(svc)
	return handler
}
