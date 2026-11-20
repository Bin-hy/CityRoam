package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"trippilot/internal/tools"
)

type fakeWeatherProvider struct {
	forecast     []tools.WeatherDay
	geocodes     []tools.Geocode
	weatherErr   error
	geocodeErr   error
	weatherCalls int
	geocodeCalls int
	lastCity     string
}

func (p *fakeWeatherProvider) Weather(ctx context.Context, city string) ([]tools.WeatherDay, error) {
	p.weatherCalls++
	p.lastCity = city
	return p.forecast, p.weatherErr
}

func (p *fakeWeatherProvider) Geocode(ctx context.Context, address, city string) ([]tools.Geocode, error) {
	p.geocodeCalls++
	return p.geocodes, p.geocodeErr
}

func weatherTestForecast() []tools.WeatherDay {
	return []tools.WeatherDay{
		{Date: "2026-12-01", DayWeather: "晴", NightWeather: "多云", DayTemp: "8", NightTemp: "-2"},
		{Date: "2026-12-02", DayWeather: "小雨", NightWeather: "阴", DayTemp: "6", NightTemp: "-1"},
		{Date: "2026-12-03", DayWeather: "阴", NightWeather: "晴", DayTemp: "5", NightTemp: "-3"},
		{Date: "2026-12-04", DayWeather: "晴", NightWeather: "晴", DayTemp: "7", NightTemp: "-2"},
	}
}

func decodeWeatherOutput(t *testing.T, raw json.RawMessage) WeatherOutput {
	t.Helper()
	var out WeatherOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode weather output %s: %v", raw, err)
	}
	return out
}

func weatherErrorCode(t *testing.T, err error) string {
	t.Helper()
	var toolErr *ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("err = %v, want ToolError", err)
	}
	return toolErr.Code
}

func TestWeatherTool(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC) }

	t.Run("rejects invalid input", func(t *testing.T) {
		tool, err := NewWeatherTool(&fakeWeatherProvider{})
		if err != nil {
			t.Fatalf("new tool: %v", err)
		}
		cases := []string{
			`not json`,
			`{"city":"","start_date":"2026-12-01","days":2}`,
			`{"city":"北京","start_date":"12/01/2026","days":2}`,
			`{"city":"北京","start_date":"2026-12-01","days":0}`,
			`{"city":"北京","start_date":"2026-12-01","days":8}`,
		}
		for _, input := range cases {
			if _, err := tool.Call(context.Background(), json.RawMessage(input)); weatherErrorCode(t, err) != CodeInvalidInput {
				t.Errorf("input %s code = %q", input, weatherErrorCode(t, err))
			}
		}
	})

	t.Run("cache key is stable and city scoped", func(t *testing.T) {
		tool, _ := NewWeatherTool(&fakeWeatherProvider{}, WithWeatherCacheTTL(15*time.Minute))
		first, err := tool.CacheKey(json.RawMessage(`{"city":" 北京 ","start_date":"2026-12-01","days":2}`))
		if err != nil {
			t.Fatalf("key: %v", err)
		}
		second, _ := tool.CacheKey(json.RawMessage(`{"city":"北京","start_date":"2026-12-01","days":2}`))
		if first != second || first != "weather:v2:北京:2026-12-01:2" {
			t.Fatalf("keys = %q %q", first, second)
		}
		other, _ := tool.CacheKey(json.RawMessage(`{"city":"上海","start_date":"2026-12-01","days":2}`))
		if other == first {
			t.Fatal("different cities share a cache key")
		}
		if tool.CacheTTL() != 15*time.Minute {
			t.Fatalf("ttl = %v", tool.CacheTTL())
		}
		defaults, _ := NewWeatherTool(&fakeWeatherProvider{})
		if defaults.CacheTTL() != 30*time.Minute {
			t.Fatalf("default ttl = %v", defaults.CacheTTL())
		}
	})

	t.Run("chinese city queries directly and filters the window", func(t *testing.T) {
		provider := &fakeWeatherProvider{forecast: weatherTestForecast()}
		tool, _ := NewWeatherTool(provider, WithWeatherClock(clock))
		payload, err := tool.Call(context.Background(), json.RawMessage(`{"city":"北京","start_date":"2026-12-02","days":2}`))
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		if provider.geocodeCalls != 0 || provider.lastCity != "北京" {
			t.Fatalf("geocode=%d city=%q", provider.geocodeCalls, provider.lastCity)
		}
		out := decodeWeatherOutput(t, payload)
		if out.Provider != "amap" || out.City != "北京" || len(out.Days) != 2 {
			t.Fatalf("output = %+v", out)
		}
		if out.Days[0].Date != "2026-12-02" || out.Days[1].Date != "2026-12-03" {
			t.Fatalf("days = %+v", out.Days)
		}
		if !out.FetchedAt.Equal(clock()) {
			t.Fatalf("fetched_at = %v", out.FetchedAt)
		}
		if tool.ResultCount(payload) != 2 {
			t.Fatalf("result count = %d", tool.ResultCount(payload))
		}
	})

	t.Run("out of window explains the available forecast", func(t *testing.T) {
		provider := &fakeWeatherProvider{forecast: weatherTestForecast()}
		tool, _ := NewWeatherTool(provider, WithWeatherClock(clock))
		payload, err := tool.Call(context.Background(), json.RawMessage(`{"city":"北京","start_date":"2026-12-05","days":2}`))
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		out := decodeWeatherOutput(t, payload)
		if len(out.Days) != 0 || len(out.Warnings) != 1 {
			t.Fatalf("output = %+v", out)
		}
		want := "天气不可用（所选日期暂无预报，AMap 当前仅提供 2026-12-01 至 2026-12-04 的预报）"
		if out.Warnings[0] != want {
			t.Fatalf("warning = %q", out.Warnings[0])
		}
	})

	t.Run("partial window warns about coverage", func(t *testing.T) {
		provider := &fakeWeatherProvider{forecast: weatherTestForecast()}
		tool, _ := NewWeatherTool(provider, WithWeatherClock(clock))
		payload, err := tool.Call(context.Background(), json.RawMessage(`{"city":"北京","start_date":"2026-12-03","days":3}`))
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		out := decodeWeatherOutput(t, payload)
		if len(out.Days) != 2 || len(out.Warnings) != 1 {
			t.Fatalf("output = %+v", out)
		}
		want := "天气仅覆盖 2/3 天（AMap 当前仅提供 2026-12-01 至 2026-12-04 的预报）"
		if out.Warnings[0] != want {
			t.Fatalf("warning = %q", out.Warnings[0])
		}
	})

	t.Run("ascii city is geocoded once", func(t *testing.T) {
		provider := &fakeWeatherProvider{
			forecast: weatherTestForecast(),
			geocodes: []tools.Geocode{{City: "北京", Province: "北京市"}},
		}
		tool, _ := NewWeatherTool(provider)
		payload, err := tool.Call(context.Background(), json.RawMessage(`{"city":"beijing","start_date":"2026-12-01","days":1}`))
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		if provider.geocodeCalls != 1 || provider.lastCity != "北京" {
			t.Fatalf("geocode=%d city=%q", provider.geocodeCalls, provider.lastCity)
		}
		out := decodeWeatherOutput(t, payload)
		if out.City != "北京" || len(out.Days) != 1 {
			t.Fatalf("output = %+v", out)
		}

		provinceOnly := &fakeWeatherProvider{
			forecast: weatherTestForecast(),
			geocodes: []tools.Geocode{{Province: "北京市"}},
		}
		provinceTool, _ := NewWeatherTool(provinceOnly)
		if _, err := provinceTool.Call(context.Background(), json.RawMessage(`{"city":"beijing","start_date":"2026-12-01","days":1}`)); err != nil {
			t.Fatalf("province fallback: %v", err)
		}
		if provinceOnly.lastCity != "北京市" {
			t.Fatalf("province fallback city = %q", provinceOnly.lastCity)
		}
	})

	t.Run("unresolvable city is invalid input", func(t *testing.T) {
		tool, _ := NewWeatherTool(&fakeWeatherProvider{})
		_, err := tool.Call(context.Background(), json.RawMessage(`{"city":"nowhere","start_date":"2026-12-01","days":1}`))
		if code := weatherErrorCode(t, err); code != CodeInvalidInput {
			t.Fatalf("code = %q", code)
		}
	})

	t.Run("empty forecast returns an empty day list", func(t *testing.T) {
		provider := &fakeWeatherProvider{}
		tool, _ := NewWeatherTool(provider)
		payload, err := tool.Call(context.Background(), json.RawMessage(`{"city":"北京","start_date":"2026-12-01","days":2}`))
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		out := decodeWeatherOutput(t, payload)
		if out.Days == nil || len(out.Days) != 0 || tool.ResultCount(payload) != 0 {
			t.Fatalf("output = %+v", out)
		}
	})

	t.Run("upstream errors are retryable envelopes", func(t *testing.T) {
		provider := &fakeWeatherProvider{weatherErr: errors.New("boom")}
		tool, _ := NewWeatherTool(provider)
		_, err := tool.Call(context.Background(), json.RawMessage(`{"city":"北京","start_date":"2026-12-01","days":1}`))
		if code := weatherErrorCode(t, err); code != CodeUpstreamFailure {
			t.Fatalf("code = %q", code)
		}

		timeout := &fakeWeatherProvider{weatherErr: context.DeadlineExceeded}
		timeoutTool, _ := NewWeatherTool(timeout)
		_, err = timeoutTool.Call(context.Background(), json.RawMessage(`{"city":"北京","start_date":"2026-12-01","days":1}`))
		if code := weatherErrorCode(t, err); code != CodeTimeout {
			t.Fatalf("timeout code = %q", code)
		}

		geo := &fakeWeatherProvider{geocodeErr: errors.New("geo down")}
		geoTool, _ := NewWeatherTool(geo)
		_, err = geoTool.Call(context.Background(), json.RawMessage(`{"city":"beijing","start_date":"2026-12-01","days":1}`))
		if code := weatherErrorCode(t, err); code != CodeUpstreamFailure {
			t.Fatalf("geocode code = %q", code)
		}
	})

	t.Run("schema mentions required fields", func(t *testing.T) {
		tool, _ := NewWeatherTool(&fakeWeatherProvider{})
		schema := string(tool.InputSchema())
		for _, field := range []string{`"city"`, `"start_date"`, `"days"`, `"maximum": 7`} {
			if !strings.Contains(schema, field) {
				t.Fatalf("schema missing %s: %s", field, schema)
			}
		}
	})
}
