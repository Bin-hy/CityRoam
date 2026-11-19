package config

import (
	"log/slog"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/trippilot")
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("RATE_LIMIT_PER_HOUR", "5")
	t.Setenv("LLM_BASE_URL", "https://example.com")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_MODEL", "test-model")
	t.Setenv("AMAP_WEB_KEY", "map-key")
	t.Setenv("ADMIN_TOKEN", "admin")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 9090 {
		t.Fatalf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("LogLevel = %v, want debug", cfg.LogLevel)
	}
	if cfg.RateLimitPerHour != 5 {
		t.Fatalf("RateLimitPerHour = %d, want 5", cfg.RateLimitPerHour)
	}
	if cfg.LLMBaseURL != "https://example.com" || cfg.LLMModel != "test-model" {
		t.Fatalf("LLM config not loaded: %+v", cfg)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/trippilot")
	t.Setenv("PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("RATE_LIMIT_PER_HOUR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 8080 || cfg.LogLevel != slog.LevelInfo || cfg.RateLimitPerHour != 10 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/trippilot")
	t.Setenv("PORT", "70000")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestParseLevelRejectsUnknown(t *testing.T) {
	if _, err := parseLevel("trace"); err == nil {
		t.Fatal("parseLevel() error = nil, want error")
	}
}
