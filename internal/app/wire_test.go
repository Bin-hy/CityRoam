package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"trippilot/internal/config"
	"trippilot/internal/repo"
)

type fakeStore struct{}

func (fakeStore) InsertToolCall(context.Context, repo.InsertToolCallParams) (repo.ToolCall, error) {
	return repo.ToolCall{}, nil
}

func (fakeStore) InsertEvidence(context.Context, repo.InsertEvidenceParams) (repo.Evidence, error) {
	return repo.Evidence{}, nil
}

func (fakeStore) GetPluginCache(context.Context, string) (repo.PluginCache, error) {
	return repo.PluginCache{}, nil
}

func (fakeStore) SetPluginCache(context.Context, repo.SetPluginCacheParams) error { return nil }

func (fakeStore) DeleteExpiredPluginCache(context.Context) error { return nil }

type fakeEvents struct{}

func (fakeEvents) Publish(context.Context, string, string, any) error { return nil }

func testConfig() config.Config {
	return config.Config{
		HotelSearchTimeout:   time.Second,
		HotelCacheTTL:        time.Minute,
		GuideSearchTimeout:   time.Second,
		GuideCacheTTL:        time.Minute,
		ResearchTimeout:      5 * time.Second,
		ResearchMaxToolCalls: 6,
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestWirePlugins(t *testing.T) {
	t.Run("disabled plugins expose no tools", func(t *testing.T) {
		cfg := testConfig()
		wiring := WirePlugins(context.Background(), cfg, fakeStore{}, fakeEvents{}, testLogger())

		if len(wiring.Tools) != 0 || len(wiring.Registry.ActiveTools()) != 0 {
			t.Fatalf("active tools = %+v", wiring.Tools)
		}
		if len(wiring.Statuses) != 3 {
			t.Fatalf("statuses = %+v", wiring.Statuses)
		}
		if wiring.Statuses[0].Name != "hotel" || wiring.Statuses[1].Name != "guide" || wiring.Statuses[2].Name != "weather" {
			t.Fatalf("status names = %+v", wiring.Statuses)
		}
		for _, status := range wiring.Statuses {
			if status.Enabled {
				t.Fatalf("status enabled: %+v", status)
			}
			if status.DisabledReason == "" {
				t.Fatalf("missing reason: %+v", status)
			}
		}
		if len(wiring.Disabled) != 3 {
			t.Fatalf("disabled prompt hints = %+v", wiring.Disabled)
		}
		if wiring.Statuses[2].DisabledReason != "AMAP_WEB_KEY is empty" {
			t.Fatalf("weather reason = %q", wiring.Statuses[2].DisabledReason)
		}
	})

	t.Run("healthy hotel plugin is registered", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				t.Fatalf("probe path = %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.HotelPluginEnabled = true
		cfg.HotelToolURL = server.URL
		wiring := WirePlugins(context.Background(), cfg, fakeStore{}, fakeEvents{}, testLogger())

		if len(wiring.Tools) != 1 || wiring.Tools[0].Name() != "search_hotels" {
			t.Fatalf("tools = %+v", wiring.Tools)
		}
		if !wiring.Statuses[0].Enabled || wiring.Statuses[1].Enabled || wiring.Statuses[2].Enabled {
			t.Fatalf("statuses = %+v", wiring.Statuses)
		}
		if len(wiring.Disabled) != 2 || wiring.Disabled[0].Name != "search_travel_notes" || wiring.Disabled[1].Name != "get_weather" {
			t.Fatalf("disabled = %+v", wiring.Disabled)
		}
	})

	t.Run("failed or missing dependencies stay disabled without leaking", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		cfg := testConfig()
		cfg.HotelPluginEnabled = true
		cfg.HotelToolURL = server.URL
		cfg.HotelToolToken = "hotel-secret-token"
		cfg.GuidePluginEnabled = true
		cfg.GuideSearchBaseURL = "https://guide.internal.example"
		cfg.GuideSearchAPIKey = ""
		wiring := WirePlugins(context.Background(), cfg, fakeStore{}, fakeEvents{}, testLogger())

		if len(wiring.Tools) != 0 {
			t.Fatalf("tools = %+v", wiring.Tools)
		}
		if wiring.Statuses[0].Enabled || wiring.Statuses[1].Enabled || wiring.Statuses[2].Enabled {
			t.Fatalf("statuses = %+v", wiring.Statuses)
		}
		if !strings.Contains(wiring.Statuses[0].DisabledReason, "health") {
			t.Fatalf("hotel reason = %q", wiring.Statuses[0].DisabledReason)
		}
		if !strings.Contains(wiring.Statuses[1].DisabledReason, "GUIDE_SEARCH_API_KEY") {
			t.Fatalf("guide reason = %q", wiring.Statuses[1].DisabledReason)
		}
		if !strings.Contains(wiring.Statuses[2].DisabledReason, "AMAP_WEB_KEY") {
			t.Fatalf("weather reason = %q", wiring.Statuses[2].DisabledReason)
		}
		encoded, _ := json.Marshal(wiring.Statuses)
		for _, leak := range []string{server.URL, "hotel-secret-token", "guide.internal.example"} {
			if strings.Contains(string(encoded), leak) {
				t.Fatalf("status leaks %q: %s", leak, encoded)
			}
		}
	})

	t.Run("guide plugin registers when configured", func(t *testing.T) {
		cfg := testConfig()
		cfg.GuidePluginEnabled = true
		cfg.GuideSearchBaseURL = "https://api.example.com"
		cfg.GuideSearchAPIKey = "guide-key"
		cfg.GuideCacheTTL = time.Minute
		wiring := WirePlugins(context.Background(), cfg, fakeStore{}, fakeEvents{}, testLogger())

		if len(wiring.Tools) != 1 || wiring.Tools[0].Name() != "search_travel_notes" {
			t.Fatalf("tools = %+v", wiring.Tools)
		}
		if !wiring.Statuses[1].Enabled {
			t.Fatalf("statuses = %+v", wiring.Statuses)
		}
	})

	t.Run("weather plugin registers when the map key is present", func(t *testing.T) {
		cfg := testConfig()
		cfg.AMapWebKey = "map-key"
		cfg.WeatherCacheTTL = time.Minute
		cfg.WeatherToolTimeout = time.Second
		cfg.WeatherMaxCallsPerRun = 1
		wiring := WirePlugins(context.Background(), cfg, fakeStore{}, fakeEvents{}, testLogger())

		if len(wiring.Tools) != 1 || wiring.Tools[0].Name() != "get_weather" {
			t.Fatalf("tools = %+v", wiring.Tools)
		}
		if !wiring.Statuses[2].Enabled || wiring.Statuses[2].DisabledReason != "" {
			t.Fatalf("weather status = %+v", wiring.Statuses[2])
		}
		if len(wiring.Disabled) != 2 {
			t.Fatalf("disabled = %+v", wiring.Disabled)
		}
	})
}
