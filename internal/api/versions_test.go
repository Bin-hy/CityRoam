package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
	"trippilot/internal/repo"
)

func versionPlan(cost int) domain.Itinerary {
	return domain.Itinerary{
		City: "beijing",
		Days: []domain.DayPlan{{
			Date:      "2026-11-20",
			StartTime: "09:00",
			EndTime:   "11:30",
			Blocks: []domain.Block{{
				ID: "b1", Type: domain.BlockTypePOI, POIID: "poi-0",
				Title: "景点0", Start: "09:00", End: "10:30", DurationMin: 90, CostYuan: cost,
			}},
			DayCostYuan: cost,
		}},
		Totals: domain.Totals{CostYuan: cost, POICount: 1, WalkingKM: 1.5},
	}
}

func versionFixture(t *testing.T, plans ...domain.Itinerary) (*fakeTripStore, repo.Trip, []repo.PlanVersion) {
	t.Helper()
	store := newFakeTripStore()
	trip, run, _, err := store.CreateTripAndRun(context.Background(), repo.CreateTripParams{
		City: "beijing", Days: 1, Status: "planning",
	}, repo.CreateRunParams{Mode: "initial", Status: "accepted", Stage: "present"})
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}
	constraints, _ := json.Marshal(domain.ConstraintSet{City: "beijing", Days: 1})
	versions := make([]repo.PlanVersion, 0, len(plans))
	for index, plan := range plans {
		encoded, err := json.Marshal(plan)
		if err != nil {
			t.Fatalf("marshal plan: %v", err)
		}
		versions = append(versions, repo.PlanVersion{
			ID:          "v" + strings.Repeat("x", index+1),
			TripID:      trip.ID,
			RunID:       run.ID,
			Seq:         int32(index + 1),
			Plan:        encoded,
			Constraints: constraints,
			Violations:  json.RawMessage(`[]`),
			Status:      "accepted",
		})
	}
	for left, right := 0, len(versions)-1; left < right; left, right = left+1, right-1 {
		versions[left], versions[right] = versions[right], versions[left]
	}
	store.versions[trip.ID] = versions
	return store, trip, versions
}

func TestVersions(t *testing.T) {
	t.Run("lists and returns versions", func(t *testing.T) {
		store, trip, versions := versionFixture(t, versionPlan(30), versionPlan(50))
		router := tripRouter(store, &fakeRunner{})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+trip.ID+"/versions", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("list status = %d body=%s", rec.Code, rec.Body.String())
		}
		var list struct {
			Versions []repo.PlanVersion `json:"versions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		if len(list.Versions) != 2 || list.Versions[0].Seq != 2 {
			t.Fatalf("versions = %+v", list.Versions)
		}

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+trip.ID+"/versions/1", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("detail status = %d body=%s", rec.Code, rec.Body.String())
		}
		var version repo.PlanVersion
		if err := json.Unmarshal(rec.Body.Bytes(), &version); err != nil {
			t.Fatalf("decode version: %v", err)
		}
		if version.Seq != 1 || version.ID != versions[1].ID {
			t.Fatalf("version = %+v", version)
		}

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+trip.ID+"/versions/0", nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid seq status = %d", rec.Code)
		}
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+trip.ID+"/versions/9", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("missing seq status = %d", rec.Code)
		}
	})

	t.Run("diffs the latest two versions by default", func(t *testing.T) {
		store, trip, _ := versionFixture(t, versionPlan(30), versionPlan(50))
		router := tripRouter(store, &fakeRunner{})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+trip.ID+"/diff", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		var body struct {
			From int32 `json:"from"`
			To   int32 `json:"to"`
			Diff struct {
				Days []struct {
					DayIndex int    `json:"day_index"`
					Change   string `json:"change"`
				} `json:"days"`
			} `json:"diff"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.From != 1 || body.To != 2 {
			t.Fatalf("range = %d..%d", body.From, body.To)
		}
		if len(body.Diff.Days) != 1 || body.Diff.Days[0].DayIndex != 0 || body.Diff.Days[0].Change != "modified" {
			t.Fatalf("diff = %+v", body.Diff)
		}
	})

	t.Run("diff requires two versions and validates the range", func(t *testing.T) {
		store, trip, _ := versionFixture(t, versionPlan(30))
		router := tripRouter(store, &fakeRunner{})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+trip.ID+"/diff", nil))
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+trip.ID+"/diff?from=0&to=1", nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid range status = %d", rec.Code)
		}
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+trip.ID+"/diff?from=1&to=9", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("missing version status = %d", rec.Code)
		}
	})
}

func TestAdjustments(t *testing.T) {
	baseState := func(t *testing.T, store *fakeTripStore, trip repo.Trip) *agent.TripState {
		t.Helper()
		runs := store.runs[trip.ID]
		if len(runs) == 0 {
			t.Fatal("missing adjustment run")
		}
		state, err := agent.DecodeTripState(runs[0].State)
		if err != nil {
			t.Fatalf("decode state: %v", err)
		}
		return state
	}

	t.Run("creates a queued adjustment run from the base version", func(t *testing.T) {
		store, trip, _ := versionFixture(t, versionPlan(30))
		runner := &fakeRunner{}
		router := tripRouter(store, runner)

		rec := httptest.NewRecorder()
		body := `{"input":"第二天少走路","base_version":1,"client_request_id":"adj-1"}`
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+trip.ID+"/adjustments", strings.NewReader(body)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		var created adjustmentResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if created.RunID == "" || created.EventsURL != "/api/runs/"+created.RunID+"/events" {
			t.Fatalf("body = %+v", created)
		}

		run := store.runs[trip.ID][0]
		if run.ID != created.RunID || run.Mode != "adjustment" || run.Stage != "adjust" || run.Status == "" {
			t.Fatalf("run = %+v", run)
		}
		if run.BaseVersionID == nil || *run.BaseVersionID != "vx" {
			t.Fatalf("base version = %v", run.BaseVersionID)
		}
		if run.RequestID == nil || *run.RequestID != "adj-1" {
			t.Fatalf("request id = %v", run.RequestID)
		}
		if len(runner.started) != 1 || runner.started[0] != run.ID || runner.keys[0] != "adj-1" {
			t.Fatalf("runner = %+v %+v", runner.started, runner.keys)
		}

		state := baseState(t, store, trip)
		if state.Mode != "adjustment" || state.Plan.City != "beijing" || state.Previous == nil {
			t.Fatalf("state = %+v", state)
		}
		if state.PreviousSeq != 1 || state.Constraints.City != "beijing" || state.Input != "第二天少走路" {
			t.Fatalf("state = %+v", state)
		}
		if len(state.Draft.Days) != 1 || len(state.Draft.Days[0].Items) != 1 || state.Draft.Days[0].Items[0].POIID != "poi-0" {
			t.Fatalf("draft = %+v", state.Draft)
		}
	})

	t.Run("returns the existing run for a repeated request id", func(t *testing.T) {
		store, trip, _ := versionFixture(t, versionPlan(30))
		runner := &fakeRunner{}
		router := tripRouter(store, runner)
		body := `{"input":"少走路","base_version":1,"client_request_id":"adj-2"}`

		first := httptest.NewRecorder()
		router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/trips/"+trip.ID+"/adjustments", strings.NewReader(body)))
		second := httptest.NewRecorder()
		router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/trips/"+trip.ID+"/adjustments", strings.NewReader(body)))
		if first.Code != http.StatusAccepted || second.Code != http.StatusAccepted {
			t.Fatalf("status = %d/%d", first.Code, second.Code)
		}
		var firstBody, secondBody adjustmentResponse
		_ = json.Unmarshal(first.Body.Bytes(), &firstBody)
		_ = json.Unmarshal(second.Body.Bytes(), &secondBody)
		if firstBody.RunID != secondBody.RunID {
			t.Fatalf("runs = %s/%s", firstBody.RunID, secondBody.RunID)
		}
		if len(runner.started) != 1 {
			t.Fatalf("started = %v", runner.started)
		}
	})

	t.Run("rejects invalid requests and missing versions", func(t *testing.T) {
		store, trip, _ := versionFixture(t, versionPlan(30))
		router := tripRouter(store, &fakeRunner{})

		cases := []struct {
			name   string
			body   string
			status int
		}{
			{"empty input", `{"input":"  ","base_version":1}`, http.StatusBadRequest},
			{"missing base", `{"input":"少走路"}`, http.StatusBadRequest},
			{"bad request id", `{"input":"少走路","base_version":1,"client_request_id":"\u0001"}`, http.StatusBadRequest},
			{"unknown version", `{"input":"少走路","base_version":9}`, http.StatusNotFound},
			{"unknown trip", `{"input":"少走路","base_version":1}`, http.StatusNotFound},
		}
		for _, tc := range cases {
			tripID := trip.ID
			if tc.name == "unknown trip" {
				tripID = "missing"
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+tripID+"/adjustments", strings.NewReader(tc.body)))
			if rec.Code != tc.status {
				t.Fatalf("%s: status = %d body=%s", tc.name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("rejects a request id bound to another run", func(t *testing.T) {
		store, trip, _ := versionFixture(t, versionPlan(30))
		requestID := "shared"
		other, _, _, err := store.CreateTripAndRun(context.Background(), repo.CreateTripParams{
			City: "beijing", Days: 1, Status: "planning", RequestID: &requestID,
		}, repo.CreateRunParams{Mode: "initial", Status: "awaiting_user", Stage: "present", RequestID: &requestID})
		if err != nil {
			t.Fatalf("create other trip: %v", err)
		}
		router := tripRouter(store, &fakeRunner{})

		body := `{"input":"少走路","base_version":1,"client_request_id":"shared"}`
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+trip.ID+"/adjustments", strings.NewReader(body)))
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		if len(store.runs[trip.ID]) != 1 {
			t.Fatalf("runs = %+v", store.runs[trip.ID])
		}
		if other.ID == "" {
			t.Fatal("missing other trip")
		}
	})
}
