package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
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

	HotelPluginEnabled bool
	HotelToolURL       string
	HotelToolToken     string
	HotelSearchTimeout time.Duration
	HotelCacheTTL      time.Duration

	GuidePluginEnabled bool
	GuideSearchBaseURL string
	GuideSearchAPIKey  string
	GuideSearchTimeout time.Duration
	GuideCacheTTL      time.Duration

	ResearchTimeout      time.Duration
	ResearchMaxToolCalls int
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

	hotelEnabled, err := envBool("HOTEL_PLUGIN_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	guideEnabled, err := envBool("GUIDE_PLUGIN_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	hotelTimeout, err := envDuration("HOTEL_SEARCH_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	hotelCacheTTL, err := envDuration("HOTEL_CACHE_TTL", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}
	guideTimeout, err := envDuration("GUIDE_SEARCH_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	guideCacheTTL, err := envDuration("GUIDE_CACHE_TTL", 6*time.Hour)
	if err != nil {
		return Config{}, err
	}
	researchTimeout, err := envDuration("RESEARCH_TIMEOUT", 12*time.Second)
	if err != nil {
		return Config{}, err
	}
	maxCalls, err := envInt("RESEARCH_MAX_TOOL_CALLS", 6, 1, 100)
	if err != nil {
		return Config{}, err
	}

	cfg.HotelPluginEnabled = hotelEnabled
	cfg.HotelToolURL = envString("HOTEL_TOOL_URL", "http://hotel-tool:8090")
	cfg.HotelToolToken = os.Getenv("HOTEL_TOOL_TOKEN")
	cfg.HotelSearchTimeout = hotelTimeout
	cfg.HotelCacheTTL = hotelCacheTTL
	cfg.GuidePluginEnabled = guideEnabled
	cfg.GuideSearchBaseURL = envString("GUIDE_SEARCH_BASE_URL", "https://api.deepseek.com")
	cfg.GuideSearchAPIKey = os.Getenv("GUIDE_SEARCH_API_KEY")
	cfg.GuideSearchTimeout = guideTimeout
	cfg.GuideCacheTTL = guideCacheTTL
	cfg.ResearchTimeout = researchTimeout
	cfg.ResearchMaxToolCalls = maxCalls

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

func envBool(key string, fallback bool) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return value, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 10s or 30m: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
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
