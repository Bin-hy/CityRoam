package agent

import (
	"context"
	"errors"
	"testing"

	"trippilot/internal/domain"
)

type fakeScheduler struct {
	plan      domain.Itinerary
	err       error
	lastDraft domain.DraftPlan
}

func (f *fakeScheduler) Build(draft domain.DraftPlan, constraints domain.ConstraintSet, pois map[string]domain.POI, routes domain.RouteProvider) (domain.Itinerary, error) {
	f.lastDraft = draft
	if f.err != nil {
		return domain.Itinerary{}, f.err
	}
	return f.plan, nil
}

type fakeValidator struct {
	violations []domain.Violation
}

func (f *fakeValidator) Validate(plan domain.Itinerary, constraints domain.ConstraintSet, pois map[string]domain.POI, routes domain.RouteProvider) []domain.Violation {
	return f.violations
}

func TestScheduleNode(t *testing.T) {
	constraints := domain.ConstraintSet{City: "beijing", Days: 1, StartDate: "2026-12-01"}
	plan := domain.Itinerary{
		City: "beijing",
		Days: []domain.DayPlan{{Date: "2026-12-01", StartTime: "09:00", EndTime: "11:00"}},
	}

	t.Run("publishes plan.drafted", func(t *testing.T) {
		events := &eventRecorder{}
		nodes := NewNodes(nil, events,
			WithScheduler(&fakeScheduler{plan: plan}),
			WithNodeLogger(discardLogger()),
		)
		out, err := nodes.Schedule(context.Background(), ScheduleInput{
			RunID: "run-1", Constraints: constraints,
			Draft: domain.DraftPlan{Days: []domain.DraftDay{{Items: []domain.DraftItem{{POIID: "p", Type: "poi", DurationMin: 60}}}}},
			POIs:  map[string]domain.POI{"p": {ID: "p"}},
		})
		if err != nil {
			t.Fatalf("schedule: %v", err)
		}
		if len(out.Days) != 1 || out.Days[0].StartTime != "09:00" {
			t.Fatalf("plan = %+v", out)
		}
		if got := events.types(); len(got) != 1 || got[0] != "plan.drafted" {
			t.Fatalf("events = %v", got)
		}
	})

	t.Run("scheduler failure surfaces", func(t *testing.T) {
		nodes := NewNodes(nil, &eventRecorder{},
			WithScheduler(&fakeScheduler{err: errors.New("unknown poi")}),
			WithNodeLogger(discardLogger()),
		)
		_, err := nodes.Schedule(context.Background(), ScheduleInput{Constraints: constraints})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "schedule_failed" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("default scheduler runs", func(t *testing.T) {
		pois := map[string]domain.POI{
			"p": {ID: "p", City: "beijing", Name: "故宫", Lat: 39.9163, Lng: 116.3972, SuggestedMinutes: 90},
		}
		events := &eventRecorder{}
		nodes := NewNodes(nil, events, WithNodeLogger(discardLogger()))
		draft := domain.DraftPlan{Days: []domain.DraftDay{{Date: "2026-12-01", Items: []domain.DraftItem{{POIID: "p", Type: "poi", DurationMin: 90}}}}}

		plan, err := nodes.Schedule(context.Background(), ScheduleInput{
			RunID: "run-2", Constraints: constraints, Draft: draft, POIs: pois,
		})
		if err != nil {
			t.Fatalf("schedule: %v", err)
		}
		if len(plan.Days) != 1 || len(plan.Days[0].Blocks) == 0 {
			t.Fatalf("plan = %+v", plan)
		}
		if plan.City != "beijing" || plan.Totals.POICount != 1 {
			t.Fatalf("plan = %+v", plan)
		}
	})
}

func TestValidateNode(t *testing.T) {
	constraints := domain.ConstraintSet{City: "beijing", Days: 1}
	plan := domain.Itinerary{City: "beijing", Days: []domain.DayPlan{{Date: "2026-12-01"}}}

	t.Run("publishes counts", func(t *testing.T) {
		events := &eventRecorder{}
		nodes := NewNodes(nil, events,
			WithValidator(&fakeValidator{violations: []domain.Violation{
				{Code: "C1", Level: domain.LevelBlocking, Day: 1, Message: "闭馆"},
				{Code: "C9", Level: domain.LevelWarning, Day: 1, Message: "太赶"},
			}}),
			WithNodeLogger(discardLogger()),
		)
		out, err := nodes.Validate(context.Background(), ValidateInput{
			RunID: "run-1", Constraints: constraints, Plan: plan, POIs: map[string]domain.POI{},
		})
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if out.Blocking != 1 || out.Warnings != 1 || len(out.Violations) != 2 {
			t.Fatalf("output = %+v", out)
		}
		if got := events.types(); len(got) != 1 || got[0] != "validation.result" {
			t.Fatalf("events = %v", got)
		}
		payload := events.events[0].payload.(map[string]any)
		if payload["blocking"] != 1 || payload["warnings"] != 1 {
			t.Fatalf("payload = %v", payload)
		}
	})

	t.Run("default validator runs", func(t *testing.T) {
		pois := map[string]domain.POI{
			"p": {ID: "p", Name: "故宫", Lat: 39.9, Lng: 116.4, SuggestedMinutes: 90},
		}
		scheduled := domain.Itinerary{City: "beijing", Days: []domain.DayPlan{{
			Date: "2026-12-01",
			Blocks: []domain.Block{{
				ID: "b1", Type: domain.BlockTypePOI, POIID: "p", Title: "故宫",
				Start: "09:00", End: "10:30", DurationMin: 90,
			}},
		}}}
		events := &eventRecorder{}
		nodes := NewNodes(nil, events, WithNodeLogger(discardLogger()))
		out, err := nodes.Validate(context.Background(), ValidateInput{
			RunID: "run-2", Constraints: constraints, Plan: scheduled, POIs: pois,
		})
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if out.Blocking != 0 {
			t.Fatalf("unexpected blocking: %+v", out.Violations)
		}
	})

	t.Run("event failure surfaces", func(t *testing.T) {
		nodes := NewNodes(nil, &eventRecorder{err: errors.New("db down")},
			WithValidator(&fakeValidator{}),
			WithNodeLogger(discardLogger()),
		)
		_, err := nodes.Validate(context.Background(), ValidateInput{Constraints: constraints, Plan: plan})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "event_failed" {
			t.Fatalf("err = %v", err)
		}
	})
}
