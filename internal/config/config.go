package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL      string
	Port             int
	LogLevel         slog.Level
	LLMBaseURL       string
	LLMAPIKey        string
	LLMModel         string
	AMapWebKey       string
	AdminToken       string
	RateLimitPerHour int
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		LLMBaseURL:  os.Getenv("LLM_BASE_URL"),
		LLMAPIKey:   os.Getenv("LLM_API_KEY"),
		LLMModel:    os.Getenv("LLM_MODEL"),
		AMapWebKey:  os.Getenv("AMAP_WEB_KEY"),
		AdminToken:  os.Getenv("ADMIN_TOKEN"),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	port, err := envInt("PORT", 8080, 1, 65535)
	if err != nil {
		return Config{}, err
	}
	cfg.Port = port

	rate, err := envInt("RATE_LIMIT_PER_HOUR", 10, 1, 1_000_000)
	if err != nil {
		return Config{}, err
	}
	cfg.RateLimitPerHour = rate

	level, err := parseLevel(envString("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}
	cfg.LogLevel = level

	return cfg, nil
}

func envString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback, min, max int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	if value < min || value > max {
		return 0, fmt.Errorf("%s must be between %d and %d", key, min, max)
	}
	return value, nil
}

func parseLevel(raw string) (slog.Level, error) {
	switch raw {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error")
	}
}
