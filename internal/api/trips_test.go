package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"trippilot/internal/repo"
)

type fakeTripStore struct {
	mu       sync.Mutex
	trips    map[string]repo.Trip
	runs     map[string][]repo.Run
	versions map[string][]repo.PlanVersion
	byReq    map[string]string
	deleted  map[string]bool
	next     int
}

func newFakeTripStore() *fakeTripStore {
	return &fakeTripStore{
		trips:    make(map[string]repo.Trip),
		runs:     make(map[string][]repo.Run),
		versions: make(map[string][]repo.PlanVersion),
		byReq:    make(map[string]string),
		deleted:  make(map[string]bool),
	}
}

func (s *fakeTripStore) CreateTripAndRun(_ context.Context, tripArg repo.CreateTripParams, runArg repo.CreateRunParams) (repo.Trip, repo.Run, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tripArg.RequestID != nil {
		if id, ok := s.byReq[*tripArg.RequestID]; ok {
			runs := s.runs[id]
			if len(runs) == 0 {
				return repo.Trip{}, repo.Run{}, false, fmt.Errorf("missing run")
			}
			return s.trips[id], runs[0], false, nil
		}
	}
	s.next++
	now := time.Unix(1700000000, 0).UTC()
	trip := repo.Trip{
		ID:          fmt.Sprintf("trip-%d", s.next),
		City:        tripArg.City,
		Days:        tripArg.Days,
		Constraints: tripArg.Constraints,
		Status:      tripArg.Status,
		RequestID:   tripArg.RequestID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	runArg.TripID = trip.ID
	run := repo.Run{
		ID:          fmt.Sprintf("run-%d", s.next),
		TripID:      trip.ID,
		Mode:        runArg.Mode,
		Status:      runArg.Status,
		Stage:       runArg.Stage,
		Input:       runArg.Input,
		Constraints: runArg.Constraints,
		RequestID:   runArg.RequestID,
		CreatedAt:   now,
	}
	s.trips[trip.ID] = trip
	s.runs[trip.ID] = append([]repo.Run{run}, s.runs[trip.ID]...)
	if tripArg.RequestID != nil {
		s.byReq[*tripArg.RequestID] = trip.ID
	}
	return trip, run, true, nil
}

func (s *fakeTripStore) GetTrip(_ context.Context, id string) (repo.Trip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	trip, ok := s.trips[id]
	if !ok || s.deleted[id] {
		return repo.Trip{}, pgx.ErrNoRows
	}
	return trip, nil
}

func (s *fakeTripStore) SoftDeleteTrip(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted[id] = true
	return nil
}

func (s *fakeTripStore) ListRunsByTrip(_ context.Context, tripID string) ([]repo.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]repo.Run{}, s.runs[tripID]...), nil
}

func (s *fakeTripStore) ListPlanVersions(_ context.Context, tripID string) ([]repo.PlanVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]repo.PlanVersion{}, s.versions[tripID]...), nil
}

type fakeRunner struct {
	mu      sync.Mutex
	started []string
	keys    []string
}

func (r *fakeRunner) Start(_ context.Context, runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = append(r.started, runID)
	return nil
}

func (r *fakeRunner) StartRequest(_ context.Context, runID, requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = append(r.started, runID)
	r.keys = append(r.keys, requestID)
	return nil
}

func tripRouter(store TripStore, runner RunStarter) http.Handler {
	return NewRouter(Deps{
		Version:          "test",
		Logger:           discardLogger(),
		AdminToken:       "secret",
		RateLimitPerHour: 100,
		Now:              func() time.Time { return time.Unix(1700000000, 0) },
		Store:            store,
		Runner:           runner,
	})
}

func TestTrips(t *testing.T) {
	t.Run("creates a trip and starts the run", func(t *testing.T) {
		store := newFakeTripStore()
		runner := &fakeRunner{}
		router := tripRouter(store, runner)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/trips", strings.NewReader(`{"input":"北京两天，预算800元"}`))
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		var body createTripResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.TripID == "" || body.RunID == "" || body.EventsURL != "/api/runs/"+body.RunID+"/events" {
			t.Fatalf("body = %+v", body)
		}
		trip := store.trips[body.TripID]
		if trip.City != "beijing" || trip.Days != 2 || trip.Status != "planning" {
			t.Fatalf("trip = %+v", trip)
		}
		if len(runner.started) != 1 || runner.started[0] != body.RunID {
			t.Fatalf("started = %v", runner.started)
		}
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		router := tripRouter(newFakeTripStore(), &fakeRunner{})
		cases := []struct {
			name string
			body string
		}{
			{"empty", `{"input":"  "}`},
			{"city", `{"input":"上海两天","city":"shanghai"}`},
			{"bad json", `{"input":`},
			{"too long", `{"input":"` + strings.Repeat("京", maxInputRunes+1) + `"}`},
		}
		for _, tc := range cases {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips", strings.NewReader(tc.body)))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s: status = %d body=%s", tc.name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("idempotent request returns the existing trip", func(t *testing.T) {
		store := newFakeTripStore()
		runner := &fakeRunner{}
		router := tripRouter(store, runner)
		body := `{"input":"北京三天","client_request_id":"abc"}`

		first := httptest.NewRecorder()
		router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/trips", strings.NewReader(body)))
		second := httptest.NewRecorder()
		router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/trips", strings.NewReader(body)))

		var firstBody, secondBody createTripResponse
		_ = json.Unmarshal(first.Body.Bytes(), &firstBody)
		_ = json.Unmarshal(second.Body.Bytes(), &secondBody)
		if firstBody.TripID != secondBody.TripID || firstBody.RunID != secondBody.RunID {
			t.Fatalf("first=%+v second=%+v", firstBody, secondBody)
		}
		if len(runner.started) != 1 {
			t.Fatalf("started = %v", runner.started)
		}
		if store.trips[firstBody.TripID].Days != 3 {
			t.Fatalf("days = %d", store.trips[firstBody.TripID].Days)
		}
	})

	t.Run("gets the summary with the latest version", func(t *testing.T) {
		store := newFakeTripStore()
		_, run, _, _ := store.CreateTripAndRun(context.Background(), repo.CreateTripParams{
			City: "beijing", Days: 1, Status: "planning",
		}, repo.CreateRunParams{Mode: "initial", Status: "awaiting_user", Stage: "present"})
		store.versions[run.TripID] = []repo.PlanVersion{{
			ID: "v1", TripID: run.TripID, RunID: run.ID, Seq: 1,
			Plan: json.RawMessage(`{"city":"beijing"}`), Status: "accepted",
		}}
		router := tripRouter(store, &fakeRunner{})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+run.TripID, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var body tripResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.LatestRun == nil || body.LatestRun.ID != run.ID {
			t.Fatalf("latest run = %+v", body.LatestRun)
		}
		if body.LatestVersion == nil || body.LatestVersion.Seq != 1 {
			t.Fatalf("latest version = %+v", body.LatestVersion)
		}
	})

	t.Run("missing trip is 404 and delete is idempotent", func(t *testing.T) {
		store := newFakeTripStore()
		router := tripRouter(store, &fakeRunner{})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/nope", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d", rec.Code)
		}

		trip, _, _, _ := store.CreateTripAndRun(context.Background(), repo.CreateTripParams{
			City: "beijing", Days: 1, Status: "planning",
		}, repo.CreateRunParams{Mode: "initial", Status: "queued", Stage: "admit"})
		for i := 0; i < 2; i++ {
			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/trips/"+trip.ID, nil))
			if rec.Code != http.StatusNoContent {
				t.Fatalf("delete status = %d", rec.Code)
			}
		}
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+trip.ID, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("deleted get status = %d", rec.Code)
		}
	})
}
