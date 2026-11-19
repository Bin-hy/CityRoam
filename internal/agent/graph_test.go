package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"trippilot/internal/domain"
)

func graphPlan() domain.Itinerary {
	return domain.Itinerary{
		City: "beijing",
		Days: []domain.DayPlan{{
			Date:      "2026-11-20",
			StartTime: "09:00",
			EndTime:   "11:30",
			Blocks: []domain.Block{
				{ID: "b1", Type: domain.BlockTypePOI, POIID: "poi-0", Title: "poi-0景点0", Start: "09:00", End: "10:30", DurationMin: 90, CostYuan: 30},
			},
			DayCostYuan: 30,
		}},
		Totals: domain.Totals{CostYuan: 30, POICount: 1, WalkingKM: 1.5},
	}
}

func graphDraftReply() json.RawMessage {
	return json.RawMessage(`{"days":[{"date":"2026-11-20","items":[{"poi_id":"poi-0","type":"poi","duration_min":90}]}],"explanations":[{"poi_id":"poi-0","text":"经典打卡点"}]}`)
}

func graphRepairReply(poiID string) json.RawMessage {
	payload, _ := json.Marshal(map[string]any{
		"day_index": 0,
		"items":     []map[string]any{{"poi_id": poiID, "type": "poi", "duration_min": 90}},
		"reason":    "换成同类地点",
	})
	return payload
}

func graphNodes(llm *fakeLLM, events *eventRecorder, validator Validator) *Nodes {
	source := &fakeSource{fn: func(context.Context, domain.POIQuery) ([]domain.POI, error) {
		return makePOIs("poi", 2), nil
	}}
	return NewNodes(llm, events,
		WithSnapshotSource(source),
		WithScheduler(&fakeScheduler{plan: graphPlan()}),
		WithValidator(validator),
		WithNodeClock(func() time.Time { return time.Date(2026, 11, 19, 10, 0, 0, 0, time.Local) }),
		WithNodeLogger(discardLogger()),
	)
}

func TestGraph(t *testing.T) {
	t.Run("runs parse to present", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
		}}
		events := &eventRecorder{}
		nodes := graphNodes(llm, events, &fakeValidator{})
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}

		out, err := graph.Run(context.Background(), &TripState{RunID: "run-1", Input: "北京一天"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Status != StatusAwaitingUser {
			t.Fatalf("status = %s", out.Status)
		}
		if out.Constraints.City != "beijing" || out.Constraints.Days != 1 {
			t.Fatalf("constraints = %+v", out.Constraints)
		}
		if len(out.Candidates) != 2 || out.Plan.Totals.CostYuan != 30 {
			t.Fatalf("state = %+v", out)
		}
		if out.Snapshot == nil || out.Snapshot.Seq != 1 || out.Snapshot.Reason != "initial" {
			t.Fatalf("snapshot = %+v", out.Snapshot)
		}
		if len(out.Snapshot.Explanations) != 1 || out.Snapshot.Explanations[0].Text != "经典打卡点" {
			t.Fatalf("explanations = %+v", out.Snapshot.Explanations)
		}
		if len(out.History) != 1 || out.History[0].Blocking != 0 {
			t.Fatalf("history = %+v", out.History)
		}

		types := events.types()
		for _, want := range []string{"constraint.parsed", "candidate.retrieved", "plan.drafted", "validation.result", "plan.updated"} {
			if !containsString(types, want) {
				t.Fatalf("missing event %s in %v", want, types)
			}
		}
		if containsString(types, "repair.started") {
			t.Fatalf("unexpected repair: %v", types)
		}
		if len(llm.calls) != 2 {
			t.Fatalf("llm calls = %d", len(llm.calls))
		}
	})

	t.Run("stops after parse when critical fields are missing", func(t *testing.T) {
		llm := &fakeLLM{reply: json.RawMessage(`{"city":"","days":0}`)}
		events := &eventRecorder{}
		nodes := graphNodes(llm, events, &fakeValidator{})
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}

		out, err := graph.Run(context.Background(), &TripState{RunID: "run-2", Input: "去玩"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.Status != StatusAwaitingUser || len(out.Missing) != 2 {
			t.Fatalf("state = %+v", out)
		}
		if out.Snapshot != nil || len(out.Candidates) != 0 {
			t.Fatalf("graph continued past parse: %+v", out)
		}
		if got := events.types(); len(got) != 1 || got[0] != "constraint.needs_input" {
			t.Fatalf("events = %v", got)
		}
	})

	t.Run("repairs then stops when violations repeat", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
			graphRepairReply("poi-1"),
			graphRepairReply("poi-1"),
		}}
		events := &eventRecorder{}
		blocking := []domain.Violation{{Code: "C4", Level: domain.LevelBlocking, Day: 1, Message: "超出每日时长"}}
		nodes := graphNodes(llm, events, &fakeValidator{violations: blocking})
		graph, err := NewGraph(context.Background(), nodes)
		if err != nil {
			t.Fatalf("new graph: %v", err)
		}

		out, err := graph.Run(context.Background(), &TripState{RunID: "run-3", Input: "北京一天"})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out.StopReason != string(StopNoImprovement) {
			t.Fatalf("stop reason = %s", out.StopReason)
		}
		if len(out.History) != 3 {
			t.Fatalf("history = %+v", out.History)
		}
		if out.Snapshot == nil || len(out.Snapshot.Violations) != 1 {
			t.Fatalf("snapshot = %+v", out.Snapshot)
		}
		if len(out.Suggestions) == 0 {
			t.Fatal("missing relaxation suggestions")
		}
		if out.Snapshot.Reason != "repair" {
			t.Fatalf("snapshot reason = %s", out.Snapshot.Reason)
		}
		if len(llm.calls) != 4 {
			t.Fatalf("llm calls = %d", len(llm.calls))
		}
		types := events.types()
		if countString(types, "repair.started") != 2 || countString(types, "repair.completed") != 2 {
			t.Fatalf("events = %v", types)
		}
	})
}

func TestGraphRequiresState(t *testing.T) {
	graph, err := NewGraph(context.Background(), NewNodes(&fakeLLM{}, &eventRecorder{}, WithNodeLogger(discardLogger())))
	if err != nil {
		t.Fatalf("new graph: %v", err)
	}
	if _, err := graph.Run(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil state")
	}
	if _, err := NewGraph(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil nodes")
	}
}

func TestTripStateRoundtrip(t *testing.T) {
	snapshot := VersionSnapshot{
		Seq:          2,
		Plan:         graphPlan(),
		Constraints:  domain.ConstraintSet{City: "beijing", Days: 1},
		Violations:   []domain.Violation{{Code: "C9", Level: domain.LevelWarning, Day: 1, Message: "偏满"}},
		Reason:       "repair",
		Explanations: []domain.Explanation{{POIID: "poi-0", Text: "经典"}},
		Suggestions:  []string{"减少景点"},
		CreatedAt:    time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC),
	}
	best := graphPlan()
	state := &TripState{
		TripID:         "trip-1",
		RunID:          "run-1",
		Mode:           "adjustment",
		Input:          "北京一天，少走路",
		Constraints:    domain.ConstraintSet{City: "beijing", Days: 1},
		Missing:        []string{"days"},
		Candidates:     makePOIs("poi", 2),
		Draft:          domain.DraftPlan{Days: []domain.DraftDay{{Date: "2026-11-20", Items: []domain.DraftItem{{POIID: "poi-0", Type: "poi", DurationMin: 90}}}}},
		Plan:           graphPlan(),
		Violations:     []domain.Violation{{Code: "C4", Level: domain.LevelBlocking, Day: 1, Message: "超时"}},
		Round:          1,
		Best:           &best,
		BestViolations: []domain.Violation{{Code: "C9", Level: domain.LevelWarning, Day: 1, Message: "偏满"}},
		History:        []RoundMetrics{{Round: 0, Blocking: 1, Warnings: 0, CostYuan: 30, WalkingKM: 1.5}},
		StopReason:     "max_rounds",
		Suggestions:    []string{"减少景点"},
		Snapshot:       &snapshot,
		Previous:       &best,
		PreviousSeq:    1,
		Status:         StatusAwaitingUser,
		Error:          &RunError{Code: "event_failed", Message: "db down"},
	}

	encoded, err := state.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeTripState(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(state, decoded) {
		t.Fatalf("roundtrip mismatch:\nwant %+v\ngot  %+v", state, decoded)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func countString(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}
