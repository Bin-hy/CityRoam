package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"trippilot/internal/events"
	"trippilot/internal/plugins"
	"trippilot/internal/skills"
)

const defaultMaxBodyBytes = 64 << 10

// Deps wires the HTTP layer to the store, broker, runner and configuration.
type Deps struct {
	Version          string
	Logger           *slog.Logger
	AdminToken       string
	RateLimitPerHour int
	MaxBodyBytes     int64
	Now              func() time.Time
	Ready            func(ctx context.Context) error
	Broker           *events.Broker
	Store            TripStore
	Runner           RunStarter
	Eval             EvalService
	PluginStatuses   []plugins.Status
	SkillCatalog     func() []skills.CatalogEntry
	Auth             Authenticator
	AllowedOrigins   []string
}

// NewRouter builds the public API. Handlers are filled in per endpoint card;
// the routing table and middleware order are stable.
func NewRouter(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.MaxBodyBytes <= 0 {
		deps.MaxBodyBytes = defaultMaxBodyBytes
	}
	if deps.Auth == nil {
		deps.Auth = disabledAuth{adminToken: deps.AdminToken}
	}

	router := chi.NewRouter()
	router.Use(RequestID)
	router.Use(RequestLogger(deps.Logger))
	router.Use(Recover(deps.Logger))
	router.Use(BodyLimit(deps.MaxBodyBytes))
	router.Use(CORS(deps.AllowedOrigins))
	router.Use(SameOrigin(deps.AllowedOrigins, deps.Auth.Enabled()))

	router.Get("/healthz", handleHealth)
	router.Get("/readyz", handleReady(deps))
	router.Get("/api/meta", handleMeta(deps))

	router.Route("/api", func(api chi.Router) {
		api.Group(func(public chi.Router) {
			public.Use(RateLimit(deps.RateLimitPerHour, deps.Now))
			public.Get("/auth/me", deps.Auth.Me)
			public.Get("/auth/login", deps.Auth.Login)
			public.Get("/auth/callback", deps.Auth.Callback)
			public.Post("/auth/logout", deps.Auth.Logout)
			public.Get("/share/{slug}", handleGetShare(deps))
		})

		api.Group(func(protected chi.Router) {
			protected.Use(deps.Auth.Middleware)

			protected.Get("/trips", handleListTrips(deps))
			protected.With(TripOwner(deps)).Get("/trips/{tripID}", handleGetTrip(deps))
			protected.With(TripOwner(deps)).Get("/trips/{tripID}/versions", handleListVersions(deps))
			protected.With(TripOwner(deps)).Get("/trips/{tripID}/versions/{seq}", handleGetVersion(deps))
			protected.With(TripOwner(deps)).Get("/trips/{tripID}/diff", handleDiff(deps))

			protected.With(RunOwner(deps)).Get("/runs/{runID}", handleNotImplemented())
			protected.With(RunOwner(deps)).Get("/runs/{runID}/trace", handleNotImplemented())
			protected.With(RunOwner(deps)).Get("/runs/{runID}/events", handleRunEvents(deps))

			protected.Group(func(write chi.Router) {
				write.Use(RateLimitBy(deps.RateLimitPerHour, deps.Now, principalRateKey))
				write.Post("/trips", handleCreateTrip(deps))
				write.With(TripOwner(deps)).Delete("/trips/{tripID}", handleDeleteTrip(deps))
				write.With(TripOwner(deps)).Post("/trips/{tripID}/adjustments", handleCreateAdjustment(deps))
				write.With(TripOwner(deps)).Post("/trips/{tripID}/replan", handleReplan(deps))
				write.With(TripOwner(deps)).Post("/trips/{tripID}/versions/{seq}/restore", handleRestoreVersion(deps))
				write.With(TripOwner(deps)).Post("/trips/{tripID}/share", handleCreateShare(deps))
				write.With(RunOwner(deps)).Post("/runs/{runID}/resume", handleResumeRun(deps))
				write.With(RunOwner(deps)).Post("/runs/{runID}/cancel", handleCancelRun(deps))
			})
		})

		api.Route("/eval", func(eval chi.Router) {
			eval.Use(deps.Auth.AdminOrToken(deps.AdminToken))
			eval.Get("/cases", handleListEvalCases(deps))
			eval.Post("/runs", handleStartEval(deps))
			eval.Get("/runs", handleListEvalRuns(deps))
		})
	})

	return router
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleReady(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Ready != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := deps.Ready(ctx); err != nil {
				logError(deps.Logger, "readiness check failed", r, err)
				WriteError(w, r, &Error{
					Status:  http.StatusServiceUnavailable,
					Code:    "not_ready",
					Message: "依赖服务尚未就绪",
				})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func handleMeta(deps Deps) http.HandlerFunc {
	type pluginMeta struct {
		Enabled        bool   `json:"enabled"`
		DisabledReason string `json:"disabled_reason,omitempty"`
	}
	type skillMeta struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		SHA256  string `json:"sha256"`
	}
	type meta struct {
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
		Skills []skillMeta `json:"skills,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		payload := meta{Version: deps.Version, Cities: []string{"beijing"}}
		payload.Defaults.Days = 2
		payload.Defaults.Pace = "normal"
		payload.Defaults.Currency = "CNY"
		payload.Defaults.MaxTripDays = 4
		payload.RateLimit.PerHour = deps.RateLimitPerHour
		for _, status := range deps.PluginStatuses {
			item := pluginMeta{Enabled: status.Enabled, DisabledReason: status.DisabledReason}
			switch strings.ToLower(status.Name) {
			case "hotel", "hotels":
				payload.Plugins.Hotels = item
			case "guide", "guides":
				payload.Plugins.Guides = item
			case "weather":
				payload.Plugins.Weather = item
			}
		}
		if deps.SkillCatalog != nil {
			for _, entry := range deps.SkillCatalog() {
				payload.Skills = append(payload.Skills, skillMeta{
					Name:    entry.Name,
					Version: entry.Version,
					SHA256:  entry.SHA256,
				})
			}
		}
		writeJSON(w, http.StatusOK, payload)
	}
}
