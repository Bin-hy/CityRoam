package domain

import "testing"

func TestDraftFromPlan(t *testing.T) {
	plan := Itinerary{Days: []DayPlan{
		{
			Date: "2026-11-20",
			Blocks: []Block{
				{Type: BlockTypePOI, POIID: "poi-1", DurationMin: 120},
				{Type: BlockTypeTransit, Title: "步行"},
				{Type: BlockTypeMeal, POIID: "meal-1", DurationMin: 60},
				{Type: BlockTypePOI, DurationMin: 30},
			},
		},
	}}
	draft := DraftFromPlan(plan)
	if len(draft.Days) != 1 || draft.Days[0].Date != "2026-11-20" {
		t.Fatalf("draft days = %+v", draft.Days)
	}
	items := draft.Days[0].Items
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].POIID != "poi-1" || items[0].Type != BlockTypePOI || items[0].DurationMin != 120 {
		t.Fatalf("first item = %+v", items[0])
	}
	if items[1].POIID != "meal-1" || items[1].Type != BlockTypeMeal || items[1].DurationMin != 60 {
		t.Fatalf("second item = %+v", items[1])
	}
}
