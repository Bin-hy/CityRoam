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

func TestItineraryLodging(t *testing.T) {
	score := 4.8
	original := Itinerary{
		City: "beijing",
		Lodging: &LodgingSummary{
			Hotel:          Hotel{Name: "全季", Score: &score},
			Nights:         2,
			NightlyYuan:    500,
			TotalYuan:      1000,
			AnchorDays:     []int{0, 2},
			Source:         "user_selected",
			PriceEstimated: true,
		},
		Totals: Totals{CostYuan: 1500, LodgingYuan: 1000},
	}

	clone := original.Clone()
	clone.Lodging.Hotel.Name = "万豪"
	clone.Lodging.AnchorDays[0] = 1
	*clone.Lodging.Hotel.Score = 4.0
	clone.Totals.LodgingYuan = 0

	if original.Lodging.Hotel.Name != "全季" || original.Lodging.AnchorDays[0] != 0 {
		t.Fatalf("clone mutated lodging: %+v", original.Lodging)
	}
	if *original.Lodging.Hotel.Score != 4.8 {
		t.Fatalf("clone aliased hotel score: %v", *original.Lodging.Hotel.Score)
	}
	if original.Totals.LodgingYuan != 1000 {
		t.Fatalf("clone mutated totals: %+v", original.Totals)
	}

	empty := Itinerary{}
	if empty.Lodging != nil {
		t.Fatal("zero itinerary has lodging")
	}
	if (Itinerary{}).Clone().Lodging != nil {
		t.Fatal("empty clone has lodging")
	}
}

func TestItineraryCloneLegs(t *testing.T) {
	original := Itinerary{
		Days: []DayPlan{{
			Date: "2026-11-20",
			Blocks: []Block{{
				ID: "b1",
				Transport: &Transport{
					Mode: TransportTransit,
					Legs: []TransitLeg{
						{Mode: TransportWalk, DistanceM: 300, Polyline: [][2]float64{{116.40, 39.90}, {116.41, 39.91}}},
						{Mode: TransportTransit, LineName: "地铁 1 号线", DistanceM: 2400,
							Polyline: [][2]float64{{116.41, 39.91}, {116.42, 39.92}, {116.43, 39.93}}},
					},
				},
			}},
		}},
	}

	clone := original.Clone()
	clone.Days[0].Blocks[0].Transport.Legs[0].Polyline[0][0] = 0
	clone.Days[0].Blocks[0].Transport.Legs[1].LineName = "changed"
	clone.Days[0].Blocks[0].Transport.Legs = append(clone.Days[0].Blocks[0].Transport.Legs, TransitLeg{Mode: TransportWalk})

	source := original.Days[0].Blocks[0].Transport.Legs
	if len(source) != 2 {
		t.Fatalf("clone aliased legs slice: %+v", source)
	}
	if source[0].Polyline[0][0] != 116.40 {
		t.Fatalf("clone mutated polyline: %+v", source[0].Polyline)
	}
	if source[1].LineName != "地铁 1 号线" {
		t.Fatalf("clone mutated leg: %+v", source[1])
	}

	plain := Itinerary{Days: []DayPlan{{Blocks: []Block{{ID: "b2", Transport: &Transport{Mode: TransportWalk}}}}}}
	if legs := plain.Clone().Days[0].Blocks[0].Transport.Legs; legs != nil {
		t.Fatalf("empty legs = %+v", legs)
	}
}
