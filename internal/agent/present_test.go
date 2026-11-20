package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"trippilot/internal/domain"
	"trippilot/internal/domain/diff"
)

func presentPlan() domain.Itinerary {
	return domain.Itinerary{
		City: "beijing",
		Days: []domain.DayPlan{
			{
				Date:      "2026-11-20",
				StartTime: "09:00",
				EndTime:   "16:00",
				Blocks: []domain.Block{
					{ID: "b1", Type: domain.BlockTypePOI, POIID: "poi-1", Title: "故宫", Start: "09:00", End: "11:30", DurationMin: 150, CostYuan: 60},
					{ID: "b2", Type: domain.BlockTypeMeal, Title: "午餐", Start: "12:00", End: "13:00", DurationMin: 60, CostYuan: 50},
					{ID: "b3", Type: domain.BlockTypePOI, POIID: "poi-2", Title: "天坛", Start: "14:00", End: "15:30", DurationMin: 90, CostYuan: 34},
				},
				DayCostYuan: 144,
			},
		},
		Totals: domain.Totals{CostYuan: 144, POICount: 2, WalkingKM: 4.2},
	}
}

func newPresentNodes(events *eventRecorder, fixed time.Time) *Nodes {
	return NewNodes(&fakeLLM{}, events,
		WithNodeClock(func() time.Time { return fixed }),
		WithNodeLogger(discardLogger()),
	)
}

func TestPresentResearch(t *testing.T) {
	fixed := time.Date(2026, 11, 19, 10, 0, 0, 0, time.Local)
	research := func() *domain.ResearchBrief {
		return &domain.ResearchBrief{
			Status: domain.ResearchOK,
			Accommodation: &domain.AccommodationBrief{
				Mentioned:       true,
				Selected:        &domain.Hotel{ID: "h1", Name: "测试酒店", PricePerNightYuan: 500, Provider: "flyai"},
				AnchorDays:      []int{0},
				Nights:          1,
				TotalYuan:       500,
				IncludeInBudget: true,
			},
			Guides: []domain.GuideSignal{{
				POIID:    "poi-1",
				Weight:   0.8,
				Mentions: 2,
				Note:     "攻略推荐",
				Citations: []domain.GuideCitation{{
					Title: "故宫攻略", URL: "https://example.com/a", Authority: 3,
				}},
			}},
			Notes:     []string{"已检索"},
			Warnings:  []string{},
			ToolCalls: 4,
		}
	}

	t.Run("snapshot deep copies research", func(t *testing.T) {
		input := research()
		out, err := newPresentNodes(&eventRecorder{}, fixed).Present(context.Background(), PresentInput{
			RunID: "run-r1", Plan: presentPlan(), Research: input,
		})
		if err != nil {
			t.Fatalf("present: %v", err)
		}
		if out.Snapshot.Research == nil || out.Snapshot.Research.Status != domain.ResearchOK {
			t.Fatalf("snapshot research = %+v", out.Snapshot.Research)
		}

		input.Accommodation.Selected.Name = "被改写"
		input.Accommodation.AnchorDays[0] = 1
		input.Guides[0].POIID = "mutated"
		input.Guides[0].Citations[0].Title = "被改写"
		input.Notes[0] = "被改写"

		snapshot := out.Snapshot.Research
		if snapshot.Accommodation.Selected.Name != "测试酒店" || snapshot.Accommodation.AnchorDays[0] != 0 {
			t.Fatalf("accommodation shared state: %+v", snapshot.Accommodation)
		}
		if snapshot.Guides[0].POIID != "poi-1" || snapshot.Guides[0].Citations[0].Title != "故宫攻略" {
			t.Fatalf("guides shared state: %+v", snapshot.Guides)
		}
		if snapshot.Notes[0] != "已检索" {
			t.Fatalf("notes shared state: %+v", snapshot.Notes)
		}
	})

	t.Run("nil research stays nil and compatible", func(t *testing.T) {
		out, err := newPresentNodes(&eventRecorder{}, fixed).Present(context.Background(), PresentInput{
			RunID: "run-r2", Plan: presentPlan(),
		})
		if err != nil {
			t.Fatalf("present: %v", err)
		}
		if out.Snapshot.Research != nil {
			t.Fatalf("research = %+v, want nil", out.Snapshot.Research)
		}
		encoded, err := json.Marshal(out.Snapshot)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(encoded), `"research"`) {
			t.Fatalf("snapshot JSON contains research: %s", encoded)
		}
	})

	t.Run("plan.updated only carries summaries", func(t *testing.T) {
		events := &eventRecorder{}
		plan := presentPlan()
		plan.Lodging = &domain.LodgingSummary{
			Hotel: domain.Hotel{Name: "测试酒店"}, Nights: 1, TotalYuan: 500,
			IncludeInBudget: true, PriceEstimated: true,
		}
		plan.Totals.LodgingYuan = 500
		_, err := newPresentNodes(events, fixed).Present(context.Background(), PresentInput{
			RunID: "run-r3", Plan: plan, Research: research(),
		})
		if err != nil {
			t.Fatalf("present: %v", err)
		}
		payload := events.events[0].payload.(map[string]any)
		if payload["research_status"] != domain.ResearchOK {
			t.Fatalf("research_status = %v", payload["research_status"])
		}
		lodging, ok := payload["lodging_summary"].(*lodgingEventSummary)
		if !ok {
			t.Fatalf("lodging_summary = %T", payload["lodging_summary"])
		}
		if lodging.HotelName != "测试酒店" || lodging.TotalYuan != 500 || !lodging.PriceEstimated {
			t.Fatalf("lodging summary = %+v", lodging)
		}
		guides, ok := payload["guide_summary"].(*guideEventSummary)
		if !ok {
			t.Fatalf("guide_summary = %T", payload["guide_summary"])
		}
		if guides.Matches != 1 || len(guides.POIIDs) != 1 || guides.POIIDs[0] != "poi-1" {
			t.Fatalf("guide summary = %+v", guides)
		}
		for _, banned := range []string{"research", "tool_calls", "guides", "accommodation"} {
			if _, exists := payload[banned]; exists {
				t.Fatalf("payload leaked %q: %+v", banned, payload)
			}
		}
	})

	t.Run("adjustment version copies base research", func(t *testing.T) {
		input := research()
		previous := presentPlan()
		out, err := newPresentNodes(&eventRecorder{}, fixed).Present(context.Background(), PresentInput{
			RunID: "run-r4", Plan: presentPlan(), Previous: &previous,
			PreviousSeq: 2, Reason: "adjustment", Research: input,
		})
		if err != nil {
			t.Fatalf("present: %v", err)
		}
		if out.Snapshot.Seq != 3 || out.Snapshot.Reason != "adjustment" {
			t.Fatalf("snapshot = %+v", out.Snapshot)
		}
		if out.Snapshot.Research == nil || out.Snapshot.Research.Accommodation.Selected.ID != "h1" {
			t.Fatalf("adjustment research = %+v", out.Snapshot.Research)
		}
		if out.Snapshot.Research == input {
			t.Fatal("adjustment research shares the input pointer")
		}
	})
}

func TestPresentNode(t *testing.T) {
	fixed := time.Date(2026, 11, 19, 10, 0, 0, 0, time.Local)
	constraints := domain.ConstraintSet{City: "beijing", Days: 1}
	violations := []domain.Violation{{Code: "C9", Level: domain.LevelWarning, Day: 1, Message: "日程偏满"}}

	t.Run("initial version snapshots plan and fills explanations", func(t *testing.T) {
		events := &eventRecorder{}
		nodes := newPresentNodes(events, fixed)

		out, err := nodes.Present(context.Background(), PresentInput{
			RunID:        "run-1",
			Constraints:  constraints,
			Plan:         presentPlan(),
			Violations:   violations,
			Explanations: []domain.Explanation{{POIID: "poi-1", Text: "故宫需要提前预约。"}, {POIID: "ghost", Text: "已移除"}},
		})
		if err != nil {
			t.Fatalf("present: %v", err)
		}
		if out.Status != StatusAwaitingUser {
			t.Fatalf("status = %s", out.Status)
		}
		snapshot := out.Snapshot
		if snapshot.Seq != 1 || snapshot.Reason != "initial" {
			t.Fatalf("snapshot = %+v", snapshot)
		}
		if !snapshot.CreatedAt.Equal(fixed.UTC()) {
			t.Fatalf("created at = %s", snapshot.CreatedAt)
		}
		if len(snapshot.Diff.Days) != 1 || snapshot.Diff.Days[0].Change != diff.Added {
			t.Fatalf("diff days = %+v", snapshot.Diff.Days)
		}
		if len(snapshot.Diff.Blocks) != 0 {
			t.Fatalf("diff blocks = %+v", snapshot.Diff.Blocks)
		}
		if len(snapshot.Explanations) != 2 {
			t.Fatalf("explanations = %+v", snapshot.Explanations)
		}
		if snapshot.Explanations[0].POIID != "poi-1" || snapshot.Explanations[0].Text != "故宫需要提前预约。" {
			t.Fatalf("kept explanation = %+v", snapshot.Explanations[0])
		}
		fallback := snapshot.Explanations[1]
		if fallback.POIID != "poi-2" || !strings.Contains(fallback.Text, "第 1 天") || !strings.Contains(fallback.Text, "天坛") {
			t.Fatalf("fallback = %+v", fallback)
		}
		if !strings.Contains(fallback.Text, "90 分钟") || !strings.Contains(fallback.Text, "34 元") {
			t.Fatalf("fallback detail = %s", fallback.Text)
		}
		if len(snapshot.Violations) != 1 {
			t.Fatalf("violations = %+v", snapshot.Violations)
		}

		if got := events.types(); len(got) != 1 || got[0] != "plan.updated" {
			t.Fatalf("events = %v", got)
		}
		payload, ok := events.events[0].payload.(map[string]any)
		if !ok {
			t.Fatalf("payload = %T", events.events[0].payload)
		}
		if payload["seq"] != 1 || payload["reason"] != "initial" {
			t.Fatalf("payload = %+v", payload)
		}
		if _, ok := payload["plan"].(domain.Itinerary); !ok {
			t.Fatalf("payload plan = %T", payload["plan"])
		}
	})

	t.Run("diffs against previous version and increments seq", func(t *testing.T) {
		previous := presentPlan()
		previous.Days[0].Blocks[0].Title = "故宫（上午）"
		current := presentPlan()
		current.Days[0].Blocks[0].Notes = "已预约"

		events := &eventRecorder{}
		nodes := newPresentNodes(events, fixed)
		out, err := nodes.Present(context.Background(), PresentInput{
			RunID:       "run-2",
			Constraints: constraints,
			Plan:        current,
			Previous:    &previous,
			PreviousSeq: 3,
			Reason:      "按反馈调整了故宫时段",
			Suggestions: []string{"可以减少下午景点"},
		})
		if err != nil {
			t.Fatalf("present: %v", err)
		}
		if out.Snapshot.Seq != 4 || out.Snapshot.Reason != "按反馈调整了故宫时段" {
			t.Fatalf("snapshot = %+v", out.Snapshot)
		}
		if len(out.Snapshot.Diff.Days) != 1 || out.Snapshot.Diff.Days[0].Change != diff.Modified {
			t.Fatalf("diff days = %+v", out.Snapshot.Diff.Days)
		}
		if len(out.Snapshot.Diff.Blocks) != 1 || out.Snapshot.Diff.Blocks[0].BlockID != "b1" {
			t.Fatalf("diff blocks = %+v", out.Snapshot.Diff.Blocks)
		}
		if len(out.Snapshot.Suggestions) != 1 || out.Snapshot.Suggestions[0] != "可以减少下午景点" {
			t.Fatalf("suggestions = %+v", out.Snapshot.Suggestions)
		}
	})

	t.Run("defaults reason to updated when a previous version exists", func(t *testing.T) {
		previous := domain.Itinerary{City: "beijing", Days: []domain.DayPlan{{Date: "2026-11-20"}}}
		nodes := newPresentNodes(&eventRecorder{}, fixed)
		out, err := nodes.Present(context.Background(), PresentInput{
			RunID:    "run-3",
			Plan:     presentPlan(),
			Previous: &previous,
		})
		if err != nil {
			t.Fatalf("present: %v", err)
		}
		if out.Snapshot.Reason != "updated" {
			t.Fatalf("reason = %s", out.Snapshot.Reason)
		}
	})

	t.Run("event failure fails the node", func(t *testing.T) {
		nodes := newPresentNodes(&eventRecorder{err: errors.New("db down")}, fixed)
		_, err := nodes.Present(context.Background(), PresentInput{RunID: "run-4", Plan: presentPlan()})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "event_failed" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("empty plan still snapshots", func(t *testing.T) {
		nodes := newPresentNodes(&eventRecorder{}, fixed)
		out, err := nodes.Present(context.Background(), PresentInput{RunID: "run-5"})
		if err != nil {
			t.Fatalf("present: %v", err)
		}
		if len(out.Snapshot.Explanations) != 0 || len(out.Snapshot.Diff.Days) != 0 {
			t.Fatalf("snapshot = %+v", out.Snapshot)
		}
	})
}
