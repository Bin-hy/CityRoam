package diff

import (
	"testing"

	"trippilot/internal/domain"
)

func TestDiff(t *testing.T) {
	t.Run("identical versions", func(t *testing.T) {
		plan := itinerary(
			day("2026-11-16", block("b1", "09:00", "10:00")),
		)
		result := Diff(plan, plan)
		if len(result.Days) != 0 || len(result.Blocks) != 0 {
			t.Fatalf("result = %+v, want no changes", result)
		}
	})

	t.Run("added day", func(t *testing.T) {
		before := itinerary(day("2026-11-16", block("b1", "09:00", "10:00")))
		after := itinerary(
			day("2026-11-16", block("b1", "09:00", "10:00")),
			day("2026-11-17", block("b2", "09:00", "10:00")),
		)

		result := Diff(before, after)
		if len(result.Days) != 1 {
			t.Fatalf("days = %+v, want 1", result.Days)
		}
		change := result.Days[0]
		if change.Change != Added || change.DayIndex != 1 || change.After == nil || change.Before != nil {
			t.Fatalf("day change = %+v, want added day 1", change)
		}
	})

	t.Run("removed day", func(t *testing.T) {
		before := itinerary(
			day("2026-11-16", block("b1", "09:00", "10:00")),
			day("2026-11-17", block("b2", "09:00", "10:00")),
		)
		after := itinerary(day("2026-11-16", block("b1", "09:00", "10:00")))

		result := Diff(before, after)
		if len(result.Days) != 1 || result.Days[0].Change != Removed {
			t.Fatalf("days = %+v, want one removed day", result.Days)
		}
		if result.Days[0].Before == nil || result.Days[0].After != nil {
			t.Fatalf("day change = %+v, want before only", result.Days[0])
		}
	})

	t.Run("modified blocks", func(t *testing.T) {
		before := itinerary(day("2026-11-16",
			block("b1", "09:00", "10:00"),
			block("b2", "10:30", "11:30"),
		))
		after := itinerary(day("2026-11-16",
			block("b1", "09:00", "10:30"),
			block("b3", "11:00", "12:00"),
		))

		result := Diff(before, after)
		if len(result.Days) != 1 || result.Days[0].Change != Modified {
			t.Fatalf("days = %+v, want one modified day", result.Days)
		}

		changes := map[string]Change{}
		for _, blockChange := range result.Blocks {
			changes[blockChange.BlockID] = blockChange.Change
		}
		want := map[string]Change{"b1": Modified, "b2": Removed, "b3": Added}
		if len(changes) != len(want) {
			t.Fatalf("block changes = %+v, want %+v", changes, want)
		}
		for id, change := range want {
			if changes[id] != change {
				t.Fatalf("block %s change = %q, want %q", id, changes[id], change)
			}
		}
	})

	t.Run("date change without block change", func(t *testing.T) {
		before := itinerary(day("2026-11-16", block("b1", "09:00", "10:00")))
		after := itinerary(day("2026-11-17", block("b1", "09:00", "10:00")))

		result := Diff(before, after)
		if len(result.Days) != 1 || result.Days[0].Change != Modified {
			t.Fatalf("days = %+v, want one modified day", result.Days)
		}
		if len(result.Blocks) != 0 {
			t.Fatalf("blocks = %+v, want none", result.Blocks)
		}
	})
}

func itinerary(days ...domain.DayPlan) domain.Itinerary {
	return domain.Itinerary{City: "beijing", Days: days}
}

func day(date string, blocks ...domain.Block) domain.DayPlan {
	return domain.DayPlan{Date: date, Blocks: blocks}
}

func block(id, start, end string) domain.Block {
	return domain.Block{
		ID:    id,
		Type:  domain.BlockTypePOI,
		POIID: id,
		Start: start,
		End:   end,
	}
}
