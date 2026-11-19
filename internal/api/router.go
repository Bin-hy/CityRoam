package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"trippilot/internal/events"
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

	router := chi.NewRouter()
	router.Use(RequestID)
	router.Use(Recover(deps.Logger))
	router.Use(BodyLimit(deps.MaxBodyBytes))

	router.Get("/healthz", handleHealth)
	router.Get("/readyz", handleReady(deps))
	router.Get("/api/meta", handleMeta(deps))

	router.Route("/api", func(api chi.Router) {
		api.Group(func(public chi.Router) {
			public.Use(RateLimit(deps.RateLimitPerHour, deps.Now))
			public.Post("/trips", handleCreateTrip(deps))
			public.Delete("/trips/{tripID}", handleDeleteTrip(deps))
			public.Post("/trips/{tripID}/adjustments", handleCreateAdjustment(deps))
			public.Post("/trips/{tripID}/share", handleCreateShare(deps))
			public.Post("/runs/{runID}/resume", handleResumeRun(deps))
			public.Post("/runs/{runID}/cancel", handleCancelRun(deps))
		})

		api.Get("/trips/{tripID}", handleGetTrip(deps))
		api.Get("/trips/{tripID}/versions", handleListVersions(deps))
		api.Get("/trips/{tripID}/versions/{seq}", handleGetVersion(deps))
		api.Get("/trips/{tripID}/diff", handleDiff(deps))

		api.Get("/runs/{runID}", handleNotImplemented())
		api.Get("/runs/{runID}/trace", handleNotImplemented())
		api.Get("/runs/{runID}/events", handleRunEvents(deps))

		api.Get("/share/{slug}", handleGetShare(deps))

		api.Route("/eval", func(eval chi.Router) {
			eval.Use(AdminToken(deps.AdminToken))
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
	}
	payload := meta{Version: deps.Version, Cities: []string{"beijing"}}
	payload.Defaults.Days = 2
	payload.Defaults.Pace = "normal"
	payload.Defaults.Currency = "CNY"
	payload.Defaults.MaxTripDays = 4
	payload.RateLimit.PerHour = deps.RateLimitPerHour
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, payload)
	}
}
