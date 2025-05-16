package config

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewDB creates a new database connection using configuration settings
func NewDB() (*gorm.DB, error) {
	// Get configuration
	cfg := Get()

	// Create DSN string for PostgreSQL
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)

	// Configure GORM
	gormConfig := &gorm.Config{}

	// Set logging level based on application environment
	if cfg.Server.Env == "development" {
		gormConfig.Logger = logger.Default.LogMode(logger.Info)
	} else {
		gormConfig.Logger = logger.Default.LogMode(logger.Error)
	}

	// Add retry mechanism
	var db *gorm.DB
	var err error
	retries := 5
	delay := 5 * time.Second

	for i := 0; i < retries; i++ {
		db, err = gorm.Open(postgres.Open(dsn), gormConfig)
		if err == nil {
			break
		}

		fmt.Printf("Failed to connect to database. Retrying in %v...\n", delay)
		time.Sleep(delay)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database after %d retries: %w", retries, err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(cfg.Database.MaxConns)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	return db, nil
}

// TestConnection checks if the database connection is working
func TestConnection(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	return nil
}
