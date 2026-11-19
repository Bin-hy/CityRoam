package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"trippilot/internal/domain"
)

func mealPOI(id, name string) domain.POI {
	return domain.POI{
		ID: id, City: "beijing", Name: name, Category: "餐饮",
		Lat: 39.90, Lng: 116.40, CoordSystem: domain.CoordSystemGCJ02,
		SuggestedMinutes: 60,
	}
}

func draftReply(days ...[]domain.DraftItem) json.RawMessage {
	plan := domain.DraftPlan{}
	for _, items := range days {
		plan.Days = append(plan.Days, domain.DraftDay{Date: "ignored", Items: items})
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		panic(err)
	}
	return encoded
}

func item(id string, kind string, minutes int) domain.DraftItem {
	return domain.DraftItem{POIID: id, Type: kind, DurationMin: minutes}
}

func TestDraftNode(t *testing.T) {
	candidates := append(makePOIs("p", 4), mealPOI("meal-1", "四季民福"))
	constraints := domain.ConstraintSet{
		City: "beijing", Days: 2, StartDate: "2026-12-01",
		Meals: domain.MealConstraint{Enabled: false},
	}

	t.Run("valid draft gets deterministic dates", func(t *testing.T) {
		llm := &fakeLLM{
			reply: draftReply(
				[]domain.DraftItem{item("p-0", "poi", 120), item("p-1", "poi", 90)},
				[]domain.DraftItem{item("p-2", "poi", 60), item("p-3", "poi", 60)},
			),
			usage: Usage{Model: "m", TotalTokens: 100, Attempts: 1},
		}
		nodes := NewNodes(llm, &eventRecorder{}, WithNodeLogger(discardLogger()),
			WithNodeClock(func() time.Time { return time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC) }))

		out, err := nodes.Draft(context.Background(), DraftInput{
			RunID: "run-1", Constraints: constraints, Candidates: candidates,
		})
		if err != nil {
			t.Fatalf("draft: %v", err)
		}
		if len(out.Draft.Days) != 2 || out.Draft.Days[0].Date != "2026-12-01" || out.Draft.Days[1].Date != "2026-12-02" {
			t.Fatalf("days = %+v", out.Draft.Days)
		}
		if len(out.POIs) != 5 {
			t.Fatalf("poi map = %d", len(out.POIs))
		}
		if out.Usage.TotalTokens != 100 || out.Usage.Attempts != 1 {
			t.Fatalf("usage = %+v", out.Usage)
		}
		if len(llm.calls) != 1 {
			t.Fatalf("calls = %d", len(llm.calls))
		}
		prompt := llm.calls[0].prompt
		if !strings.Contains(prompt, `"p-0"`) || !strings.Contains(prompt, "为 2 天") {
			t.Fatalf("prompt missing candidates/days:\n%s", prompt)
		}
		if string(llm.calls[0].schema) != string(DraftPlanSchema) {
			t.Fatal("wrong schema")
		}
	})

	t.Run("unknown id retries once and succeeds", func(t *testing.T) {
		llm := &fakeLLM{
			replies: []json.RawMessage{
				draftReply(
					[]domain.DraftItem{item("made-up", "poi", 120)},
					[]domain.DraftItem{item("p-1", "poi", 90)},
				),
				draftReply(
					[]domain.DraftItem{item("p-0", "poi", 120)},
					[]domain.DraftItem{item("p-1", "poi", 90)},
				),
			},
			usage: Usage{TotalTokens: 50, Attempts: 1},
		}
		nodes := NewNodes(llm, &eventRecorder{}, WithNodeLogger(discardLogger()))

		out, err := nodes.Draft(context.Background(), DraftInput{Constraints: constraints, Candidates: candidates})
		if err != nil {
			t.Fatalf("draft: %v", err)
		}
		if out.Draft.Days[0].Items[0].POIID != "p-0" {
			t.Fatalf("draft = %+v", out.Draft)
		}
		if len(llm.calls) != 2 {
			t.Fatalf("calls = %d", len(llm.calls))
		}
		if !strings.Contains(llm.calls[1].prompt, "候选集之外") {
			t.Fatalf("retry prompt missing explanation:\n%s", llm.calls[1].prompt)
		}
		if out.Usage.TotalTokens != 100 || out.Usage.Attempts != 2 {
			t.Fatalf("usage = %+v", out.Usage)
		}
	})

	t.Run("unknown id twice fails", func(t *testing.T) {
		llm := &fakeLLM{reply: draftReply(
			[]domain.DraftItem{item("made-up", "poi", 120)},
			[]domain.DraftItem{item("p-1", "poi", 90)},
		)}
		nodes := NewNodes(llm, &eventRecorder{}, WithNodeLogger(discardLogger()))
		_, err := nodes.Draft(context.Background(), DraftInput{Constraints: constraints, Candidates: candidates})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "draft_invalid" {
			t.Fatalf("err = %v", err)
		}
		if !strings.Contains(nodeErr.Message, "made-up") {
			t.Fatalf("message = %s", nodeErr.Message)
		}
		if len(llm.calls) != 2 {
			t.Fatalf("calls = %d", len(llm.calls))
		}
	})

	t.Run("meals enabled requires one per day", func(t *testing.T) {
		mealConstraints := constraints
		mealConstraints.Meals.Enabled = true
		llm := &fakeLLM{reply: draftReply(
			[]domain.DraftItem{item("p-0", "poi", 120)},
			[]domain.DraftItem{item("p-1", "poi", 90)},
		)}
		nodes := NewNodes(llm, &eventRecorder{}, WithNodeLogger(discardLogger()))
		_, err := nodes.Draft(context.Background(), DraftInput{Constraints: mealConstraints, Candidates: candidates})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || !strings.Contains(nodeErr.Message, "缺少餐饮安排") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("meal satisfies meal constraint", func(t *testing.T) {
		mealConstraints := constraints
		mealConstraints.Meals.Enabled = true
		llm := &fakeLLM{reply: draftReply(
			[]domain.DraftItem{item("p-0", "poi", 120), item("meal-1", "meal", 60)},
			[]domain.DraftItem{item("p-1", "poi", 90), item("meal-1", "meal", 60)},
		)}
		nodes := NewNodes(llm, &eventRecorder{}, WithNodeLogger(discardLogger()))
		out, err := nodes.Draft(context.Background(), DraftInput{Constraints: mealConstraints, Candidates: candidates})
		if err != nil {
			t.Fatalf("draft: %v", err)
		}
		if len(out.Draft.Days[0].Items) != 2 {
			t.Fatalf("draft = %+v", out.Draft)
		}
	})

	t.Run("wrong day count rejected", func(t *testing.T) {
		llm := &fakeLLM{reply: draftReply([]domain.DraftItem{item("p-0", "poi", 120)})}
		nodes := NewNodes(llm, &eventRecorder{}, WithNodeLogger(discardLogger()))
		_, err := nodes.Draft(context.Background(), DraftInput{Constraints: constraints, Candidates: candidates})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || !strings.Contains(nodeErr.Message, "恰好 2 天") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("empty candidates never calls llm", func(t *testing.T) {
		llm := &fakeLLM{}
		nodes := NewNodes(llm, &eventRecorder{}, WithNodeLogger(discardLogger()))
		_, err := nodes.Draft(context.Background(), DraftInput{Constraints: constraints})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "no_candidates" {
			t.Fatalf("err = %v", err)
		}
		if len(llm.calls) != 0 {
			t.Fatalf("calls = %d", len(llm.calls))
		}
	})

	t.Run("llm failure surfaces", func(t *testing.T) {
		nodes := NewNodes(&fakeLLM{err: errors.New("timeout")}, &eventRecorder{}, WithNodeLogger(discardLogger()))
		_, err := nodes.Draft(context.Background(), DraftInput{Constraints: constraints, Candidates: candidates})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "draft_llm_failed" {
			t.Fatalf("err = %v", err)
		}
	})
}
