package budget

import (
	"reflect"
	"testing"

	"trippilot/internal/domain"
)

func TestSummarize(t *testing.T) {
	t.Run("empty itinerary", func(t *testing.T) {
		summary := Summarize(domain.Itinerary{})
		if len(summary.Days) != 0 {
			t.Fatalf("days = %+v, want none", summary.Days)
		}
		if summary.Breakdown != (Breakdown{}) {
			t.Fatalf("breakdown = %+v, want zero", summary.Breakdown)
		}
		if summary.Days == nil {
			t.Fatal("days must be an empty slice, not nil")
		}
	})

	t.Run("mixed categories", func(t *testing.T) {
		plan := domain.Itinerary{Days: []domain.DayPlan{
			{
				Date: "2026-11-16",
				Blocks: []domain.Block{
					{ID: "b1", Type: domain.BlockTypePOI, CostYuan: 50},
					{ID: "t1", Type: domain.BlockTypeTransit, CostYuan: 2},
					{ID: "m1", Type: domain.BlockTypeMeal, CostYuan: 80},
					{ID: "r1", Type: domain.BlockTypeRest, CostYuan: 0},
				},
			},
			{
				Date: "2026-11-17",
				Blocks: []domain.Block{
					{ID: "b2", Type: domain.BlockTypePOI, CostYuan: 0},
					{ID: "m2", Type: domain.BlockTypeMeal, CostYuan: 60},
				},
			},
		}}

		summary := Summarize(plan)
		if len(summary.Days) != 2 {
			t.Fatalf("days = %+v, want 2", summary.Days)
		}

		wantDay := Day{Date: "2026-11-16", Breakdown: Breakdown{TicketsYuan: 50, MealsYuan: 80, TransitYuan: 2, TotalYuan: 132}}
		if !reflect.DeepEqual(summary.Days[0], wantDay) {
			t.Fatalf("day 1 = %+v, want %+v", summary.Days[0], wantDay)
		}

		wantTotal := Breakdown{TicketsYuan: 50, MealsYuan: 140, TransitYuan: 2, TotalYuan: 192}
		if summary.Breakdown != wantTotal {
			t.Fatalf("total = %+v, want %+v", summary.Breakdown, wantTotal)
		}
	})

	t.Run("single day", func(t *testing.T) {
		plan := domain.Itinerary{Days: []domain.DayPlan{{
			Date:   "2026-11-16",
			Blocks: []domain.Block{{ID: "m1", Type: domain.BlockTypeMeal, CostYuan: 45}},
		}}}
		summary := Summarize(plan)
		want := Breakdown{MealsYuan: 45, TotalYuan: 45}
		if summary.Breakdown != want {
			t.Fatalf("total = %+v, want %+v", summary.Breakdown, want)
		}
	})
}
