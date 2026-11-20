package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"trippilot/internal/domain"
	"trippilot/internal/tools"
)

const (
	weatherDefaultCacheTTL = 30 * time.Minute
	weatherMaxDays         = 7
	weatherProviderName    = "amap"
)

var weatherInputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["city", "start_date", "days"],
  "properties": {
    "city": {"type": "string", "minLength": 1, "maxLength": 60},
    "start_date": {"type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
    "days": {"type": "integer", "minimum": 1, "maximum": 7}
  }
}`)

type WeatherInput struct {
	City      string `json:"city"`
	StartDate string `json:"start_date"`
	Days      int    `json:"days"`
}

func (in WeatherInput) Validate() error {
	if strings.TrimSpace(in.City) == "" {
		return fmt.Errorf("city is required")
	}
	if _, err := time.Parse("2006-01-02", in.StartDate); err != nil {
		return fmt.Errorf("start_date must use YYYY-MM-DD")
	}
	if in.Days < 1 || in.Days > weatherMaxDays {
		return fmt.Errorf("days must be between 1 and 7")
	}
	return nil
}

type WeatherOutput struct {
	Provider  string              `json:"provider"`
	City      string              `json:"city"`
	Days      []domain.WeatherDay `json:"days"`
	FetchedAt time.Time           `json:"fetched_at"`
	Warnings  []string            `json:"warnings,omitempty"`
}

type weatherProvider interface {
	Weather(ctx context.Context, city string) ([]tools.WeatherDay, error)
	Geocode(ctx context.Context, address, city string) ([]tools.Geocode, error)
}

type WeatherTool struct {
	provider weatherProvider
	now      func() time.Time
	cacheTTL time.Duration
}

type WeatherOption func(*WeatherTool)

func WithWeatherClock(now func() time.Time) WeatherOption {
	return func(t *WeatherTool) {
		if now != nil {
			t.now = now
		}
	}
}

func WithWeatherCacheTTL(ttl time.Duration) WeatherOption {
	return func(t *WeatherTool) {
		if ttl > 0 {
			t.cacheTTL = ttl
		}
	}
}

func NewWeatherTool(provider weatherProvider, opts ...WeatherOption) (*WeatherTool, error) {
	if provider == nil {
		return nil, fmt.Errorf("plugins: weather provider is required")
	}
	tool := &WeatherTool{
		provider: provider,
		now:      time.Now,
		cacheTTL: weatherDefaultCacheTTL,
	}
	for _, opt := range opts {
		opt(tool)
	}
	return tool, nil
}

func (t *WeatherTool) Name() string {
	return "get_weather"
}

func (t *WeatherTool) Provider() string {
	return weatherProviderName
}

func (t *WeatherTool) Description() string {
	return "Get the daily weather forecast for a city. Coverage is limited to the provider's short forecast window; dates outside it return no forecast. Use it to pick indoor places on rainy days; never invent weather."
}

func (t *WeatherTool) InputSchema() json.RawMessage {
	return weatherInputSchema
}

func (t *WeatherTool) CacheKey(input json.RawMessage) (string, error) {
	var in WeatherInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	return fmt.Sprintf("weather:v2:%s:%s:%d", normalizeComponent(in.City), in.StartDate, in.Days), nil
}

func (t *WeatherTool) CacheTTL() time.Duration {
	return t.cacheTTL
}

func (t *WeatherTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in WeatherInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, &ToolError{Code: CodeInvalidInput, Message: "weather input is not valid JSON"}
	}
	if err := in.Validate(); err != nil {
		return nil, &ToolError{Code: CodeInvalidInput, Message: err.Error()}
	}

	city := strings.TrimSpace(in.City)
	if isASCIIText(city) {
		resolved, err := t.provider.Geocode(ctx, city, "")
		if err != nil {
			return nil, weatherToolError(err)
		}
		city = ""
		if len(resolved) > 0 {
			city = strings.TrimSpace(resolved[0].City)
			if city == "" {
				city = strings.TrimSpace(resolved[0].Province)
			}
		}
		if city == "" {
			return nil, &ToolError{Code: CodeInvalidInput, Message: "city could not be resolved"}
		}
	}

	forecast, err := t.provider.Weather(ctx, city)
	if err != nil {
		return nil, weatherToolError(err)
	}

	days := filterWeatherWindow(forecast, in.StartDate, in.Days)
	output, err := json.Marshal(WeatherOutput{
		Provider:  t.Provider(),
		City:      city,
		Days:      days,
		FetchedAt: t.now().UTC(),
		Warnings:  weatherCoverageWarnings(forecast, days, in.Days),
	})
	if err != nil {
		return nil, &ToolError{Code: CodeBadResponse, Message: "failed to encode weather result"}
	}
	return output, nil
}

func (t *WeatherTool) ResultCount(output json.RawMessage) int {
	var decoded struct {
		Days []json.RawMessage `json:"days"`
	}
	if json.Unmarshal(output, &decoded) != nil {
		return 0
	}
	return len(decoded.Days)
}

func filterWeatherWindow(forecast []tools.WeatherDay, startDate string, days int) []domain.WeatherDay {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return []domain.WeatherDay{}
	}
	end := start.AddDate(0, 0, days-1)
	seen := make(map[string]struct{}, len(forecast))
	out := make([]domain.WeatherDay, 0, days)
	for _, day := range forecast {
		date, err := time.Parse("2006-01-02", day.Date)
		if err != nil {
			continue
		}
		if date.Before(start) || date.After(end) {
			continue
		}
		if _, dup := seen[day.Date]; dup {
			continue
		}
		seen[day.Date] = struct{}{}
		out = append(out, domain.WeatherDay{
			Date:         day.Date,
			DayWeather:   day.DayWeather,
			NightWeather: day.NightWeather,
			DayTemp:      day.DayTemp,
			NightTemp:    day.NightTemp,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func weatherCoverageWarnings(forecast []tools.WeatherDay, days []domain.WeatherDay, requested int) []string {
	availableFrom, availableTo := weatherForecastRange(forecast)
	available := "近期"
	if availableFrom != "" {
		available = availableFrom + " 至 " + availableTo
	}
	switch {
	case len(forecast) == 0:
		return []string{"天气不可用（暂无预报数据）"}
	case len(days) == 0:
		return []string{fmt.Sprintf("天气不可用（所选日期暂无预报，AMap 当前仅提供 %s 的预报）", available)}
	case len(days) < requested:
		return []string{fmt.Sprintf("天气仅覆盖 %d/%d 天（AMap 当前仅提供 %s 的预报）", len(days), requested, available)}
	default:
		return nil
	}
}

func weatherForecastRange(forecast []tools.WeatherDay) (string, string) {
	first, last := "", ""
	for _, day := range forecast {
		date := strings.TrimSpace(day.Date)
		if date == "" {
			continue
		}
		if first == "" || date < first {
			first = date
		}
		if last == "" || date > last {
			last = date
		}
	}
	return first, last
}

func weatherToolError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &ToolError{Code: CodeTimeout, Message: "weather lookup timed out", Retryable: true}
	}
	return &ToolError{Code: CodeUpstreamFailure, Message: "weather service is unavailable", Retryable: true}
}

func isASCIIText(value string) bool {
	for _, r := range value {
		if r > 127 {
			return false
		}
	}
	return true
}
