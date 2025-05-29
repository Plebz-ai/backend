package service

import (
	"ai-agent-character-demo/backend/conversation/models"
	"ai-agent-character-demo/backend/conversation/repository"
)

type MessageService struct {
	repo repository.MessageRepository
}

func NewMessageService(repo repository.MessageRepository) *MessageService {
	return &MessageService{repo: repo}
}

func (s *MessageService) CreateMessage(message *models.Message, userID uint) error {
	// Add business logic, validation, etc. here
	// userID is available for AI orchestration
	return s.repo.Create(message)
}

func (s *MessageService) GetMessageByID(id uint) (*models.Message, error) {
	return s.repo.GetByID(id)
}

func (s *MessageService) GetMessagesBySession(sessionID string) ([]models.Message, error) {
	return s.repo.GetBySession(sessionID)
}

func (s *MessageService) GetMessagesBySessionPaginated(sessionID string, limit, offset int) ([]models.Message, int, error) {
	messages, err := s.repo.GetBySessionPaginated(sessionID, limit, offset)
	total := 0
	if err == nil {
		total = len(messages)
	}
	return messages, total, err
}
