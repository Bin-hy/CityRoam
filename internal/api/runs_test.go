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
				name: "revise", body: `{"action":"revise","input":" 第二天轻松点 "}`, kind: agent.ResumeRevise, status: agent.StatusQueued,
				check: func(t *testing.T, action agent.ResumeAction) {
					if action.Kind != agent.ResumeRevise || action.Input != "第二天轻松点" {
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
			runner := &fakeRunner{store: store}
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

	t.Run("reports the stored status after a proposal reject", func(t *testing.T) {
		store, run := runFixture(t, agent.StatusAwaitingUser)
		runner := &fakeRunner{
			store:        store,
			resumeStatus: map[agent.ResumeKind]string{agent.ResumeReject: agent.StatusAwaitingUser},
		}
		router := tripRouter(store, runner)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+run.ID+"/resume", strings.NewReader(`{"action":"reject"}`)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		var body runActionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Status != agent.StatusAwaitingUser {
			t.Fatalf("body = %+v", body)
		}
	})

	t.Run("accepts constraints without free text", func(t *testing.T) {
		store, run := runFixture(t, agent.StatusAwaitingUser)
		runner := &fakeRunner{store: store}
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
		runner := &fakeRunner{store: store}
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
			{"empty revise", run.ID, `{"action":"revise","input":"  "}`, http.StatusBadRequest},
			{"overlong revise", run.ID, `{"action":"revise","input":"` + strings.Repeat("改", maxInputRunes+1) + `"}`, http.StatusBadRequest},
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
		runner := &fakeRunner{store: store}
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
			runner := &fakeRunner{store: store}
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

func researchRunFixture(t *testing.T, withQuestion bool) (*fakeTripStore, repo.Run) {
	t.Helper()
	store, run := runFixture(t, agent.StatusAwaitingUser)
	state := agent.TripState{RunID: run.ID, Status: agent.StatusAwaitingUser}
	if withQuestion {
		state.ResearchQuestion = &domain.ResearchQuestion{
			Code:    "hotel_unconfirmed",
			Message: "未找到指定酒店",
			Options: []domain.ResearchOption{
				{ID: "retry_search", Label: "重新搜索"},
				{ID: "continue_without_lodging", Label: "不带住宿继续"},
				{ID: "hotel-1", Label: "测试酒店", Hotel: &domain.Hotel{Name: "测试酒店", Lat: 39.8, Lng: 116.3}},
			},
		}
	}
	encoded, err := state.Encode()
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}
	if err := store.SaveRunState(context.Background(), repo.UpdateRunStateParams{
		ID: run.ID, State: encoded, Stage: "research", Status: agent.StatusAwaitingUser,
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}
	run, err = store.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	return store, run
}

func TestResumeResearch(t *testing.T) {
	t.Run("forwards the client choice to the runner", func(t *testing.T) {
		cases := []struct {
			name string
			body string
		}{
			{"continue without lodging", `{"action":"resolve_research","research_choice":{"option_id":"continue_without_lodging"}}`},
			{"hotel option", `{"action":"resolve_research","research_choice":{"option_id":"hotel-1","hotel":{"name":"测试酒店","lat":39.8,"lng":116.3}}}`},
		}
		for _, tc := range cases {
			store, run := researchRunFixture(t, true)
			runner := &fakeRunner{store: store}
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
			if body.RunID != run.ID || body.Status != agent.StatusQueued {
				t.Fatalf("%s: body = %+v", tc.name, body)
			}
			if len(runner.resumed) != 1 {
				t.Fatalf("%s: resumed = %+v", tc.name, runner.resumed)
			}
			action := runner.resumed[0]
			if action.Kind != agent.ResumeResearch || action.ResearchChoice == nil {
				t.Fatalf("%s: action = %+v", tc.name, action)
			}
			if tc.name == "hotel option" && action.ResearchChoice.Hotel == nil {
				t.Fatalf("hotel choice lost: %+v", action.ResearchChoice)
			}
		}
	})

	t.Run("rejects invalid research resumes", func(t *testing.T) {
		cases := []struct {
			name     string
			status   string
			question bool
			body     string
			code     string
		}{
			{"missing choice", agent.StatusAwaitingUser, true, `{"action":"resolve_research"}`, "invalid_research_choice"},
			{"missing option id", agent.StatusAwaitingUser, true, `{"action":"resolve_research","research_choice":{}}`, "invalid_research_choice"},
			{"not awaiting", agent.StatusRunning, true, `{"action":"resolve_research","research_choice":{"option_id":"retry_search"}}`, "invalid_run_state"},
			{"no question", agent.StatusAwaitingUser, false, `{"action":"resolve_research","research_choice":{"option_id":"retry_search"}}`, "no_research_question"},
		}
		for _, tc := range cases {
			var store *fakeTripStore
			var run repo.Run
			if tc.status == agent.StatusAwaitingUser {
				store, run = researchRunFixture(t, tc.question)
			} else {
				store, run = runFixture(t, tc.status)
			}
			runner := &fakeRunner{store: store}
			router := tripRouter(store, runner)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+run.ID+"/resume", strings.NewReader(tc.body)))
			if rec.Code != http.StatusConflict && rec.Code != http.StatusBadRequest {
				t.Fatalf("%s: status = %d body=%s", tc.name, rec.Code, rec.Body.String())
			}
			body := decodeError(t, rec)
			if body.Code != tc.code || body.RequestID == "" {
				t.Fatalf("%s: error = %+v", tc.name, body)
			}
			if len(runner.resumed) != 0 {
				t.Fatalf("%s: runner called: %+v", tc.name, runner.resumed)
			}
		}
	})

	t.Run("maps research choice errors to 400", func(t *testing.T) {
		store, run := researchRunFixture(t, true)
		runner := &fakeRunner{resumeErr: &agent.ResearchChoiceError{
			Code: "invalid_research_choice", Message: "提交的酒店与选项不一致",
		}}
		router := tripRouter(store, runner)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+run.ID+"/resume", strings.NewReader(
			`{"action":"resolve_research","research_choice":{"option_id":"hotel-1","hotel":{"name":"伪造酒店","lat":31.2,"lng":121.5}}}`,
		)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		body := decodeError(t, rec)
		if body.Code != "invalid_research_choice" || body.RequestID == "" {
			t.Fatalf("error = %+v", body)
		}
	})
}
