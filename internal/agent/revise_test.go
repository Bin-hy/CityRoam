package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"trippilot/internal/domain"
)

func awaitingPlanState(t *testing.T, withProposal bool) (*TripState, []byte) {
	t.Helper()
	plan := graphPlan()
	draft := domain.DraftFromPlan(plan)
	state := &TripState{
		RunID:       "run-1",
		TripID:      "trip-1",
		Mode:        "initial",
		Input:       "北京一天",
		Status:      StatusAwaitingUser,
		Stage:       nodePresent,
		Constraints: domain.ConstraintSet{City: "beijing", Days: 1, StartDate: "2026-11-20"},
		Plan:        plan,
		Draft:       draft,
		Candidates:  makePOIs("poi", 2),
		Snapshot:    &VersionSnapshot{Seq: 1},
		PreviousSeq: 1,
	}
	if withProposal {
		state.Stage = "proposal"
		state.Snapshot = nil
		state.Plan = graphPlan()
		state.Plan.Days[0].Blocks[0].POIID = "poi-1"
		state.Proposal = &AdjustmentProposal{
			BaseVersion: 1,
			Scope:       "day",
			DayIndex:    0,
			Summary:     "换掉第一天",
		}
		state.Draft = domain.DraftFromPlan(state.Plan)
	}
	raw, err := state.Encode()
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}
	return state, raw
}

func TestRunnerRevise(t *testing.T) {
	t.Run("rejection feedback reruns the same run as an adjustment", func(t *testing.T) {
		_, raw := awaitingPlanState(t, false)
		store := newFakeRunStore(RunRecord{
			ID: "run-1", TripID: "trip-1", Mode: "initial", Status: StatusAwaitingUser, Input: "北京一天", State: raw,
		})
		events := &eventRecorder{}
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"scope":"day","affected_days":[0],"summary":"换掉第一天"}`),
			json.RawMessage(`{"day_index":0,"items":[{"poi_id":"poi-1","type":"poi","duration_min":90}],"reason":"换成另一处"}`),
		}}
		runner := newTestRunner(t, store, events, llm)

		if err := runner.Resume(context.Background(), "run-1", ResumeAction{
			Kind: ResumeRevise, Input: "第一天换个安静的景点",
		}); err != nil {
			t.Fatalf("revise: %v", err)
		}
		runner.Wait()

		if got := store.statusOf("run-1"); got != StatusAwaitingUser {
			t.Fatalf("status = %s", got)
		}
		run, _ := store.GetRun(context.Background(), "run-1")
		if run.Mode != "adjustment" {
			t.Fatalf("run mode = %s", run.Mode)
		}
		state := store.stateOf(t, "run-1")
		if state.Mode != "adjustment" || state.Stage != "proposal" {
			t.Fatalf("state mode/stage = %s/%s", state.Mode, state.Stage)
		}
		if state.Input != "第一天换个安静的景点" {
			t.Fatalf("state input = %q", state.Input)
		}
		if state.PreviousSeq != 1 || state.Previous == nil {
			t.Fatalf("previous = %+v seq=%d", state.Previous, state.PreviousSeq)
		}
		if state.Proposal == nil || state.Snapshot != nil {
			t.Fatalf("proposal = %+v snapshot = %+v", state.Proposal, state.Snapshot)
		}
		types := events.types()
		if countString(types, "run.cancelled") != 0 {
			t.Fatalf("events = %v", types)
		}
		if !containsString(types, "run.started") || !containsString(types, "run.awaiting_user") {
			t.Fatalf("events = %v", types)
		}
	})

	t.Run("blank feedback is rejected without changing status", func(t *testing.T) {
		_, raw := awaitingPlanState(t, false)
		store := newFakeRunStore(RunRecord{
			ID: "run-1", TripID: "trip-1", Status: StatusAwaitingUser, Input: "北京一天", State: raw,
		})
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{})

		err := runner.Resume(context.Background(), "run-1", ResumeAction{Kind: ResumeRevise, Input: "   "})
		if err == nil || !strings.Contains(err.Error(), "empty") {
			t.Fatalf("err = %v", err)
		}
		if got := store.statusOf("run-1"); got != StatusAwaitingUser {
			t.Fatalf("status = %s", got)
		}
	})

	t.Run("overlong feedback is rejected", func(t *testing.T) {
		_, raw := awaitingPlanState(t, false)
		store := newFakeRunStore(RunRecord{
			ID: "run-1", TripID: "trip-1", Status: StatusAwaitingUser, Input: "北京一天", State: raw,
		})
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{})

		err := runner.Resume(context.Background(), "run-1", ResumeAction{
			Kind: ResumeRevise, Input: strings.Repeat("改", maxReviseInput+1),
		})
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("err = %v", err)
		}
		if got := store.statusOf("run-1"); got != StatusAwaitingUser {
			t.Fatalf("status = %s", got)
		}
	})
}

func TestRunnerRejectProposal(t *testing.T) {
	base := graphPlan()
	baseConstraints := domain.ConstraintSet{City: "beijing", Days: 1}
	baseViolations := []domain.Violation{{Code: "budget", Level: domain.LevelWarning, Message: "接近预算"}}

	t.Run("restores the base version and keeps awaiting", func(t *testing.T) {
		_, raw := awaitingPlanState(t, true)
		store := newFakeRunStore(RunRecord{
			ID: "run-1", TripID: "trip-1", Status: StatusAwaitingUser, Input: "北京一天", State: raw,
		})
		store.seedPlanVersion("trip-1", VersionState{
			Seq: 1, Plan: base, Constraints: baseConstraints, Violations: baseViolations,
		})
		events := &eventRecorder{}
		runner := newTestRunner(t, store, events, &fakeLLM{})

		if err := runner.Resume(context.Background(), "run-1", ResumeAction{Kind: ResumeReject}); err != nil {
			t.Fatalf("reject: %v", err)
		}

		if got := store.statusOf("run-1"); got != StatusAwaitingUser {
			t.Fatalf("status = %s", got)
		}
		state := store.stateOf(t, "run-1")
		if state.Stage != proposalRejectedStage {
			t.Fatalf("stage = %s", state.Stage)
		}
		if state.Proposal != nil || state.Snapshot != nil {
			t.Fatalf("proposal = %+v snapshot = %+v", state.Proposal, state.Snapshot)
		}
		if !reflect.DeepEqual(state.Plan, base) {
			t.Fatalf("plan = %+v", state.Plan)
		}
		if !reflect.DeepEqual(state.Constraints, baseConstraints) {
			t.Fatalf("constraints = %+v", state.Constraints)
		}
		if !reflect.DeepEqual(state.Draft, domain.DraftFromPlan(base)) {
			t.Fatalf("draft = %+v", state.Draft)
		}
		if state.PreviousSeq != 1 || state.Previous == nil {
			t.Fatalf("previous = %+v seq=%d", state.Previous, state.PreviousSeq)
		}

		types := events.types()
		if countString(types, "run.awaiting_user") != 1 {
			t.Fatalf("events = %v", types)
		}
		if countString(types, "run.cancelled") != 0 || countString(types, "run.completed") != 0 {
			t.Fatalf("events = %v", types)
		}
	})

	t.Run("missing base version surfaces an error", func(t *testing.T) {
		_, raw := awaitingPlanState(t, true)
		store := newFakeRunStore(RunRecord{
			ID: "run-1", TripID: "trip-1", Status: StatusAwaitingUser, Input: "北京一天", State: raw,
		})
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{})

		err := runner.Resume(context.Background(), "run-1", ResumeAction{Kind: ResumeReject})
		if err == nil || !strings.Contains(err.Error(), "load base version") {
			t.Fatalf("err = %v", err)
		}
		if got := store.statusOf("run-1"); got != StatusAwaitingUser {
			t.Fatalf("status = %s", got)
		}
	})
}
