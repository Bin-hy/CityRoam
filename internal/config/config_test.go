package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func clearPluginEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"HOTEL_PLUGIN_ENABLED", "HOTEL_TOOL_URL", "HOTEL_TOOL_TOKEN", "HOTEL_SEARCH_TIMEOUT", "HOTEL_CACHE_TTL",
		"GUIDE_PLUGIN_ENABLED", "GUIDE_SEARCH_BASE_URL", "GUIDE_SEARCH_API_KEY", "GUIDE_SEARCH_TIMEOUT", "GUIDE_CACHE_TTL",
		"RESEARCH_TIMEOUT", "RESEARCH_MAX_TOOL_CALLS",
	} {
		t.Setenv(key, "")
	}
}

func TestLoad(t *testing.T) {
	clearPluginEnv(t)
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
	clearPluginEnv(t)
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
	clearPluginEnv(t)
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	clearPluginEnv(t)
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

func TestLoadPluginDefaults(t *testing.T) {
	clearPluginEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/trippilot")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HotelPluginEnabled || cfg.GuidePluginEnabled {
		t.Fatalf("plugins should default off: %+v", cfg)
	}
	if cfg.HotelToolURL != "http://hotel-tool:8090" || cfg.GuideSearchBaseURL != "https://api.deepseek.com" {
		t.Fatalf("unexpected plugin URLs: %+v", cfg)
	}
	if cfg.HotelSearchTimeout != 10*time.Second || cfg.GuideSearchTimeout != 10*time.Second {
		t.Fatalf("unexpected tool timeouts: %+v", cfg)
	}
	if cfg.HotelCacheTTL != 30*time.Minute || cfg.GuideCacheTTL != 6*time.Hour {
		t.Fatalf("unexpected cache TTLs: %+v", cfg)
	}
	if cfg.ResearchTimeout != 12*time.Second || cfg.ResearchMaxToolCalls != 6 {
		t.Fatalf("unexpected research limits: %+v", cfg)
	}
}

func TestLoadPluginExplicitValues(t *testing.T) {
	clearPluginEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/trippilot")
	t.Setenv("HOTEL_PLUGIN_ENABLED", "true")
	t.Setenv("GUIDE_PLUGIN_ENABLED", "false")
	t.Setenv("HOTEL_SEARCH_TIMEOUT", "250ms")
	t.Setenv("HOTEL_CACHE_TTL", "45m")
	t.Setenv("RESEARCH_TIMEOUT", "8s")
	t.Setenv("RESEARCH_MAX_TOOL_CALLS", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.HotelPluginEnabled || cfg.GuidePluginEnabled {
		t.Fatalf("plugin toggles = %+v", cfg)
	}
	if cfg.HotelSearchTimeout != 250*time.Millisecond || cfg.HotelCacheTTL != 45*time.Minute {
		t.Fatalf("duration overrides = %+v", cfg)
	}
	if cfg.ResearchTimeout != 8*time.Second || cfg.ResearchMaxToolCalls != 3 {
		t.Fatalf("research overrides = %+v", cfg)
	}
}

func TestLoadRejectsInvalidPluginConfig(t *testing.T) {
	tests := []struct {
		key   string
		value string
		want  string
	}{
		{key: "HOTEL_PLUGIN_ENABLED", value: "yes-please", want: "HOTEL_PLUGIN_ENABLED"},
		{key: "GUIDE_PLUGIN_ENABLED", value: "maybe", want: "GUIDE_PLUGIN_ENABLED"},
		{key: "HOTEL_SEARCH_TIMEOUT", value: "abc", want: "HOTEL_SEARCH_TIMEOUT"},
		{key: "GUIDE_CACHE_TTL", value: "-1s", want: "GUIDE_CACHE_TTL"},
		{key: "RESEARCH_MAX_TOOL_CALLS", value: "0", want: "RESEARCH_MAX_TOOL_CALLS"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			clearPluginEnv(t)
			t.Setenv("DATABASE_URL", "postgres://localhost/trippilot")
			t.Setenv(tt.key, tt.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %v, want mention of %s", err, tt.want)
			}
		})
	}
}

func TestLoadAllowsEmptyGuideKey(t *testing.T) {
	clearPluginEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/trippilot")
	t.Setenv("GUIDE_PLUGIN_ENABLED", "true")
	t.Setenv("GUIDE_SEARCH_API_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.GuidePluginEnabled || cfg.GuideSearchAPIKey != "" {
		t.Fatalf("guide config = %+v", cfg)
	}
}
