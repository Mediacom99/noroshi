package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration.
type Config struct {
	TelegramToken           string
	TelegramChatID          int64
	DatabasePath            string
	HTTPTimeout             time.Duration
	MaxFailureNotifications int
	FailureThreshold        int
	SlowThresholdMs         int64
	ReminderInterval        time.Duration
	Digest                  string   // "", "daily" or "weekly"
	DigestTimeMinutes       int      // minutes since midnight UTC
	AlertWebhookURL         string   // generic alert webhook; "" = disabled
	AlertWebhookToken       string   // optional Bearer token for the webhook
	TelegramWebhookURL      string   // public https URL for Telegram webhook mode; "" = long polling
	TelegramWebhookPort     int      // local listen port for webhook mode
	TelegramWebhookSecret   string   // verified against X-Telegram-Bot-Api-Secret-Token
	DashboardToken          string   // Bearer token for the /api/ dashboard API; "" = API disabled
	DashboardOrigins        []string // allowed CORS origins for the dashboard API; empty = same-origin only
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
	if maxFailures < 1 {
		return nil, fmt.Errorf("MAX_FAILURE_NOTIFICATIONS must be at least 1")
	}

	failureThreshold := 1
	if v := os.Getenv("FAILURE_THRESHOLD"); v != "" {
		failureThreshold, err = strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("FAILURE_THRESHOLD must be a valid integer: %w", err)
		}
	}
	if failureThreshold < 1 {
		return nil, fmt.Errorf("FAILURE_THRESHOLD must be at least 1")
	}

	var slowThresholdMs int64
	if v := os.Getenv("SLOW_THRESHOLD_MS"); v != "" {
		slowThresholdMs, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("SLOW_THRESHOLD_MS must be a valid integer: %w", err)
		}
		if slowThresholdMs < 0 {
			return nil, fmt.Errorf("SLOW_THRESHOLD_MS must not be negative")
		}
	}

	var reminderInterval time.Duration
	if v := os.Getenv("REMINDER_INTERVAL"); v != "" {
		reminderInterval, err = time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("REMINDER_INTERVAL must be a valid duration: %w", err)
		}
		if reminderInterval < 0 {
			return nil, fmt.Errorf("REMINDER_INTERVAL must not be negative")
		}
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	digest := os.Getenv("DIGEST")
	switch digest {
	case "", "off":
		digest = ""
	case "daily", "weekly":
	default:
		return nil, fmt.Errorf("DIGEST must be daily, weekly or off")
	}

	digestTimeMinutes := 9 * 60
	if v := os.Getenv("DIGEST_TIME"); v != "" {
		var h, m int
		if _, err := fmt.Sscanf(v, "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			return nil, fmt.Errorf("DIGEST_TIME must be HH:MM (00:00-23:59, UTC)")
		}
		digestTimeMinutes = h*60 + m
	}

	healthPort := 8080
	if v := os.Getenv("HEALTH_PORT"); v != "" {
		healthPort, err = strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("HEALTH_PORT must be a valid integer: %w", err)
		}
	}

	alertWebhookURL := os.Getenv("ALERT_WEBHOOK_URL")
	if alertWebhookURL != "" {
		parsed, err := url.Parse(alertWebhookURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("ALERT_WEBHOOK_URL must be a valid http(s) URL")
		}
	}
	alertWebhookToken := os.Getenv("ALERT_WEBHOOK_TOKEN")

	telegramWebhookURL := os.Getenv("TELEGRAM_WEBHOOK_URL")
	telegramWebhookPort := 8081
	if telegramWebhookURL != "" {
		parsed, err := url.Parse(telegramWebhookURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, fmt.Errorf("TELEGRAM_WEBHOOK_URL must be a valid https URL (Telegram requires TLS, e.g. behind a reverse proxy)")
		}
		if v := os.Getenv("TELEGRAM_WEBHOOK_PORT"); v != "" {
			telegramWebhookPort, err = strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("TELEGRAM_WEBHOOK_PORT must be a valid integer: %w", err)
			}
		}
	}
	telegramWebhookSecret := os.Getenv("TELEGRAM_WEBHOOK_SECRET")

	dashboardToken := os.Getenv("DASHBOARD_TOKEN")
	var dashboardOrigins []string
	if v := os.Getenv("DASHBOARD_ORIGIN"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				dashboardOrigins = append(dashboardOrigins, o)
			}
		}
	}

	return &Config{
		TelegramToken:           token,
		TelegramChatID:          chatID,
		DatabasePath:            dbPath,
		HTTPTimeout:             httpTimeout,
		MaxFailureNotifications: maxFailures,
		FailureThreshold:        failureThreshold,
		SlowThresholdMs:         slowThresholdMs,
		ReminderInterval:        reminderInterval,
		Digest:                  digest,
		DigestTimeMinutes:       digestTimeMinutes,
		AlertWebhookURL:         alertWebhookURL,
		AlertWebhookToken:       alertWebhookToken,
		TelegramWebhookURL:      telegramWebhookURL,
		TelegramWebhookPort:     telegramWebhookPort,
		TelegramWebhookSecret:   telegramWebhookSecret,
		DashboardToken:          dashboardToken,
		DashboardOrigins:        dashboardOrigins,
		LogLevel:                logLevel,
		HealthPort:              healthPort,
	}, nil
}
