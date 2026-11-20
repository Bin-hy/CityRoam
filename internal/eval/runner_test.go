package eval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
	"trippilot/internal/repo"
)

type fakeStore struct {
	accepted bool
	inserted []repo.InsertEvalRunParams
	state    json.RawMessage
}

func (f *fakeStore) CreateTripAndRun(_ context.Context, _ repo.CreateTripParams, _ repo.CreateRunParams) (repo.Trip, repo.Run, bool, error) {
	return repo.Trip{ID: "trip-1"}, repo.Run{ID: "run-1", TripID: "trip-1", Status: agent.StatusQueued}, true, nil
}

func (f *fakeStore) GetRun(_ context.Context, _ string) (repo.Run, error) {
	status := agent.StatusAwaitingUser
	if f.accepted {
		status = agent.StatusCompleted
	}
	return repo.Run{ID: "run-1", TripID: "trip-1", Status: status, State: f.state}, nil
}

func (f *fakeStore) InsertEvalRun(_ context.Context, arg repo.InsertEvalRunParams) (repo.EvalRun, error) {
	f.inserted = append(f.inserted, arg)
	return repo.EvalRun{CaseID: arg.CaseID, Passed: arg.Passed}, nil
}

func (f *fakeStore) ListEvalRuns(_ context.Context) ([]repo.EvalRun, error) {
	return nil, nil
}

type fakeAgent struct{ store *fakeStore }

func (f *fakeAgent) Start(_ context.Context, _ string) error { return nil }

func (f *fakeAgent) Resume(_ context.Context, _ string, action agent.ResumeAction) error {
	if action.Kind != agent.ResumeAccept {
		return nil
	}
	f.store.accepted = true
	return nil
}

func TestRunCaseAcceptsPlanAndRecordsMetrics(t *testing.T) {
	state := agent.TripState{
		Plan: domain.Itinerary{
			City: "beijing",
			Days: []domain.DayPlan{{
				Date: "2026-11-21",
				Blocks: []domain.Block{
					{ID: "b1", Type: domain.BlockTypePOI, POIID: "forbidden-city", Title: "故宫博物院"},
				},
			}},
		},
		Candidates: []domain.POI{{ID: "forbidden-city", Name: "故宫博物院"}},
		LLM:        agent.Usage{Calls: 2, Attempts: 2},
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}
	store := &fakeStore{state: encoded}
	runner := NewRunner(store, &fakeAgent{store: store},
		WithPollInterval(time.Millisecond),
		WithCaseTimeout(time.Second),
	)

	result, err := runner.RunCase(context.Background(), Case{
		CaseID:   "case-1",
		Input:    "北京一天，必须去故宫博物院。",
		Expected: Expected{Days: 1, MustVisit: []string{"故宫博物院"}},
	})
	if err != nil {
		t.Fatalf("RunCase: %v", err)
	}
	if !result.Completed || !result.Passed {
		t.Fatalf("result = %+v", result)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("inserted = %d, want 1", len(store.inserted))
	}
	if !store.inserted[0].Passed || store.inserted[0].RunID == nil || *store.inserted[0].RunID != "run-1" {
		t.Fatalf("inserted = %+v", store.inserted[0])
	}
}

func TestRunCaseAcceptsExpectedResearchQuestion(t *testing.T) {
	state := agent.TripState{
		ResearchQuestion: &domain.ResearchQuestion{Code: "hotel_unconfirmed", Message: "未确认你指定的酒店"},
		Research:         &domain.ResearchBrief{Status: domain.ResearchDegraded, ToolCalls: 2, DurationMS: 400},
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}
	store := &fakeStore{state: encoded}
	runner := NewRunner(store, &fakeAgent{store: store},
		WithPollInterval(time.Millisecond),
		WithCaseTimeout(time.Second),
	)

	result, err := runner.RunCase(context.Background(), Case{
		CaseID:   "case-3",
		Input:    "北京两天，住不存在的酒店",
		Expected: Expected{Days: 2, AwaitingCode: "hotel_unconfirmed"},
	})
	if err != nil {
		t.Fatalf("RunCase: %v", err)
	}
	if !result.Passed || result.Completed {
		t.Fatalf("result = %+v", result)
	}
	if result.Metrics.ResearchStatus != "degraded" || result.Metrics.ToolCalls != 2 {
		t.Fatalf("metrics = %+v", result.Metrics)
	}
	if store.accepted {
		t.Fatal("awaited run must not be accepted")
	}
}

func TestRunCaseRejectsUnexpectedAwaiting(t *testing.T) {
	state := agent.TripState{
		ResearchQuestion: &domain.ResearchQuestion{Code: "lodging_budget"},
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}
	store := &fakeStore{state: encoded}
	runner := NewRunner(store, &fakeAgent{store: store},
		WithPollInterval(time.Millisecond),
		WithCaseTimeout(time.Second),
	)

	result, err := runner.RunCase(context.Background(), Case{
		CaseID:   "case-4",
		Input:    "北京两天",
		Expected: Expected{Days: 2, AwaitingCode: "hotel_unconfirmed"},
	})
	if err != nil {
		t.Fatalf("RunCase: %v", err)
	}
	if result.Passed || !strings.Contains(result.Reason, "hotel_unconfirmed") {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunCaseReportsMissingConstraints(t *testing.T) {
	state := agent.TripState{Missing: []string{"city"}, Status: agent.StatusAwaitingUser}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}
	store := &fakeStore{state: encoded}
	runner := NewRunner(store, &fakeAgent{store: store},
		WithPollInterval(time.Millisecond),
		WithCaseTimeout(time.Second),
	)

	result, err := runner.RunCase(context.Background(), Case{
		CaseID:   "case-2",
		Input:    "帮我安排一下",
		Expected: Expected{Days: 1},
	})
	if err != nil {
		t.Fatalf("RunCase: %v", err)
	}
	if result.Passed {
		t.Fatal("expected failure")
	}
	if result.Reason == "" {
		t.Fatal("expected a reason")
	}
}
