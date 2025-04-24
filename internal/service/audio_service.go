package service

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ai-agent-character-demo/backend/internal/models"
)

// AudioServiceConfig defines configuration for the audio service
type AudioServiceConfig struct {
	MaxChunksPerSession int // Maximum number of audio chunks allowed per session
	DefaultTTL          time.Duration
}

// DefaultAudioServiceConfig returns default configuration
func DefaultAudioServiceConfig() AudioServiceConfig {
	return AudioServiceConfig{
		MaxChunksPerSession: 1000, // Default: allow 1000 chunks per session
		DefaultTTL:          24 * time.Hour,
	}
}

// AudioService handles operations related to audio data
type AudioService struct {
	db     *gorm.DB
	config AudioServiceConfig
}

// NewAudioService creates a new audio service with default config
func NewAudioService(db *gorm.DB) *AudioService {
	return NewAudioServiceWithConfig(db, DefaultAudioServiceConfig())
}

// NewAudioServiceWithConfig creates a new audio service with custom config
func NewAudioServiceWithConfig(db *gorm.DB, config AudioServiceConfig) *AudioService {
	service := &AudioService{
		db:     db,
		config: config,
	}

	// Start the cleanup routine in a separate goroutine
	go service.startCleanupRoutine()

	return service
}

// StoreAudioChunk saves an audio chunk to the database with TTL
func (s *AudioService) StoreAudioChunk(
	userID string,
	sessionID string,
	charID uint,
	audioData []byte,
	format string,
	duration float64,
	sampleRate int,
	channels int,
	metadata string,
	ttl time.Duration,
) (string, error) {
	if len(audioData) == 0 {
		return "", errors.New("audio data cannot be empty")
	}

	// Check if this session has reached the maximum number of chunks
	if s.config.MaxChunksPerSession > 0 {
		var count int64
		if err := s.db.Model(&models.AudioChunk{}).Where("session_id = ?", sessionID).Count(&count).Error; err != nil {
			log.Printf("Error counting audio chunks: %v", err)
		} else if count >= int64(s.config.MaxChunksPerSession) {
			return "", fmt.Errorf("session has reached the maximum limit of %d audio chunks", s.config.MaxChunksPerSession)
		}
	}

	// Generate a unique ID for this chunk
	chunkID := uuid.New().String()

	// Create the audio chunk record
	chunk := &models.AudioChunk{
		UserID:           userID,
		SessionID:        sessionID,
		CharID:           charID,
		AudioData:        audioData,
		Format:           format,
		Duration:         duration,
		SampleRate:       sampleRate,
		Channels:         channels,
		CreatedAt:        time.Now(),
		ExpiresAt:        time.Now().Add(ttl),
		Metadata:         metadata,
		ProcessingStatus: "pending",
	}

	// Store in database
	if err := s.db.Create(chunk).Error; err != nil {
		return "", fmt.Errorf("failed to store audio chunk: %w", err)
	}

	return chunkID, nil
}

// GetAudioChunk retrieves an audio chunk by ID
func (s *AudioService) GetAudioChunk(id string) (*models.AudioChunk, error) {
	var chunk models.AudioChunk

	if err := s.db.Where("id = ?", id).First(&chunk).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("audio chunk not found")
		}
		return nil, fmt.Errorf("error retrieving audio chunk: %w", err)
	}

	// Check if the chunk has expired
	if chunk.Expired() {
		s.db.Delete(&chunk) // Delete expired chunk
		return nil, errors.New("audio chunk has expired")
	}

	return &chunk, nil
}

// GetSessionAudioChunks retrieves all audio chunks for a session
func (s *AudioService) GetSessionAudioChunks(sessionID string) ([]*models.AudioChunk, error) {
	var chunks []*models.AudioChunk

	if err := s.db.Where("session_id = ? AND expires_at > ?", sessionID, time.Now()).Find(&chunks).Error; err != nil {
		return nil, fmt.Errorf("error retrieving session audio chunks: %w", err)
	}

	return chunks, nil
}

// UpdateProcessingStatus updates the processing status of an audio chunk
func (s *AudioService) UpdateProcessingStatus(id string, status string) error {
	result := s.db.Model(&models.AudioChunk{}).
		Where("id = ?", id).
		Update("processing_status", status)

	if result.Error != nil {
		return fmt.Errorf("failed to update processing status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errors.New("audio chunk not found")
	}

	return nil
}

// DeleteAudioChunk deletes an audio chunk by ID
func (s *AudioService) DeleteAudioChunk(id string) error {
	result := s.db.Delete(&models.AudioChunk{}, "id = ?", id)

	if result.Error != nil {
		return fmt.Errorf("failed to delete audio chunk: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errors.New("audio chunk not found")
	}

	return nil
}

// CleanupExpiredChunks removes all expired audio chunks
func (s *AudioService) CleanupExpiredChunks() (int64, error) {
	result := s.db.Where("expires_at < ?", time.Now()).Delete(&models.AudioChunk{})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup expired chunks: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// startCleanupRoutine runs periodic cleanup of expired audio chunks
func (s *AudioService) startCleanupRoutine() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		count, err := s.CleanupExpiredChunks()
		if err != nil {
			log.Printf("Error cleaning up expired audio chunks: %v", err)
		} else if count > 0 {
			log.Printf("Cleaned up %d expired audio chunks", count)
		}
	}
}

// GetPendingAudioChunks retrieves audio chunks with pending processing status
func (s *AudioService) GetPendingAudioChunks(limit int, offset int) ([]*models.AudioChunk, int64, error) {
	if limit <= 0 {
		limit = 100 // Default limit
	}

	var chunks []*models.AudioChunk
	var total int64

	// Count total pending chunks
	if err := s.db.Model(&models.AudioChunk{}).
		Where("processing_status = ? AND expires_at > ?", "pending", time.Now()).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("error counting pending audio chunks: %w", err)
	}

	// Get chunks with pagination
	if err := s.db.Where("processing_status = ? AND expires_at > ?", "pending", time.Now()).
		Limit(limit).
		Offset(offset).
		Order("created_at DESC").
		Find(&chunks).Error; err != nil {
		return nil, 0, fmt.Errorf("error retrieving pending audio chunks: %w", err)
	}

	return chunks, total, nil
}

// GetAllAudioChunks retrieves all audio chunks regardless of status
func (s *AudioService) GetAllAudioChunks(limit, offset int) ([]*models.AudioChunk, int64, error) {
	if limit <= 0 {
		limit = 100
	}

	// Count the total number of audio chunks that haven't expired
	var total int64
	err := s.db.Model(&models.AudioChunk{}).
		Where("expires_at > CURRENT_TIMESTAMP").
		Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("error counting audio chunks: %w", err)
	}

	// Get the audio chunks with pagination
	chunks := []*models.AudioChunk{}
	err = s.db.Where("expires_at > CURRENT_TIMESTAMP").
		Limit(limit).
		Offset(offset).
		Order("created_at DESC").
		Find(&chunks).Error
	if err != nil {
		return nil, total, fmt.Errorf("error getting audio chunks: %w", err)
	}

	return chunks, total, nil
}

// GetConfig returns the current audio service configuration
func (s *AudioService) GetConfig() AudioServiceConfig {
	return s.config
}

// UpdateConfig updates the audio service configuration
func (s *AudioService) UpdateConfig(config AudioServiceConfig) {
	s.config = config
}
