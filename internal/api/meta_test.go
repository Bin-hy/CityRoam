package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"trippilot/internal/app"
	"trippilot/internal/config"
	"trippilot/internal/plugins"
	"trippilot/internal/repo"
	"trippilot/internal/skills"
)

func baseRouter(deps Deps) http.Handler {
	if deps.Version == "" {
		deps.Version = "test"
	}
	if deps.Logger == nil {
		deps.Logger = discardLogger()
	}
	if deps.RateLimitPerHour == 0 {
		deps.RateLimitPerHour = 100
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Unix(1700000000, 0) }
	}
	if deps.Store == nil {
		deps.Store = newFakeTripStore()
	}
	if deps.Runner == nil {
		deps.Runner = &fakeRunner{}
	}
	return NewRouter(deps)
}

func TestHealth(t *testing.T) {
	t.Run("liveness always reports ok", func(t *testing.T) {
		router := baseRouter(Deps{})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if body := rec.Body.String(); body != `{"status":"ok"}`+"\n" {
			t.Fatalf("body = %q", body)
		}
	})

	t.Run("readiness follows dependencies", func(t *testing.T) {
		var failure error
		router := baseRouter(Deps{Ready: func(context.Context) error { return failure }})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("ready status = %d", rec.Code)
		}

		failure = io.ErrUnexpectedEOF
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("not ready status = %d", rec.Code)
		}
		if body := decodeError(t, rec); body.Code != "not_ready" || body.RequestID == "" {
			t.Fatalf("error = %+v", body)
		}
	})
}

func TestMeta(t *testing.T) {
	router := baseRouter(Deps{Version: "1.2.3", RateLimitPerHour: 10})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Version  string   `json:"version"`
		Cities   []string `json:"cities"`
		Defaults struct {
			Days        int    `json:"days"`
			Pace        string `json:"pace"`
			Currency    string `json:"currency"`
			MaxTripDays int    `json:"max_trip_days"`
		} `json:"defaults"`
		RateLimit struct {
			PerHour int `json:"per_hour"`
		} `json:"rate_limit"`
		Plugins struct {
			Hotels  pluginMeta `json:"hotels"`
			Guides  pluginMeta `json:"guides"`
			Weather pluginMeta `json:"weather"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Version != "1.2.3" || len(body.Cities) != 1 || body.Cities[0] != "beijing" {
		t.Fatalf("body = %+v", body)
	}
	if body.Defaults.Days != 2 || body.Defaults.Pace != "normal" || body.Defaults.Currency != "CNY" || body.Defaults.MaxTripDays != 4 {
		t.Fatalf("defaults = %+v", body.Defaults)
	}
	if body.RateLimit.PerHour != 10 {
		t.Fatalf("rate limit = %+v", body.RateLimit)
	}
	if body.Plugins.Hotels.Enabled || body.Plugins.Guides.Enabled || body.Plugins.Weather.Enabled {
		t.Fatalf("plugins default enabled: %+v", body.Plugins)
	}
	if body.Plugins.Hotels.DisabledReason != "" || body.Plugins.Guides.DisabledReason != "" || body.Plugins.Weather.DisabledReason != "" {
		t.Fatalf("plugins default reasons: %+v", body.Plugins)
	}
}

type pluginMeta struct {
	Enabled        bool   `json:"enabled"`
	DisabledReason string `json:"disabled_reason"`
}

func TestMetaPluginStatuses(t *testing.T) {
	cfg := config.Config{
		HotelPluginEnabled:    true,
		HotelToolURL:          "http://hotel-tool:8090",
		HotelSearchTimeout:    time.Second,
		HotelCacheTTL:         time.Minute,
		GuidePluginEnabled:    true,
		GuideSearchBaseURL:    "https://guide.example.com",
		GuideSearchAPIKey:     "test-guide-key",
		GuideSearchTimeout:    time.Second,
		GuideCacheTTL:         time.Minute,
		AMapWebKey:            "test-map-key",
		WeatherCacheTTL:       time.Minute,
		WeatherToolTimeout:    time.Second,
		WeatherMaxCallsPerRun: 1,
		ResearchTimeout:       time.Second,
		ResearchMaxToolCalls:  2,
	}
	wiring := app.WirePlugins(context.Background(), cfg, wireTestStore{}, wireTestEvents{}, discardLogger(),
		app.WithHealthProbe(func(context.Context, string) bool { return true }))
	if len(wiring.Statuses) != 3 {
		t.Fatalf("statuses = %+v", wiring.Statuses)
	}

	router := baseRouter(Deps{
		Version:        "1.2.3",
		PluginStatuses: append(wiring.Statuses, plugins.Status{Name: "unrelated", Enabled: true}),
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var body struct {
		Version string `json:"version"`
		Plugins struct {
			Hotels  pluginMeta `json:"hotels"`
			Guides  pluginMeta `json:"guides"`
			Weather pluginMeta `json:"weather"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Plugins.Hotels.Enabled || body.Plugins.Hotels.DisabledReason != "" {
		t.Fatalf("hotels = %+v", body.Plugins.Hotels)
	}
	if !body.Plugins.Guides.Enabled || body.Plugins.Guides.DisabledReason != "" {
		t.Fatalf("guides = %+v", body.Plugins.Guides)
	}
	if !body.Plugins.Weather.Enabled || body.Plugins.Weather.DisabledReason != "" {
		t.Fatalf("weather = %+v", body.Plugins.Weather)
	}
	raw := rec.Body.String()
	for _, leak := range []string{"http://", "https://", "sk-", "API_KEY=", "test-guide-key"} {
		if strings.Contains(raw, leak) {
			t.Fatalf("meta leaks %q: %s", leak, raw)
		}
	}
}

func TestMetaSkillPins(t *testing.T) {
	router := baseRouter(Deps{
		Version: "1.2.3",
		SkillCatalog: func() []skills.CatalogEntry {
			return []skills.CatalogEntry{{
				Name: "research-playbook", Version: "1.0.0", Description: "研究清单", SHA256: strings.Repeat("a", 64),
			}}
		},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var body struct {
		Skills []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			SHA256  string `json:"sha256"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Skills) != 1 || body.Skills[0].Name != "research-playbook" || body.Skills[0].Version != "1.0.0" {
		t.Fatalf("skills = %+v", body.Skills)
	}
	if strings.Contains(rec.Body.String(), "研究清单") {
		t.Fatalf("meta leaked skill body/description: %s", rec.Body.String())
	}
}

type wireTestStore struct{}

func (wireTestStore) InsertToolCall(context.Context, repo.InsertToolCallParams) (repo.ToolCall, error) {
	return repo.ToolCall{}, nil
}

func (wireTestStore) InsertEvidence(context.Context, repo.InsertEvidenceParams) (repo.Evidence, error) {
	return repo.Evidence{}, nil
}

func (wireTestStore) GetPluginCache(context.Context, string) (repo.PluginCache, error) {
	return repo.PluginCache{}, io.EOF
}

func (wireTestStore) SetPluginCache(context.Context, repo.SetPluginCacheParams) error { return nil }

func (wireTestStore) DeleteExpiredPluginCache(context.Context) error { return nil }

type wireTestEvents struct{}

func (wireTestEvents) Publish(context.Context, string, string, any) error { return nil }

func TestMetaSkillCatalogRefreshesPerRequest(t *testing.T) {
	catalog := []skills.CatalogEntry{{Name: "alpha", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}}
	router := baseRouter(Deps{
		Version:      "1.2.3",
		SkillCatalog: func() []skills.CatalogEntry { return catalog },
	})

	decode := func() []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var body struct {
			Skills []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"skills"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body.Skills
	}

	if got := decode(); len(got) != 1 || got[0].Version != "1.0.0" {
		t.Fatalf("initial skills = %+v", got)
	}
	catalog = []skills.CatalogEntry{{Name: "alpha", Version: "1.1.0", SHA256: strings.Repeat("b", 64)}}
	if got := decode(); len(got) != 1 || got[0].Version != "1.1.0" {
		t.Fatalf("refreshed skills = %+v", got)
	}
	catalog = nil
	if got := decode(); len(got) != 0 {
		t.Fatalf("empty catalog = %+v", got)
	}
}
