package domain

import (
	"reflect"
	"testing"
)

func TestItinerary(t *testing.T) {
	if ids := (DraftPlan{}).POIIDs(); len(ids) != 0 {
		t.Fatalf("empty DraftPlan POIIDs() = %v, want empty", ids)
	}

	draft := DraftPlan{
		Days: []DraftDay{
			{Date: "2026-11-16", Items: []DraftItem{
				{POIID: "forbidden-city", Type: ItemTypePOI},
				{POIID: "lunch-hutong", Type: ItemTypeMeal},
			}},
			{Date: "2026-11-17", Items: []DraftItem{
				{POIID: "national-museum", Type: ItemTypePOI},
			}},
		},
	}
	wantIDs := []string{"forbidden-city", "lunch-hutong", "national-museum"}
	if got := draft.POIIDs(); !reflect.DeepEqual(got, wantIDs) {
		t.Fatalf("DraftPlan POIIDs() = %v, want %v", got, wantIDs)
	}

	empty := Itinerary{}
	if _, ok := empty.DayByIndex(0); ok {
		t.Fatal("empty itinerary returned a day")
	}

	itinerary := Itinerary{
		City: "beijing",
		Days: []DayPlan{
			{Date: "2026-11-16", Blocks: []Block{{ID: "d1-b1"}}},
			{Date: "2026-11-17", Blocks: []Block{{ID: "d2-b1"}, {ID: "d2-b2"}}},
		},
		Totals: Totals{CostYuan: 800, POICount: 4, WalkingKM: 6.5},
	}

	tests := []struct {
		index   int
		wantOK  bool
		wantDay string
	}{
		{index: 0, wantOK: true, wantDay: "2026-11-16"},
		{index: 1, wantOK: true, wantDay: "2026-11-17"},
		{index: -1, wantOK: false},
		{index: 2, wantOK: false},
	}
	for _, tt := range tests {
		day, ok := itinerary.DayByIndex(tt.index)
		if ok != tt.wantOK {
			t.Fatalf("DayByIndex(%d) ok = %v, want %v", tt.index, ok, tt.wantOK)
		}
		if ok && day.Date != tt.wantDay {
			t.Fatalf("DayByIndex(%d) date = %q, want %q", tt.index, day.Date, tt.wantDay)
		}
	}
}

func TestItineraryClone(t *testing.T) {
	original := Itinerary{
		City: "beijing",
		Days: []DayPlan{
			{
				Date: "2026-11-16",
				Blocks: []Block{
					{ID: "b1", Title: "Palace", Transport: &Transport{Mode: TransportWalk, DurationMin: 5}},
					{ID: "b2", Title: "Lunch"},
				},
				DayCostYuan: 300,
			},
		},
		Totals: Totals{CostYuan: 300, POICount: 2, WalkingKM: 1.2},
	}

	clone := original.Clone()
	clone.City = "shanghai"
	clone.Days[0].Date = "2026-12-01"
	clone.Days[0].Blocks[0].Title = "Changed"
	clone.Days[0].Blocks[1].Title = "Changed too"
	clone.Days[0].Blocks[0].Transport.Mode = TransportDrive
	clone.Totals.CostYuan = 999

	if original.City != "beijing" || original.Days[0].Date != "2026-11-16" {
		t.Fatalf("clone mutated original itinerary: %+v", original)
	}
	if original.Days[0].Blocks[0].Title != "Palace" || original.Days[0].Blocks[1].Title != "Lunch" {
		t.Fatalf("clone aliased original blocks: %+v", original.Days[0].Blocks)
	}
	if original.Days[0].Blocks[0].Transport.Mode != TransportWalk {
		t.Fatalf("clone aliased original transport: %+v", original.Days[0].Blocks[0].Transport)
	}
	if original.Totals.CostYuan != 300 {
		t.Fatalf("clone mutated original totals: %+v", original.Totals)
	}

	emptyClone := (Itinerary{}).Clone()
	if emptyClone.Days != nil {
		t.Fatalf("empty clone Days = %v, want nil", emptyClone.Days)
	}
}
