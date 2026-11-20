package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
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

	WeatherCacheTTL       time.Duration
	WeatherToolTimeout    time.Duration
	WeatherMaxCallsPerRun int

	ResearchTimeout      time.Duration
	ResearchMaxToolCalls int
	SkillsDir            string

	AuthEnabled               bool
	OIDCIssuer                string
	OIDCClientID              string
	OIDCClientSecret          string
	OIDCRedirectURI           string
	OIDCScopes                []string
	OIDCAudience              string
	OIDCProviderName          string
	OIDCPostLogoutRedirectURI string
	AuthWebBaseURL            string
	AuthSessionTTL            time.Duration
	AuthCookieSecure          string
	AdminEmails               []string
	AdminSubs                 []string
	CORSAllowedOrigins        []string
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
	weatherCacheTTL, err := envDuration("WEATHER_CACHE_TTL", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}
	weatherTimeout, err := envDuration("WEATHER_TOOL_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	weatherMaxCalls, err := envInt("WEATHER_MAX_CALLS_PER_RUN", 1, 1, 10)
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

	authEnabled, err := envBool("AUTH_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	sessionTTL, err := envDuration("AUTH_SESSION_TTL", 168*time.Hour)
	if err != nil {
		return Config{}, err
	}
	cookieSecure := envString("AUTH_COOKIE_SECURE", "auto")
	if err := validateCookieSecure(cookieSecure); err != nil {
		return Config{}, err
	}
	scopes := strings.Fields(envString("OIDC_SCOPES", "openid profile email"))
	if len(scopes) == 0 {
		return Config{}, errors.New("OIDC_SCOPES must not be empty")
	}
	cfg.AuthEnabled = authEnabled
	cfg.OIDCIssuer = strings.TrimSpace(os.Getenv("OIDC_ISSUER"))
	cfg.OIDCClientID = strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID"))
	cfg.OIDCClientSecret = os.Getenv("OIDC_CLIENT_SECRET")
	cfg.OIDCRedirectURI = strings.TrimSpace(os.Getenv("OIDC_REDIRECT_URI"))
	cfg.OIDCScopes = scopes
	cfg.OIDCAudience = strings.TrimSpace(os.Getenv("OIDC_AUDIENCE"))
	cfg.OIDCProviderName = envString("OIDC_PROVIDER_NAME", "统一登录")
	cfg.OIDCPostLogoutRedirectURI = strings.TrimSpace(os.Getenv("OIDC_POST_LOGOUT_REDIRECT_URI"))
	cfg.AuthWebBaseURL = strings.TrimSpace(os.Getenv("AUTH_WEB_BASE_URL"))
	cfg.AuthSessionTTL = sessionTTL
	cfg.AuthCookieSecure = cookieSecure
	cfg.AdminEmails = splitCSV(os.Getenv("ADMIN_EMAILS"))
	cfg.AdminSubs = splitCSV(os.Getenv("ADMIN_SUBS"))
	cfg.CORSAllowedOrigins = splitCSV(os.Getenv("CORS_ALLOWED_ORIGINS"))

	if cfg.AuthEnabled {
		if err := requireAuthEnv(cfg); err != nil {
			return Config{}, err
		}
	}
	if cfg.AuthWebBaseURL != "" {
		if err := validateAbsoluteURL("AUTH_WEB_BASE_URL", cfg.AuthWebBaseURL); err != nil {
			return Config{}, err
		}
	}
	if cfg.OIDCRedirectURI != "" {
		if err := validateAbsoluteURL("OIDC_REDIRECT_URI", cfg.OIDCRedirectURI); err != nil {
			return Config{}, err
		}
	}
	if cfg.OIDCPostLogoutRedirectURI != "" {
		if err := validateAbsoluteURL("OIDC_POST_LOGOUT_REDIRECT_URI", cfg.OIDCPostLogoutRedirectURI); err != nil {
			return Config{}, err
		}
	}
	for _, origin := range cfg.CORSAllowedOrigins {
		if origin == "*" {
			return Config{}, errors.New("CORS_ALLOWED_ORIGINS must list explicit origins, not *")
		}
		if err := validateAbsoluteURL("CORS_ALLOWED_ORIGINS", origin); err != nil {
			return Config{}, err
		}
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
	cfg.WeatherCacheTTL = weatherCacheTTL
	cfg.WeatherToolTimeout = weatherTimeout
	cfg.WeatherMaxCallsPerRun = weatherMaxCalls
	cfg.ResearchTimeout = researchTimeout
	cfg.ResearchMaxToolCalls = maxCalls
	cfg.SkillsDir = envString("SKILLS_DIR", "skills")

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

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func validateCookieSecure(raw string) error {
	switch raw {
	case "auto", "true", "false":
		return nil
	default:
		return fmt.Errorf("AUTH_COOKIE_SECURE must be one of auto, true, false")
	}
}

func requireAuthEnv(cfg Config) error {
	required := []struct {
		key   string
		value string
	}{
		{"OIDC_ISSUER", cfg.OIDCIssuer},
		{"OIDC_CLIENT_ID", cfg.OIDCClientID},
		{"OIDC_CLIENT_SECRET", cfg.OIDCClientSecret},
		{"OIDC_REDIRECT_URI", cfg.OIDCRedirectURI},
	}
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("%s is required when AUTH_ENABLED=true", item.key)
		}
	}
	return validateAbsoluteURL("OIDC_ISSUER", cfg.OIDCIssuer)
}

func validateAbsoluteURL(key, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", key, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", key)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s must include a host", key)
	}
	return nil
}
