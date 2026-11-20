package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"trippilot/internal/domain"
)

func patchReply(dayIndex int, items ...domain.DraftItem) json.RawMessage {
	encoded, err := json.Marshal(domain.DayPatch{DayIndex: dayIndex, Items: items, Reason: "减少步行"})
	if err != nil {
		panic(err)
	}
	return encoded
}

func planForDraft(draft domain.DraftPlan) domain.Itinerary {
	plan := domain.Itinerary{City: "beijing"}
	for _, day := range draft.Days {
		dayPlan := domain.DayPlan{Date: day.Date, StartTime: "09:00", EndTime: "18:00"}
		for i, item := range day.Items {
			dayPlan.Blocks = append(dayPlan.Blocks, domain.Block{
				ID: "b", POIID: item.POIID, Title: item.POIID,
				Start: "09:00", End: "10:00", DurationMin: item.DurationMin,
			})
			_ = i
		}
		plan.Days = append(plan.Days, dayPlan)
	}
	return plan
}

func TestRepairNode(t *testing.T) {
	openHours := []domain.OpenRange{{Days: []int{0, 1, 2, 3, 4, 5, 6}, Start: "09:00", End: "17:00"}}
	candidates := append(makePOIs("p", 4),
		domain.POI{ID: "p-4", Name: "国博", Category: "历史", Lat: 39.90, Lng: 116.40, OpenHours: openHours, SuggestedMinutes: 120},
		domain.POI{ID: "p-5", Name: "首博", Category: "历史", Lat: 39.91, Lng: 116.41, OpenHours: openHours, SuggestedMinutes: 120},
		domain.POI{ID: "park-1", Name: "北海公园", Category: "公园", Lat: 39.92, Lng: 116.39, SuggestedMinutes: 90},
	)
	pois := map[string]domain.POI{}
	for _, poi := range candidates {
		pois[poi.ID] = poi
	}

	draft := domain.DraftPlan{Days: []domain.DraftDay{
		{Date: "2026-12-01", Items: []domain.DraftItem{item("p-0", "poi", 120), item("p-1", "poi", 60)}},
		{Date: "2026-12-02", Items: []domain.DraftItem{item("p-2", "poi", 60), item("p-3", "poi", 60)}},
	}}
	constraints := domain.ConstraintSet{City: "beijing", Days: 2, StartDate: "2026-12-01"}
	violations := []domain.Violation{
		{Code: "C1", Level: domain.LevelBlocking, Day: 1, Message: "故宫周一闭馆"},
		{Code: "C9", Level: domain.LevelWarning, Day: 2, Message: "节奏太赶"},
	}

	baseInput := func() RepairInput {
		return RepairInput{
			RunID: "run-1", Constraints: constraints, Draft: draft, Plan: planForDraft(draft),
			POIs: pois, Candidates: candidates, Violations: violations, Round: 1,
		}
	}

	t.Run("patches only the target day", func(t *testing.T) {
		events := &eventRecorder{}
		scheduler := &fakeScheduler{plan: domain.Itinerary{City: "beijing", Days: []domain.DayPlan{{Date: "2026-12-01"}, {Date: "2026-12-02"}}}}
		validator := &fakeValidator{}
		llm := &fakeLLM{reply: patchReply(0, item("p-4", "poi", 120), item("p-5", "poi", 90)),
			usage: Usage{TotalTokens: 60, Attempts: 1}}
		nodes := NewNodes(llm, events,
			WithScheduler(scheduler), WithValidator(validator), WithNodeLogger(discardLogger()),
		)

		out, err := nodes.Repair(context.Background(), baseInput())
		if err != nil {
			t.Fatalf("repair: %v", err)
		}
		if out.Patch.DayIndex != 0 || len(out.Draft.Days[0].Items) != 2 || out.Draft.Days[0].Items[0].POIID != "p-4" {
			t.Fatalf("patch = %+v draft = %+v", out.Patch, out.Draft)
		}
		if !reflect.DeepEqual(out.Draft.Days[1], draft.Days[1]) {
			t.Fatalf("untouched day changed: %+v", out.Draft.Days[1])
		}
		if !reflect.DeepEqual(scheduler.lastDraft, out.Draft) {
			t.Fatalf("scheduler got %+v", scheduler.lastDraft)
		}
		if out.Alternatives == 0 {
			t.Fatal("expected alternatives")
		}
		if len(llm.calls) != 1 {
			t.Fatalf("calls = %d", len(llm.calls))
		}
		prompt := llm.calls[0].prompt
		if !strings.Contains(prompt, `"open_status":"open `) {
			t.Fatalf("prompt missing open status:\n%s", prompt)
		}
		if strings.Contains(prompt, "park-1") {
			t.Fatalf("other category leaked into alternatives:\n%s", prompt)
		}
		if strings.Contains(prompt, `"p-2"`) {
			t.Fatalf("used POI leaked into alternatives:\n%s", prompt)
		}
		if got := events.types(); len(got) != 2 || got[0] != "repair.started" || got[1] != "repair.completed" {
			t.Fatalf("events = %v", got)
		}
	})

	t.Run("wrong day index retries", func(t *testing.T) {
		llm := &fakeLLM{replies: []json.RawMessage{
			patchReply(1, item("p-4", "poi", 120)),
			patchReply(0, item("p-4", "poi", 120)),
		}}
		nodes := NewNodes(llm, &eventRecorder{},
			WithScheduler(&fakeScheduler{plan: domain.Itinerary{City: "beijing", Days: []domain.DayPlan{{}, {}}}}),
			WithValidator(&fakeValidator{}), WithNodeLogger(discardLogger()),
		)
		out, err := nodes.Repair(context.Background(), baseInput())
		if err != nil {
			t.Fatalf("repair: %v", err)
		}
		if out.Patch.DayIndex != 0 || len(llm.calls) != 2 {
			t.Fatalf("patch=%+v calls=%d", out.Patch, len(llm.calls))
		}
		if !strings.Contains(llm.calls[1].prompt, "day_index 必须是 0") {
			t.Fatalf("retry prompt:\n%s", llm.calls[1].prompt)
		}
	})

	t.Run("unknown ids twice fails", func(t *testing.T) {
		llm := &fakeLLM{reply: patchReply(0, item("ghost", "poi", 120))}
		nodes := NewNodes(llm, &eventRecorder{},
			WithScheduler(&fakeScheduler{}), WithValidator(&fakeValidator{}), WithNodeLogger(discardLogger()),
		)
		_, err := nodes.Repair(context.Background(), baseInput())
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "repair_invalid" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("trip-level violation targets first day", func(t *testing.T) {
		input := baseInput()
		input.Violations = []domain.Violation{{Code: "C5", Level: domain.LevelBlocking, Day: 0, Message: "超预算"}}
		llm := &fakeLLM{reply: patchReply(0, item("p-4", "poi", 120))}
		nodes := NewNodes(llm, &eventRecorder{},
			WithScheduler(&fakeScheduler{plan: domain.Itinerary{City: "beijing", Days: []domain.DayPlan{{}, {}}}}),
			WithValidator(&fakeValidator{}), WithNodeLogger(discardLogger()),
		)
		out, err := nodes.Repair(context.Background(), input)
		if err != nil {
			t.Fatalf("repair: %v", err)
		}
		if out.Patch.DayIndex != 0 {
			t.Fatalf("patch = %+v", out.Patch)
		}
	})

	t.Run("scheduler failure surfaces", func(t *testing.T) {
		llm := &fakeLLM{reply: patchReply(0, item("p-4", "poi", 120))}
		nodes := NewNodes(llm, &eventRecorder{},
			WithScheduler(&fakeScheduler{err: errors.New("bad route")}),
			WithValidator(&fakeValidator{}), WithNodeLogger(discardLogger()),
		)
		_, err := nodes.Repair(context.Background(), baseInput())
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "schedule_failed" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("nothing to repair", func(t *testing.T) {
		input := baseInput()
		input.Violations = nil
		llm := &fakeLLM{}
		nodes := NewNodes(llm, &eventRecorder{}, WithNodeLogger(discardLogger()))
		_, err := nodes.Repair(context.Background(), input)
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "nothing_to_repair" {
			t.Fatalf("err = %v", err)
		}
		if len(llm.calls) != 0 {
			t.Fatalf("calls = %d", len(llm.calls))
		}
	})

	t.Run("event failure aborts", func(t *testing.T) {
		llm := &fakeLLM{reply: patchReply(0, item("p-4", "poi", 120))}
		nodes := NewNodes(llm, &eventRecorder{err: errors.New("db down")},
			WithScheduler(&fakeScheduler{}), WithValidator(&fakeValidator{}), WithNodeLogger(discardLogger()),
		)
		_, err := nodes.Repair(context.Background(), baseInput())
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "event_failed" {
			t.Fatalf("err = %v", err)
		}
	})
}
