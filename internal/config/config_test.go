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
		"WEATHER_CACHE_TTL", "WEATHER_TOOL_TIMEOUT", "WEATHER_MAX_CALLS_PER_RUN",
		"RESEARCH_TIMEOUT", "RESEARCH_MAX_TOOL_CALLS", "SKILLS_DIR",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("AUTH_ENABLED", "")
	t.Setenv("AUTH_SESSION_TTL", "")
	t.Setenv("AUTH_COOKIE_SECURE", "")
	t.Setenv("AUTH_WEB_BASE_URL", "")
	t.Setenv("OIDC_ISSUER", "")
	t.Setenv("OIDC_CLIENT_ID", "")
	t.Setenv("OIDC_CLIENT_SECRET", "")
	t.Setenv("OIDC_REDIRECT_URI", "")
	t.Setenv("OIDC_SCOPES", "")
	t.Setenv("OIDC_AUDIENCE", "")
	t.Setenv("OIDC_PROVIDER_NAME", "")
	t.Setenv("OIDC_POST_LOGOUT_REDIRECT_URI", "")
	t.Setenv("ADMIN_EMAILS", "")
	t.Setenv("ADMIN_SUBS", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
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
	if cfg.WeatherCacheTTL != 30*time.Minute || cfg.WeatherToolTimeout != 3*time.Second || cfg.WeatherMaxCallsPerRun != 1 {
		t.Fatalf("unexpected weather limits: %+v", cfg)
	}
	if cfg.SkillsDir != "skills" {
		t.Fatalf("SkillsDir = %q", cfg.SkillsDir)
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
	t.Setenv("WEATHER_CACHE_TTL", "15m")
	t.Setenv("WEATHER_TOOL_TIMEOUT", "2s")
	t.Setenv("WEATHER_MAX_CALLS_PER_RUN", "2")

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
	if cfg.WeatherCacheTTL != 15*time.Minute || cfg.WeatherToolTimeout != 2*time.Second || cfg.WeatherMaxCallsPerRun != 2 {
		t.Fatalf("weather overrides = %+v", cfg)
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
		{key: "WEATHER_CACHE_TTL", value: "0s", want: "WEATHER_CACHE_TTL"},
		{key: "WEATHER_TOOL_TIMEOUT", value: "soon", want: "WEATHER_TOOL_TIMEOUT"},
		{key: "WEATHER_MAX_CALLS_PER_RUN", value: "11", want: "WEATHER_MAX_CALLS_PER_RUN"},
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

func TestLoadAuthDisabledAllowsEmptyOIDC(t *testing.T) {
	clearPluginEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/trippilot")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AuthEnabled {
		t.Fatal("AuthEnabled = true, want false")
	}
	if cfg.OIDCScopes[len(cfg.OIDCScopes)-1] != "email" || cfg.AuthSessionTTL != 168*time.Hour {
		t.Fatalf("unexpected auth defaults: %+v", cfg)
	}
	if cfg.AuthCookieSecure != "auto" || cfg.OIDCProviderName != "统一登录" {
		t.Fatalf("unexpected auth defaults: %+v", cfg)
	}
}

func TestLoadAuthEnabledRequiresOIDCFields(t *testing.T) {
	base := map[string]string{
		"OIDC_ISSUER":        "http://127.0.0.1:9410",
		"OIDC_CLIENT_ID":     "trippilot",
		"OIDC_CLIENT_SECRET": "secret",
		"OIDC_REDIRECT_URI":  "http://127.0.0.1:8080/api/auth/callback",
	}
	for _, key := range []string{"OIDC_ISSUER", "OIDC_CLIENT_ID", "OIDC_CLIENT_SECRET", "OIDC_REDIRECT_URI"} {
		t.Run(key, func(t *testing.T) {
			clearPluginEnv(t)
			t.Setenv("DATABASE_URL", "postgres://localhost/trippilot")
			t.Setenv("AUTH_ENABLED", "true")
			for name, value := range base {
				t.Setenv(name, value)
			}
			t.Setenv(key, "")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("Load() error = %v, want mention of %s", err, key)
			}
		})
	}
}

func TestLoadAuthParsesListsAndOverrides(t *testing.T) {
	clearPluginEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/trippilot")
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("OIDC_ISSUER", "http://127.0.0.1:9410")
	t.Setenv("OIDC_CLIENT_ID", "trippilot")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("OIDC_REDIRECT_URI", "http://127.0.0.1:8080/api/auth/callback")
	t.Setenv("OIDC_SCOPES", "openid  email")
	t.Setenv("AUTH_SESSION_TTL", "24h")
	t.Setenv("AUTH_COOKIE_SECURE", "false")
	t.Setenv("ADMIN_EMAILS", " a@example.com , ,B@Example.com")
	t.Setenv("ADMIN_SUBS", "sub-1, sub-2")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com,http://localhost:5173")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.OIDCScopes) != 2 || cfg.OIDCScopes[1] != "email" {
		t.Fatalf("OIDCScopes = %v", cfg.OIDCScopes)
	}
	if cfg.AuthSessionTTL != 24*time.Hour || cfg.AuthCookieSecure != "false" {
		t.Fatalf("auth overrides = %+v", cfg)
	}
	if len(cfg.AdminEmails) != 2 || cfg.AdminEmails[1] != "B@Example.com" {
		t.Fatalf("AdminEmails = %v", cfg.AdminEmails)
	}
	if len(cfg.AdminSubs) != 2 || len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("lists = %+v", cfg)
	}
}

func TestLoadRejectsInvalidAuthConfig(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "cookie secure", key: "AUTH_COOKIE_SECURE", value: "sometimes", want: "AUTH_COOKIE_SECURE"},
		{name: "cors wildcard", key: "CORS_ALLOWED_ORIGINS", value: "https://app.example.com,*", want: "CORS_ALLOWED_ORIGINS"},
		{name: "web base url", key: "AUTH_WEB_BASE_URL", value: "not-a-url", want: "AUTH_WEB_BASE_URL"},
		{name: "issuer url", key: "OIDC_ISSUER", value: "ftp://issuer", want: "OIDC_ISSUER"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearPluginEnv(t)
			t.Setenv("DATABASE_URL", "postgres://localhost/trippilot")
			t.Setenv("AUTH_ENABLED", "true")
			t.Setenv("OIDC_ISSUER", "http://127.0.0.1:9410")
			t.Setenv("OIDC_CLIENT_ID", "trippilot")
			t.Setenv("OIDC_CLIENT_SECRET", "secret")
			t.Setenv("OIDC_REDIRECT_URI", "http://127.0.0.1:8080/api/auth/callback")
			t.Setenv(tt.key, tt.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %v, want mention of %s", err, tt.want)
			}
		})
	}
}
