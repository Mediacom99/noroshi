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
		"TELEGRAM_TOKEN":           "test-token",
		"TELEGRAM_CHAT_ID":        "-100123456789",
		"DATABASE_PATH":            "/tmp/test.db",
		"HTTP_TIMEOUT":             "5s",
		"MAX_FAILURE_NOTIFICATIONS": "5",
		"LOG_LEVEL":                "debug",
		"HEALTH_PORT":              "9090",
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
		"TELEGRAM_TOKEN":  "test-token",
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
		"TELEGRAM_TOKEN":  "test-token",
		"TELEGRAM_CHAT_ID": "not-a-number",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should fail for invalid TELEGRAM_CHAT_ID")
	}
}

func TestLoadInvalidHTTPTimeout(t *testing.T) {
	setEnv(t, map[string]string{
		"TELEGRAM_TOKEN":  "test-token",
		"TELEGRAM_CHAT_ID": "-100123",
		"HTTP_TIMEOUT":     "bad",
	})

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should fail for invalid HTTP_TIMEOUT")
	}
}
