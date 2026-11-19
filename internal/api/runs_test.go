package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"trippilot/internal/agent"
	"trippilot/internal/repo"
)

func runFixture(t *testing.T, status string) (*fakeTripStore, repo.Run) {
	t.Helper()
	store := newFakeTripStore()
	_, run, _, err := store.CreateTripAndRun(context.Background(), repo.CreateTripParams{
		City: "beijing", Days: 1, Status: "planning",
	}, repo.CreateRunParams{Mode: "initial", Status: status, Stage: "present"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return store, run
}

func TestResume(t *testing.T) {
	t.Run("resumes with each supported action", func(t *testing.T) {
		cases := []struct {
			name   string
			body   string
			kind   agent.ResumeKind
			status string
			check  func(*testing.T, agent.ResumeAction)
		}{
			{
				name: "accept", body: `{"action":"accept"}`, kind: agent.ResumeAccept, status: agent.StatusCompleted,
				check: func(t *testing.T, action agent.ResumeAction) {
					if action.Kind != agent.ResumeAccept {
						t.Fatalf("action = %+v", action)
					}
				},
			},
			{
				name: "reject", body: `{"action":"reject"}`, kind: agent.ResumeReject, status: agent.StatusCancelled,
				check: func(t *testing.T, action agent.ResumeAction) {
					if action.Kind != agent.ResumeReject {
						t.Fatalf("action = %+v", action)
					}
				},
			},
			{
				name: "provide constraints", body: `{"action":"provide_constraints","input":"下周出发","constraints":{"start_date":"2026-11-23"}}`,
				kind: agent.ResumeSupplement, status: agent.StatusQueued,
				check: func(t *testing.T, action agent.ResumeAction) {
					if action.Kind != agent.ResumeSupplement {
						t.Fatalf("action = %+v", action)
					}
					if !strings.Contains(action.Input, "下周出发") || !strings.Contains(action.Input, `"start_date":"2026-11-23"`) {
						t.Fatalf("input = %q", action.Input)
					}
				},
			},
		}
		for _, tc := range cases {
			store, run := runFixture(t, agent.StatusAwaitingUser)
			runner := &fakeRunner{}
			router := tripRouter(store, runner)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+run.ID+"/resume", strings.NewReader(tc.body)))
			if rec.Code != http.StatusAccepted {
				t.Fatalf("%s: status = %d body=%s", tc.name, rec.Code, rec.Body.String())
			}
			var body runActionResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("%s: decode: %v", tc.name, err)
			}
			if body.RunID != run.ID || body.Status != tc.status {
				t.Fatalf("%s: body = %+v", tc.name, body)
			}
			if len(runner.resumed) != 1 || runner.resumed[0].Kind != tc.kind {
				t.Fatalf("%s: resumed = %+v", tc.name, runner.resumed)
			}
			tc.check(t, runner.resumed[0])
		}
	})

	t.Run("accepts constraints without free text", func(t *testing.T) {
		store, run := runFixture(t, agent.StatusAwaitingUser)
		runner := &fakeRunner{}
		router := tripRouter(store, runner)

		rec := httptest.NewRecorder()
		body := `{"action":"provide_constraints","constraints":{"start_date":"2026-11-23"}}`
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+run.ID+"/resume", strings.NewReader(body)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		if len(runner.resumed) != 1 || !strings.Contains(runner.resumed[0].Input, "start_date") {
			t.Fatalf("resumed = %+v", runner.resumed)
		}
	})

	t.Run("rejects invalid bodies and missing runs", func(t *testing.T) {
		store, run := runFixture(t, agent.StatusAwaitingUser)
		runner := &fakeRunner{}
		router := tripRouter(store, runner)

		cases := []struct {
			name   string
			runID  string
			body   string
			status int
		}{
			{"unknown action", run.ID, `{"action":"retry"}`, http.StatusBadRequest},
			{"empty supplement", run.ID, `{"action":"provide_constraints"}`, http.StatusBadRequest},
			{"bad constraints", run.ID, `{"action":"provide_constraints","constraints":[]}`, http.StatusBadRequest},
			{"bad json", run.ID, `{"action":`, http.StatusBadRequest},
			{"missing run", "nope", `{"action":"accept"}`, http.StatusNotFound},
		}
		for _, tc := range cases {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+tc.runID+"/resume", strings.NewReader(tc.body)))
			if rec.Code != tc.status {
				t.Fatalf("%s: status = %d body=%s", tc.name, rec.Code, rec.Body.String())
			}
		}
		if len(runner.resumed) != 0 {
			t.Fatalf("resumed = %+v", runner.resumed)
		}
	})

	t.Run("maps invalid state to 409", func(t *testing.T) {
		store, run := runFixture(t, agent.StatusCompleted)
		runner := &fakeRunner{resumeErr: agent.ErrInvalidRunStatus}
		router := tripRouter(store, runner)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+run.ID+"/resume", strings.NewReader(`{"action":"accept"}`)))
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		if body := decodeError(t, rec); body.Code != "invalid_run_state" {
			t.Fatalf("error = %+v", body)
		}
	})
}

func TestCancel(t *testing.T) {
	t.Run("cancels an active run", func(t *testing.T) {
		store, run := runFixture(t, agent.StatusRunning)
		runner := &fakeRunner{}
		router := tripRouter(store, runner)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+run.ID+"/cancel", nil))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		var body runActionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.RunID != run.ID || body.Status != agent.StatusCancelled {
			t.Fatalf("body = %+v", body)
		}
		if len(runner.cancelled) != 1 || runner.cancelled[0] != run.ID {
			t.Fatalf("cancelled = %v", runner.cancelled)
		}
	})

	t.Run("terminal runs stay immutable", func(t *testing.T) {
		for _, status := range []string{agent.StatusCompleted, agent.StatusFailed, agent.StatusCancelled} {
			store, run := runFixture(t, status)
			runner := &fakeRunner{}
			router := tripRouter(store, runner)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+run.ID+"/cancel", nil))
			if rec.Code != http.StatusAccepted {
				t.Fatalf("%s: status = %d body=%s", status, rec.Code, rec.Body.String())
			}
			var body runActionResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("%s: decode: %v", status, err)
			}
			if body.Status != status {
				t.Fatalf("%s: body = %+v", status, body)
			}
			if got := store.runs[run.TripID][0].Status; got != status {
				t.Fatalf("%s: stored status = %s", status, got)
			}
		}
	})

	t.Run("missing run is 404", func(t *testing.T) {
		router := tripRouter(newFakeTripStore(), &fakeRunner{})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/nope/cancel", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}
