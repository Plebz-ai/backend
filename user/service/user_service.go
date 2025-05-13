package service

import (
	"encoding/json"
	"fmt"
	"time"

	"ai-agent-character-demo/backend/shared/redis"
	"ai-agent-character-demo/backend/user/models"
	"ai-agent-character-demo/backend/user/repository"
)

type UserService struct {
	repo  repository.UserRepository
	cache *redis.RedisClient
}

func NewUserService(repo repository.UserRepository) *UserService {
	return &UserService{repo: repo, cache: redis.NewRedisClient()}
}

func (s *UserService) CreateUser(user *models.User) error {
	// Add business logic, validation, etc. here
	return s.repo.Create(user)
}

func (s *UserService) GetUserByEmail(email string) (*models.User, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("user:email:%s", email)
	if cached, err := s.cache.Get(cacheKey); err == nil && cached != "" {
		var user models.User
		if err := json.Unmarshal([]byte(cached), &user); err == nil {
			return &user, nil
		}
	}
	// Fallback to DB
	user, err := s.repo.GetByEmail(email)
	if err != nil {
		return nil, err
	}
	// Cache result
	if data, err := json.Marshal(user); err == nil {
		_ = s.cache.Set(cacheKey, data, 10*time.Minute)
	}
	return user, nil
}

func (s *UserService) GetUserByID(id uint) (*models.User, error) {
	return s.repo.GetByID(id)
}
