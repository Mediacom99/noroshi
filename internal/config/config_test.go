package config

import (
	"testing"
	"time"
)

func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func TestLoadValidConfig(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_TOKEN":            "test-token",
		"TELEGRAM_CHAT_ID":          "-100123456789",
		"DATABASE_PATH":             "/tmp/test.db",
		"HTTP_TIMEOUT":              "5s",
		"MAX_FAILURE_NOTIFICATIONS": "5",
		"LOG_LEVEL":                 "debug",
		"HEALTH_PORT":               "9090",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.TelegramToken != "test-token" {
		t.Errorf("TelegramToken = %q, want %q", cfg.TelegramToken, "test-token")
	}
	if cfg.TelegramChatID != -100123456789 {
		t.Errorf("TelegramChatID = %d, want %d", cfg.TelegramChatID, -100123456789)
	}
	if cfg.DatabasePath != "/tmp/test.db" {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, "/tmp/test.db")
	}
	if cfg.HTTPTimeout != 5*time.Second {
		t.Errorf("HTTPTimeout = %v, want %v", cfg.HTTPTimeout, 5*time.Second)
	}
	if cfg.MaxFailureNotifications != 5 {
		t.Errorf("MaxFailureNotifications = %d, want %d", cfg.MaxFailureNotifications, 5)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.HealthPort != 9090 {
		t.Errorf("HealthPort = %d, want %d", cfg.HealthPort, 9090)
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_TOKEN":   "test-token",
		"TELEGRAM_CHAT_ID": "-100123",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.DatabasePath != "./data/uptime.db" {
		t.Errorf("DatabasePath default = %q, want %q", cfg.DatabasePath, "./data/uptime.db")
	}
	if cfg.HTTPTimeout != 10*time.Second {
		t.Errorf("HTTPTimeout default = %v, want %v", cfg.HTTPTimeout, 10*time.Second)
	}
	if cfg.MaxFailureNotifications != 3 {
		t.Errorf("MaxFailureNotifications default = %d, want %d", cfg.MaxFailureNotifications, 3)
	}
	if cfg.FailureThreshold != 1 {
		t.Errorf("FailureThreshold default = %d, want %d", cfg.FailureThreshold, 1)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.HealthPort != 8080 {
		t.Errorf("HealthPort default = %d, want %d", cfg.HealthPort, 8080)
	}
}

func TestLoadMissingToken(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_CHAT_ID": "-100123",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should fail when TELEGRAM_TOKEN is missing")
	}
}

func TestLoadMissingChatID(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_TOKEN": "test-token",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should fail when TELEGRAM_CHAT_ID is missing")
	}
}

func TestLoadInvalidChatID(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_TOKEN":   "test-token",
		"TELEGRAM_CHAT_ID": "not-a-number",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should fail for invalid TELEGRAM_CHAT_ID")
	}
}

func TestLoadInvalidHTTPTimeout(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_TOKEN":   "test-token",
		"TELEGRAM_CHAT_ID": "-100123",
		"HTTP_TIMEOUT":     "bad",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should fail for invalid HTTP_TIMEOUT")
	}
}

func TestLoadFailureThreshold(t *testing.T) {
	base := map[string]string{
		"TELEGRAM_TOKEN":   "test-token",
		"TELEGRAM_CHAT_ID": "-100123",
	}

	base["FAILURE_THRESHOLD"] = "3"
	setEnv(t, base)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.FailureThreshold != 3 {
		t.Errorf("FailureThreshold = %d, want 3", cfg.FailureThreshold)
	}

	base["FAILURE_THRESHOLD"] = "abc"
	setEnv(t, base)
	if _, err := Load(); err == nil {
		t.Error("Load() should fail for non-integer FAILURE_THRESHOLD")
	}

	base["FAILURE_THRESHOLD"] = "0"
	setEnv(t, base)
	if _, err := Load(); err == nil {
		t.Error("Load() should fail for FAILURE_THRESHOLD < 1")
	}

	delete(base, "FAILURE_THRESHOLD")
	base["MAX_FAILURE_NOTIFICATIONS"] = "0"
	setEnv(t, base)
	if _, err := Load(); err == nil {
		t.Error("Load() should fail for MAX_FAILURE_NOTIFICATIONS < 1")
	}
}

func TestLoadSlowThresholdAndReminder(t *testing.T) {
	base := map[string]string{
		"TELEGRAM_TOKEN":   "test-token",
		"TELEGRAM_CHAT_ID": "-100123",
	}

	setEnv(t, base)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.SlowThresholdMs != 0 {
		t.Errorf("SlowThresholdMs default = %d, want 0", cfg.SlowThresholdMs)
	}
	if cfg.ReminderInterval != 0 {
		t.Errorf("ReminderInterval default = %v, want 0", cfg.ReminderInterval)
	}

	base["SLOW_THRESHOLD_MS"] = "2000"
	base["REMINDER_INTERVAL"] = "2h"
	setEnv(t, base)
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.SlowThresholdMs != 2000 {
		t.Errorf("SlowThresholdMs = %d, want 2000", cfg.SlowThresholdMs)
	}
	if cfg.ReminderInterval != 2*time.Hour {
		t.Errorf("ReminderInterval = %v, want 2h", cfg.ReminderInterval)
	}

	base["SLOW_THRESHOLD_MS"] = "-5"
	setEnv(t, base)
	if _, err := Load(); err == nil {
		t.Error("Load() should fail for negative SLOW_THRESHOLD_MS")
	}

	base["SLOW_THRESHOLD_MS"] = "2000"
	base["REMINDER_INTERVAL"] = "banana"
	setEnv(t, base)
	if _, err := Load(); err == nil {
		t.Error("Load() should fail for invalid REMINDER_INTERVAL")
	}
}

func TestLoadDigest(t *testing.T) {
	base := map[string]string{
		"TELEGRAM_TOKEN":   "test-token",
		"TELEGRAM_CHAT_ID": "123",
	}

	t.Run("default off", func(t *testing.T) {
		setEnv(t, base)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Digest != "" {
			t.Errorf("Digest = %q, want off", cfg.Digest)
		}
		if cfg.DigestTimeMinutes != 9*60 {
			t.Errorf("DigestTimeMinutes = %d, want 540", cfg.DigestTimeMinutes)
		}
	})

	t.Run("weekly with custom time", func(t *testing.T) {
		setEnv(t, base)
		t.Setenv("DIGEST", "weekly")
		t.Setenv("DIGEST_TIME", "07:30")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Digest != "weekly" {
			t.Errorf("Digest = %q, want weekly", cfg.Digest)
		}
		if cfg.DigestTimeMinutes != 7*60+30 {
			t.Errorf("DigestTimeMinutes = %d, want 450", cfg.DigestTimeMinutes)
		}
	})

	t.Run("invalid mode", func(t *testing.T) {
		setEnv(t, base)
		t.Setenv("DIGEST", "hourly")
		if _, err := Load(); err == nil {
			t.Error("expected error for DIGEST=hourly")
		}
	})

	t.Run("invalid time", func(t *testing.T) {
		setEnv(t, base)
		t.Setenv("DIGEST_TIME", "25:00")
		if _, err := Load(); err == nil {
			t.Error("expected error for DIGEST_TIME=25:00")
		}
	})
}

func TestLoadAlertWebhook(t *testing.T) {
	base := map[string]string{
		"TELEGRAM_TOKEN":   "test-token",
		"TELEGRAM_CHAT_ID": "123",
	}

	t.Run("default empty", func(t *testing.T) {
		setEnv(t, base)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AlertWebhookURL != "" || cfg.AlertWebhookToken != "" {
			t.Errorf("webhook should be off by default: %+v", cfg)
		}
	})

	t.Run("valid url and token", func(t *testing.T) {
		setEnv(t, base)
		t.Setenv("ALERT_WEBHOOK_URL", "https://alerts.example.com/hook")
		t.Setenv("ALERT_WEBHOOK_TOKEN", "secret")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AlertWebhookURL != "https://alerts.example.com/hook" || cfg.AlertWebhookToken != "secret" {
			t.Errorf("webhook config: %+v", cfg)
		}
	})

	t.Run("invalid url", func(t *testing.T) {
		setEnv(t, base)
		t.Setenv("ALERT_WEBHOOK_URL", "not-a-url")
		if _, err := Load(); err == nil {
			t.Error("expected error for invalid ALERT_WEBHOOK_URL")
		}
	})
}

func TestLoadTelegramWebhook(t *testing.T) {
	base := map[string]string{
		"TELEGRAM_TOKEN":   "test-token",
		"TELEGRAM_CHAT_ID": "123",
	}

	t.Run("default long polling", func(t *testing.T) {
		setEnv(t, base)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.TelegramWebhookURL != "" {
			t.Errorf("TelegramWebhookURL = %q, want empty (long polling)", cfg.TelegramWebhookURL)
		}
		if cfg.TelegramWebhookPort != 8081 {
			t.Errorf("TelegramWebhookPort = %d, want default 8081", cfg.TelegramWebhookPort)
		}
	})

	t.Run("valid webhook config", func(t *testing.T) {
		setEnv(t, base)
		t.Setenv("TELEGRAM_WEBHOOK_URL", "https://noroshi.example.com/telegram")
		t.Setenv("TELEGRAM_WEBHOOK_PORT", "9090")
		t.Setenv("TELEGRAM_WEBHOOK_SECRET", "s3cret")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.TelegramWebhookURL != "https://noroshi.example.com/telegram" {
			t.Errorf("TelegramWebhookURL = %q", cfg.TelegramWebhookURL)
		}
		if cfg.TelegramWebhookPort != 9090 {
			t.Errorf("TelegramWebhookPort = %d", cfg.TelegramWebhookPort)
		}
		if cfg.TelegramWebhookSecret != "s3cret" {
			t.Errorf("TelegramWebhookSecret = %q", cfg.TelegramWebhookSecret)
		}
	})

	t.Run("http url rejected", func(t *testing.T) {
		setEnv(t, base)
		t.Setenv("TELEGRAM_WEBHOOK_URL", "http://noroshi.example.com/telegram")
		if _, err := Load(); err == nil {
			t.Error("expected error: webhook URL must be https")
		}
	})

	t.Run("invalid port", func(t *testing.T) {
		setEnv(t, base)
		t.Setenv("TELEGRAM_WEBHOOK_URL", "https://noroshi.example.com/telegram")
		t.Setenv("TELEGRAM_WEBHOOK_PORT", "abc")
		if _, err := Load(); err == nil {
			t.Error("expected error for invalid TELEGRAM_WEBHOOK_PORT")
		}
	})
}
