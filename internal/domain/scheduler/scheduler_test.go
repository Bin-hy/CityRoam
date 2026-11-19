package scheduler

import (
	"context"
	"fmt"
	"testing"

	"trippilot/internal/domain"
)

func TestSchedulerStart(t *testing.T) {
	museum := testPOI("museum", "Museum", "东城", 39.9163, 116.3972, nil, 120)
	pois := map[string]domain.POI{museum.ID: museum}

	t.Run("crosses noon", func(t *testing.T) {
		constraints := domain.ConstraintSet{
			City: "beijing",
			Daily: domain.DailyConstraint{
				StartTime: "09:00",
				EndTime:   "21:00",
				MaxHours:  9,
			},
		}
		draft := domain.DraftPlan{Days: []domain.DraftDay{{
			Date:  "2026-11-16",
			Items: []domain.DraftItem{{POIID: museum.ID, Type: domain.ItemTypePOI, DurationMin: 180}},
		}}}

		plan, err := New().Build(draft, constraints, pois, nil)
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if len(plan.Days) != 1 {
			t.Fatalf("days = %d, want 1", len(plan.Days))
		}
		day := plan.Days[0]
		if day.StartTime != "09:00" || day.EndTime != "12:00" {
			t.Fatalf("day window = %s-%s, want 09:00-12:00", day.StartTime, day.EndTime)
		}
		if len(day.Blocks) != 1 {
			t.Fatalf("blocks = %d, want 1: %+v", len(day.Blocks), day.Blocks)
		}
		block := day.Blocks[0]
		if block.Type != domain.BlockTypePOI || block.POIID != museum.ID {
			t.Fatalf("first block = %+v, want POI %s", block, museum.ID)
		}
		if block.Start != "09:00" || block.End != "12:00" || block.DurationMin != 180 {
			t.Fatalf("block time = %+v, want 09:00-12:00 for 180 minutes", block)
		}
		if plan.Totals.POICount != 1 {
			t.Fatalf("POICount = %d, want 1", plan.Totals.POICount)
		}
	})

	t.Run("reports end after last block", func(t *testing.T) {
		constraints := domain.ConstraintSet{
			City: "beijing",
			Daily: domain.DailyConstraint{
				StartTime: "20:00",
				EndTime:   "21:00",
				MaxHours:  9,
			},
		}
		draft := domain.DraftPlan{Days: []domain.DraftDay{{
			Date:  "2026-11-16",
			Items: []domain.DraftItem{{POIID: museum.ID, Type: domain.ItemTypePOI, DurationMin: 90}},
		}}}

		plan, err := New().Build(draft, constraints, pois, nil)
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		day := plan.Days[0]
		if day.StartTime != "20:00" || day.EndTime != "21:30" {
			t.Fatalf("day window = %s-%s, want 20:00-21:30", day.StartTime, day.EndTime)
		}
		if got := day.Blocks[0].End; got != "21:30" {
			t.Fatalf("block end = %s, want 21:30", got)
		}
	})
}

func TestSchedulerTransit(t *testing.T) {
	a := testPOI("a", "A", "东城", 39.9163, 116.3972, nil, 60)
	b := testPOI("b", "B", "东城", 39.9170, 116.3980, nil, 60)
	c := testPOI("c", "C", "朝阳", 39.9900, 116.4700, nil, 60)
	pois := map[string]domain.POI{a.ID: a, b.ID: b, c.ID: c}

	routes := &fakeRouteProvider{
		routes: map[string]domain.Transport{
			"a->b": {DurationMin: 20, DistanceM: 900, CostYuan: 0},
			"b->c": {DurationMin: 30, DistanceM: 5000, CostYuan: 0},
		},
	}
	constraints := domain.ConstraintSet{
		City:  "beijing",
		Daily: domain.DailyConstraint{StartTime: "09:00", EndTime: "21:00", MaxHours: 12},
	}
	draft := domain.DraftPlan{Days: []domain.DraftDay{{
		Date: "2026-11-16",
		Items: []domain.DraftItem{
			{POIID: a.ID, Type: domain.ItemTypePOI, DurationMin: 60, StartHint: "09:00"},
			{POIID: b.ID, Type: domain.ItemTypePOI, DurationMin: 60, StartHint: "10:30"},
			{POIID: c.ID, Type: domain.ItemTypePOI, DurationMin: 60, StartHint: "13:00"},
		},
	}}}

	plan, err := New().Build(draft, constraints, pois, routes)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	blocks := plan.Days[0].Blocks
	wantTypes := []string{
		domain.BlockTypePOI,
		domain.BlockTypeTransit,
		domain.BlockTypePOI,
		domain.BlockTypeTransit,
		domain.BlockTypeRest,
		domain.BlockTypePOI,
	}
	if len(blocks) != len(wantTypes) {
		t.Fatalf("blocks = %d, want %d: %+v", len(blocks), len(wantTypes), blocks)
	}
	for i, want := range wantTypes {
		if blocks[i].Type != want {
			t.Fatalf("blocks[%d].Type = %s, want %s", i, blocks[i].Type, want)
		}
	}

	if blocks[0].Start != "09:00" || blocks[0].End != "10:00" {
		t.Fatalf("first block = %s-%s, want 09:00-10:00", blocks[0].Start, blocks[0].End)
	}
	if blocks[1].Start != "10:00" || blocks[1].End != "10:30" || blocks[1].DurationMin != 30 {
		t.Fatalf("same-area transit = %+v, want 10:00-10:30 for 30 minutes", blocks[1])
	}
	if blocks[1].Transport == nil || blocks[1].Transport.DurationMin != 20 {
		t.Fatalf("same-area transport = %+v, want route duration 20", blocks[1].Transport)
	}
	if blocks[2].Start != "10:30" || blocks[2].End != "11:30" {
		t.Fatalf("second POI = %s-%s, want 10:30-11:30", blocks[2].Start, blocks[2].End)
	}
	if blocks[3].Start != "11:30" || blocks[3].End != "12:20" || blocks[3].DurationMin != 50 {
		t.Fatalf("cross-area transit = %+v, want 11:30-12:20 for 50 minutes", blocks[3])
	}
	if blocks[4].Start != "12:20" || blocks[4].End != "13:00" || blocks[4].DurationMin != 40 {
		t.Fatalf("wait block = %+v, want 12:20-13:00 for 40 minutes", blocks[4])
	}
	if blocks[5].Start != "13:00" || blocks[5].End != "14:00" {
		t.Fatalf("third POI = %s-%s, want 13:00-14:00", blocks[5].Start, blocks[5].End)
	}
	if plan.Days[0].EndTime != "14:00" {
		t.Fatalf("day end = %s, want 14:00", plan.Days[0].EndTime)
	}

	if got := routes.modes["a->b"]; got != domain.TransportWalk {
		t.Fatalf("a->b mode = %s, want walk", got)
	}
	if got := routes.modes["b->c"]; got != domain.TransportTransit {
		t.Fatalf("b->c mode = %s, want transit", got)
	}
}

func TestSchedulerCost(t *testing.T) {
	ticket := 60
	museum := testPOI("museum", "Museum", "东城", 39.9163, 116.3972, &ticket, 120)
	meal := testPOI("meal", "Lunch", "东城", 39.9170, 116.3980, nil, 60)
	park := testPOI("park", "Park", "东城", 39.9180, 116.3990, nil, 60)
	pois := map[string]domain.POI{museum.ID: museum, meal.ID: meal, park.ID: park}

	routes := &fakeRouteProvider{
		routes: map[string]domain.Transport{
			"museum->meal": {Mode: domain.TransportTransit, DurationMin: 10, DistanceM: 1200, CostYuan: 0},
			"meal->park":   {Mode: domain.TransportWalk, DurationMin: 5, DistanceM: 800, CostYuan: 0},
		},
	}
	constraints := domain.ConstraintSet{
		City:  "beijing",
		Daily: domain.DailyConstraint{StartTime: "09:00", EndTime: "21:00", MaxHours: 12},
		Meals: domain.MealConstraint{Enabled: true, BudgetPerMealYuan: 50},
	}
	draft := domain.DraftPlan{Days: []domain.DraftDay{{
		Date: "2026-11-16",
		Items: []domain.DraftItem{
			{POIID: museum.ID, Type: domain.ItemTypePOI, DurationMin: 120},
			{POIID: meal.ID, Type: domain.ItemTypeMeal, DurationMin: 60},
			{POIID: park.ID, Type: domain.ItemTypePOI, DurationMin: 60},
		},
	}}}

	plan, err := New().Build(draft, constraints, pois, routes)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	day := plan.Days[0]
	if len(day.Blocks) != 5 {
		t.Fatalf("blocks = %d, want 5: %+v", len(day.Blocks), day.Blocks)
	}

	if day.Blocks[0].CostYuan != 60 {
		t.Fatalf("ticket cost = %d, want 60", day.Blocks[0].CostYuan)
	}
	firstTransit := day.Blocks[1]
	if firstTransit.CostYuan != 2 {
		t.Fatalf("transit cost = %d, want estimated 2", firstTransit.CostYuan)
	}
	if firstTransit.Transport == nil || !firstTransit.Transport.Estimated {
		t.Fatalf("transit estimated flag = %+v, want true", firstTransit.Transport)
	}
	if day.Blocks[2].CostYuan != 50 {
		t.Fatalf("meal cost = %d, want 50", day.Blocks[2].CostYuan)
	}
	secondTransit := day.Blocks[3]
	if secondTransit.CostYuan != 0 || secondTransit.Transport == nil || secondTransit.Transport.Estimated {
		t.Fatalf("walk cost = %+v, want free and not estimated", secondTransit)
	}
	if day.Blocks[4].CostYuan != 0 {
		t.Fatalf("free POI cost = %d, want 0", day.Blocks[4].CostYuan)
	}

	if day.DayCostYuan != 112 {
		t.Fatalf("day cost = %d, want 112", day.DayCostYuan)
	}
	if plan.Totals.CostYuan != 112 {
		t.Fatalf("total cost = %d, want 112", plan.Totals.CostYuan)
	}
	if plan.Totals.POICount != 2 {
		t.Fatalf("POICount = %d, want 2", plan.Totals.POICount)
	}
	if plan.Totals.WalkingKM != 0.8 {
		t.Fatalf("WalkingKM = %f, want 0.8", plan.Totals.WalkingKM)
	}
}

func TestSchedulerErrors(t *testing.T) {
	constraints := domain.ConstraintSet{City: "beijing"}
	missing := domain.DraftPlan{Days: []domain.DraftDay{{
		Items: []domain.DraftItem{{POIID: "missing", DurationMin: 60}},
	}}}
	if _, err := New().Build(missing, constraints, nil, nil); err == nil {
		t.Fatal("Build() error = nil for unknown POI")
	}

	pois := map[string]domain.POI{
		"a": testPOI("a", "A", "东城", 39.9, 116.3, nil, 60),
		"b": testPOI("b", "B", "东城", 39.91, 116.31, nil, 60),
	}
	multiStop := domain.DraftPlan{Days: []domain.DraftDay{{
		Items: []domain.DraftItem{
			{POIID: "a", DurationMin: 60},
			{POIID: "b", DurationMin: 60},
		},
	}}}
	if _, err := New().Build(multiStop, constraints, pois, nil); err == nil {
		t.Fatal("Build() error = nil without a route provider")
	}
}

type fakeRouteProvider struct {
	routes map[string]domain.Transport
	modes  map[string]string
}

func (f *fakeRouteProvider) Route(_ context.Context, from, to domain.POI, mode string) (domain.Transport, error) {
	key := from.ID + "->" + to.ID
	transport, found := f.routes[key]
	if !found {
		return domain.Transport{}, fmt.Errorf("no fake route for %s", key)
	}
	if f.modes == nil {
		f.modes = make(map[string]string)
	}
	f.modes[key] = mode
	if transport.Mode == "" {
		transport.Mode = mode
	}
	transport.FromPOIID = from.ID
	transport.ToPOIID = to.ID
	return transport, nil
}

func testPOI(id, name, area string, lat, lng float64, ticket *int, suggested int) domain.POI {
	return domain.POI{
		ID:               id,
		City:             "beijing",
		Name:             name,
		Area:             area,
		Lat:              lat,
		Lng:              lng,
		CoordSystem:      domain.CoordSystemGCJ02,
		TicketYuan:       ticket,
		SuggestedMinutes: suggested,
	}
}
