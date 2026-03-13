package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration.
type Config struct {
	TelegramToken           string
	TelegramChatID          int64
	DatabasePath            string
	HTTPTimeout             time.Duration
	MaxFailureNotifications int
	LogLevel                string
	HealthPort              int
}

// Load reads configuration from environment variables, applies defaults, and validates.
func Load() (*Config, error) {
	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_TOKEN is required")
	}

	chatIDStr := os.Getenv("TELEGRAM_CHAT_ID")
	if chatIDStr == "" {
		return nil, fmt.Errorf("TELEGRAM_CHAT_ID is required")
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("TELEGRAM_CHAT_ID must be a valid integer: %w", err)
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./data/uptime.db"
	}

	httpTimeout := 10 * time.Second
	if v := os.Getenv("HTTP_TIMEOUT"); v != "" {
		httpTimeout, err = time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("HTTP_TIMEOUT must be a valid duration: %w", err)
		}
	}

	maxFailures := 3
	if v := os.Getenv("MAX_FAILURE_NOTIFICATIONS"); v != "" {
		maxFailures, err = strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("MAX_FAILURE_NOTIFICATIONS must be a valid integer: %w", err)
		}
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	healthPort := 8080
	if v := os.Getenv("HEALTH_PORT"); v != "" {
		healthPort, err = strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("HEALTH_PORT must be a valid integer: %w", err)
		}
	}

	return &Config{
		TelegramToken:           token,
		TelegramChatID:          chatID,
		DatabasePath:            dbPath,
		HTTPTimeout:             httpTimeout,
		MaxFailureNotifications: maxFailures,
		LogLevel:                logLevel,
		HealthPort:              healthPort,
	}, nil
}
