// Package app assembles optional integrations that the API and eval binaries
// share. It keeps plugin probing and status reporting out of main packages.
package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/config"
	"trippilot/internal/plugins"
	"trippilot/internal/repo"
	"trippilot/internal/tools"
)

const defaultHealthTimeout = time.Second

// Store is the persistence surface the research plugins need.
type Store interface {
	plugins.ToolCallStore
	plugins.EvidenceStore
	GetPluginCache(ctx context.Context, key string) (repo.PluginCache, error)
	SetPluginCache(ctx context.Context, arg repo.SetPluginCacheParams) error
	DeleteExpiredPluginCache(ctx context.Context) error
}

// Wiring is the plugin slice handed to agent.Nodes and api.Deps.
type Wiring struct {
	Registry *plugins.Registry
	Tools    []agent.ResearchTool
	Statuses []plugins.Status
	Disabled []agent.DisabledPlugin
	Cache    *plugins.RepoCache
}

type wireOptions struct {
	probe func(ctx context.Context, baseURL string) bool
}

type WireOption func(*wireOptions)

// WithHealthProbe overrides the hotel sidecar health check in tests.
func WithHealthProbe(probe func(ctx context.Context, baseURL string) bool) WireOption {
	return func(options *wireOptions) {
		if probe != nil {
			options.probe = probe
		}
	}
}

// WirePlugins builds the registry and wraps enabled tools with the Governor.
// An unavailable dependency only disables its plugin; it never fails startup.
func WirePlugins(ctx context.Context, cfg config.Config, store Store, events plugins.EventPublisher, logger *slog.Logger, opts ...WireOption) Wiring {
	if logger == nil {
		logger = slog.Default()
	}
	options := wireOptions{probe: probeHealth}
	for _, opt := range opts {
		opt(&options)
	}

	wiring := Wiring{Registry: plugins.NewRegistry()}
	wiring.Cache = plugins.NewRepoCache(store, plugins.WithCacheLogger(logger))
	governor := plugins.NewGovernor(wiring.Cache, store, store, events,
		plugins.WithGovernorTimeout(cfg.ResearchTimeout),
		plugins.WithGovernorMaxCalls(cfg.ResearchMaxToolCalls),
		plugins.WithGovernorToolTimeout("search_hotels", cfg.HotelSearchTimeout),
		plugins.WithGovernorToolTimeout("search_travel_notes", cfg.GuideSearchTimeout),
		plugins.WithGovernorToolTimeout("get_weather", cfg.WeatherToolTimeout),
		plugins.WithGovernorToolLimit("get_weather", cfg.WeatherMaxCallsPerRun),
		plugins.WithGovernorLogger(logger),
	)

	hotel, _ := wireHotel(ctx, cfg, options.probe)
	guide, _ := wireGuide(cfg)
	weather, _ := wireWeather(cfg)
	wiring.Registry = plugins.NewRegistry(hotel, guide, weather)
	for _, plugin := range []plugins.Plugin{hotel, guide, weather} {
		status := plugin.Status()
		wiring.Statuses = append(wiring.Statuses, status)
		if status.Enabled {
			continue
		}
		wiring.Disabled = append(wiring.Disabled, agent.DisabledPlugin{Name: plugin.Name(), Reason: status.DisabledReason})
		logger.Warn("research plugin disabled", "plugin", plugin.Name(), "reason", status.DisabledReason)
	}
	for _, tool := range wiring.Registry.ActiveTools() {
		wiring.Tools = append(wiring.Tools, governor.Wrap(tool))
	}
	return wiring
}

func wireHotel(ctx context.Context, cfg config.Config, probe func(context.Context, string) bool) (plugins.Plugin, plugins.Status) {
	const name = "search_hotels"
	status := plugins.Status{Name: "hotel"}
	if !cfg.HotelPluginEnabled {
		status.DisabledReason = "HOTEL_PLUGIN_ENABLED is false"
		return staticPlugin{name: name, status: status}, status
	}
	if strings.TrimSpace(cfg.HotelToolURL) == "" {
		status.DisabledReason = "hotel tool URL is not configured"
		return staticPlugin{name: name, status: status}, status
	}
	if !probe(ctx, cfg.HotelToolURL) {
		status.DisabledReason = "hotel tool health check failed"
		return staticPlugin{name: name, status: status}, status
	}
	tool, err := plugins.NewHotelTool(cfg.HotelToolURL, cfg.HotelToolToken,
		plugins.WithHotelHTTPClient(&http.Client{Timeout: cfg.HotelSearchTimeout}),
		plugins.WithHotelCacheTTL(cfg.HotelCacheTTL),
	)
	if err != nil {
		status.DisabledReason = "hotel tool configuration is invalid"
		return staticPlugin{name: name, status: status}, status
	}
	status.Enabled = true
	return staticPlugin{name: name, status: status, tools: []agent.ResearchTool{tool}}, status
}

func wireGuide(cfg config.Config) (plugins.Plugin, plugins.Status) {
	const name = "search_travel_notes"
	status := plugins.Status{Name: "guide"}
	if !cfg.GuidePluginEnabled {
		status.DisabledReason = "GUIDE_PLUGIN_ENABLED is false"
		return staticPlugin{name: name, status: status}, status
	}
	if strings.TrimSpace(cfg.GuideSearchAPIKey) == "" {
		status.DisabledReason = "GUIDE_SEARCH_API_KEY is empty"
		return staticPlugin{name: name, status: status}, status
	}
	tool, err := plugins.NewGuideTool(cfg.GuideSearchBaseURL, cfg.GuideSearchAPIKey,
		plugins.WithGuideHTTPClient(&http.Client{Timeout: cfg.GuideSearchTimeout}),
		plugins.WithGuideCacheTTL(cfg.GuideCacheTTL),
	)
	if err != nil {
		status.DisabledReason = "guide search configuration is invalid"
		return staticPlugin{name: name, status: status}, status
	}
	status.Enabled = true
	return staticPlugin{name: name, status: status, tools: []agent.ResearchTool{tool}}, status
}

func wireWeather(cfg config.Config) (plugins.Plugin, plugins.Status) {
	const name = "get_weather"
	status := plugins.Status{Name: "weather"}
	if strings.TrimSpace(cfg.AMapWebKey) == "" {
		status.DisabledReason = "AMAP_WEB_KEY is empty"
		return staticPlugin{name: name, status: status}, status
	}
	options := make([]tools.AMapOption, 0, 1)
	if cfg.WeatherToolTimeout > 0 {
		options = append(options, tools.WithAMapHTTPClient(&http.Client{Timeout: cfg.WeatherToolTimeout}))
	}
	provider := tools.NewAMapClient(cfg.AMapWebKey, options...)
	tool, err := plugins.NewWeatherTool(provider, plugins.WithWeatherCacheTTL(cfg.WeatherCacheTTL))
	if err != nil {
		status.DisabledReason = "weather tool configuration is invalid"
		return staticPlugin{name: name, status: status}, status
	}
	status.Enabled = true
	return staticPlugin{name: name, status: status, tools: []agent.ResearchTool{tool}}, status
}

func probeHealth(ctx context.Context, baseURL string) bool {
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultHealthTimeout)
	defer cancel()
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/healthz"
	request, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	return response.StatusCode == http.StatusOK
}

type staticPlugin struct {
	name   string
	status plugins.Status
	tools  []agent.ResearchTool
}

func (p staticPlugin) Name() string { return p.name }

func (p staticPlugin) Status() plugins.Status { return p.status }

func (p staticPlugin) Tools() []agent.ResearchTool { return p.tools }
