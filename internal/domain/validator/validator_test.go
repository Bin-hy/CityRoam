package validator

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"trippilot/internal/domain"
)

func TestC1(t *testing.T) {
	museum := domain.POI{
		ID:   "museum",
		Name: "博物馆",
		OpenHours: []domain.OpenRange{
			{Days: []int{1}, Start: "09:00", End: "12:00"},
			{Days: []int{1}, Start: "13:00", End: "17:00"},
		},
	}

	tests := []struct {
		name      string
		start     string
		wantCount int
		wantLevel domain.Level
	}{
		{name: "open", start: "10:00"},
		{name: "before open", start: "08:59", wantCount: 1, wantLevel: domain.LevelBlocking},
		{name: "lunch break", start: "12:30", wantCount: 1, wantLevel: domain.LevelBlocking},
		{name: "at close", start: "17:00", wantCount: 1, wantLevel: domain.LevelBlocking},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := violationsByCode(violationsFor(museum, "2026-11-16", tt.start, domain.BlockTypePOI), "C1")
			if len(violations) != tt.wantCount {
				t.Fatalf("C1 violations = %+v, want %d", violations, tt.wantCount)
			}
			if tt.wantCount > 0 {
				assertViolation(t, violations[0], "C1", tt.wantLevel)
			}
		})
	}

	t.Run("unknown hours becomes warning", func(t *testing.T) {
		unknown := domain.POI{ID: "unknown", Name: "未知地点"}
		violations := violationsByCode(violationsFor(unknown, "2026-11-16", "10:00", domain.BlockTypePOI), "C1")
		if len(violations) != 1 {
			t.Fatalf("violations = %+v, want 1", violations)
		}
		assertViolation(t, violations[0], "C1", domain.LevelWarning)
	})

	t.Run("invalid date becomes warning", func(t *testing.T) {
		violations := violationsByCode(violationsFor(museum, "", "10:00", domain.BlockTypePOI), "C1")
		if len(violations) != 1 {
			t.Fatalf("violations = %+v, want 1", violations)
		}
		assertViolation(t, violations[0], "C1", domain.LevelWarning)
	})
}

func TestC2(t *testing.T) {
	museum := domain.POI{
		ID:        "museum",
		Name:      "博物馆",
		OpenHours: []domain.OpenRange{{Days: []int{1}, Start: "09:00", End: "17:00"}},
	}

	t.Run("open weekday", func(t *testing.T) {
		violations := violationsFor(museum, "2026-11-16", "10:00", domain.BlockTypePOI)
		if c1 := violationsByCode(violations, "C1"); len(c1) != 0 {
			t.Fatalf("C1 violations = %+v, want none", c1)
		}
		if c2 := violationsByCode(violations, "C2"); len(c2) != 0 {
			t.Fatalf("C2 violations = %+v, want none", c2)
		}
	})

	t.Run("closed weekday", func(t *testing.T) {
		violations := violationsByCode(violationsFor(museum, "2026-11-17", "10:00", domain.BlockTypePOI), "C2")
		if len(violations) != 1 {
			t.Fatalf("violations = %+v, want 1", violations)
		}
		assertViolation(t, violations[0], "C2", domain.LevelBlocking)
	})

	t.Run("unknown hours does not claim closed day", func(t *testing.T) {
		unknown := domain.POI{ID: "unknown", Name: "未知地点"}
		violations := violationsFor(unknown, "2026-11-17", "10:00", domain.BlockTypePOI)
		if c2 := violationsByCode(violations, "C2"); len(c2) != 0 {
			t.Fatalf("C2 violations = %+v, want none", c2)
		}
		c1 := violationsByCode(violations, "C1")
		if len(c1) != 1 {
			t.Fatalf("C1 violations = %+v, want one warning", c1)
		}
		assertViolation(t, c1[0], "C1", domain.LevelWarning)
	})
}

func TestC3(t *testing.T) {
	tests := []struct {
		name         string
		currentArea  string
		currentStart string
		transport    domain.Transport
		routeErr     error
		wantCode     string
		wantLevel    domain.Level
	}{
		{
			name: "same area fits buffer", currentArea: "东城", currentStart: "10:20",
			transport: domain.Transport{DurationMin: 10},
		},
		{
			name: "same area too tight", currentArea: "东城", currentStart: "10:10",
			transport: domain.Transport{DurationMin: 10},
			wantCode:  "C3", wantLevel: domain.LevelBlocking,
		},
		{
			name: "cross area fits buffer", currentArea: "朝阳", currentStart: "10:30",
			transport: domain.Transport{DurationMin: 10},
		},
		{
			name: "cross area too tight", currentArea: "朝阳", currentStart: "10:20",
			transport: domain.Transport{DurationMin: 10},
			wantCode:  "C3", wantLevel: domain.LevelBlocking,
		},
		{
			name: "estimated route warns", currentArea: "东城", currentStart: "10:20",
			transport: domain.Transport{DurationMin: 10, Estimated: true},
			wantCode:  "C3", wantLevel: domain.LevelWarning,
		},
		{
			name: "route error warns", currentArea: "东城", currentStart: "10:20",
			routeErr: errors.New("route unavailable"),
			wantCode: "C3", wantLevel: domain.LevelWarning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, pois := twoStopPlan(tt.currentArea, tt.currentStart)
			// The validator trusts the transit allocated by the scheduler, so
			// only fall back to the provider when the plan has no allocation.
			plan.Days[0].Blocks[1].Transport = nil
			if tt.routeErr == nil {
				transport := tt.transport
				if transport.Mode == "" {
					transport.Mode = domain.TransportWalk
				}
				plan.Days[0].Blocks[1].Transport = &transport
			}
			routes := &fakeRouteProvider{transport: tt.transport, err: tt.routeErr}
			violations := Validate(plan, domain.ConstraintSet{}, pois, routes)

			c3 := violationsByCode(violations, "C3")
			if tt.wantCode == "" {
				if len(c3) != 0 {
					t.Fatalf("C3 violations = %+v, want none", c3)
				}
				return
			}
			if len(c3) != 1 {
				t.Fatalf("C3 violations = %+v, want 1", c3)
			}
			assertViolation(t, c3[0], tt.wantCode, tt.wantLevel)
		})
	}
}

func TestC4(t *testing.T) {
	t.Run("over daily max", func(t *testing.T) {
		plan, pois := singleBlockPlan("09:00", "19:00")
		constraints := domain.ConstraintSet{
			Daily: domain.DailyConstraint{StartTime: "09:00", EndTime: "21:00", MaxHours: 9},
		}
		violations := Validate(plan, constraints, pois, nil)
		c4 := violationsByCode(violations, "C4")
		if len(c4) != 1 {
			t.Fatalf("C4 violations = %+v, want 1", c4)
		}
		assertViolation(t, c4[0], "C4", domain.LevelBlocking)
	})

	t.Run("at daily max", func(t *testing.T) {
		plan, pois := singleBlockPlan("09:00", "18:00")
		constraints := domain.ConstraintSet{
			Daily: domain.DailyConstraint{StartTime: "09:00", EndTime: "21:00", MaxHours: 9},
		}
		if c4 := violationsByCode(Validate(plan, constraints, pois, nil), "C4"); len(c4) != 0 {
			t.Fatalf("C4 violations = %+v, want none", c4)
		}
	})
}

func TestC5(t *testing.T) {
	tests := []struct {
		name      string
		budget    domain.BudgetConstraint
		cost      int
		wantLevel domain.Level
	}{
		{name: "under soft budget", budget: domain.BudgetConstraint{TotalYuan: 500}, cost: 400},
		{name: "over soft budget warns", budget: domain.BudgetConstraint{TotalYuan: 500}, cost: 600, wantLevel: domain.LevelWarning},
		{name: "over hard budget blocks", budget: domain.BudgetConstraint{TotalYuan: 500, Hard: true}, cost: 600, wantLevel: domain.LevelBlocking},
		{name: "equal to budget", budget: domain.BudgetConstraint{TotalYuan: 500, Hard: true}, cost: 500},
		{name: "no budget set", cost: 600},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, pois := singleBlockPlan("09:00", "10:00")
			plan.Totals.CostYuan = tt.cost
			constraints := domain.ConstraintSet{Budget: tt.budget}
			c5 := violationsByCode(Validate(plan, constraints, pois, nil), "C5")

			if tt.wantLevel == "" {
				if len(c5) != 0 {
					t.Fatalf("C5 violations = %+v, want none", c5)
				}
				return
			}
			if len(c5) != 1 {
				t.Fatalf("C5 violations = %+v, want 1", c5)
			}
			if c5[0].Code != "C5" || c5[0].Level != tt.wantLevel {
				t.Fatalf("violation = %s/%s, want C5/%s", c5[0].Code, c5[0].Level, tt.wantLevel)
			}
			if c5[0].Day != 0 {
				t.Fatalf("C5 day = %d, want trip-level 0", c5[0].Day)
			}
		})
	}
}

func TestC6(t *testing.T) {
	enabled := domain.MealConstraint{Enabled: true}

	t.Run("no meal blocks", func(t *testing.T) {
		plan, pois := singleBlockPlan("09:00", "10:00")
		constraints := domain.ConstraintSet{Meals: enabled}
		c6 := violationsByCode(Validate(plan, constraints, pois, nil), "C6")
		if len(c6) != 1 {
			t.Fatalf("C6 violations = %+v, want 1", c6)
		}
		assertViolation(t, c6[0], "C6", domain.LevelBlocking)
	})

	t.Run("meals disabled", func(t *testing.T) {
		plan, pois := singleBlockPlan("09:00", "10:00")
		if c6 := violationsByCode(Validate(plan, domain.ConstraintSet{}, pois, nil), "C6"); len(c6) != 0 {
			t.Fatalf("C6 violations = %+v, want none", c6)
		}
	})

	tests := []struct {
		name      string
		start     string
		end       string
		wantLevel domain.Level
	}{
		{name: "lunch inside window", start: "12:00", end: "13:00"},
		{name: "dinner inside window", start: "18:00", end: "19:00"},
		{name: "slightly outside window", start: "14:00", end: "15:00"},
		{name: "far outside window", start: "15:00", end: "16:00", wantLevel: domain.LevelWarning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := mealPlan(tt.start, tt.end)
			constraints := domain.ConstraintSet{Meals: enabled}
			c6 := violationsByCode(Validate(plan, constraints, nil, nil), "C6")

			if tt.wantLevel == "" {
				if len(c6) != 0 {
					t.Fatalf("C6 violations = %+v, want none", c6)
				}
				return
			}
			if len(c6) != 1 {
				t.Fatalf("C6 violations = %+v, want 1", c6)
			}
			assertViolation(t, c6[0], "C6", tt.wantLevel)
		})
	}

	t.Run("invalid meal time", func(t *testing.T) {
		plan := mealPlan("晚点", "19:00")
		constraints := domain.ConstraintSet{Meals: enabled}
		c6 := violationsByCode(Validate(plan, constraints, nil, nil), "C6")
		if len(c6) != 1 {
			t.Fatalf("C6 violations = %+v, want 1", c6)
		}
		assertViolation(t, c6[0], "C6", domain.LevelWarning)
	})
}

func TestC7(t *testing.T) {
	plan := poiPlan(1, "a", "b", "c", "d")
	pois := map[string]domain.POI{}
	for _, id := range []string{"a", "b", "c", "d"} {
		pois[id] = openAllDayPOI(id, strings.ToUpper(id), "东城")
	}

	t.Run("all must-visit present", func(t *testing.T) {
		constraints := domain.ConstraintSet{MustVisit: []string{"A", "c"}}
		if c7 := violationsByCode(Validate(plan, constraints, pois, nil), "C7"); len(c7) != 0 {
			t.Fatalf("C7 violations = %+v, want none", c7)
		}
	})

	t.Run("shorthand and alias match", func(t *testing.T) {
		shorthandPlan := poiPlan(1, "museum")
		shorthandPOIs := map[string]domain.POI{
			"museum": {
				ID: "museum", Name: "中国科学技术馆", Aliases: []string{"科技馆"},
				City: "beijing", Category: "博物馆",
				OpenHours: []domain.OpenRange{{Days: []int{0, 1, 2, 3, 4, 5, 6}, Start: "00:00", End: "23:59"}},
			},
			"palace": {
				ID: "palace", Name: "故宫博物院", City: "beijing", Category: "历史",
				OpenHours: []domain.OpenRange{{Days: []int{0, 1, 2, 3, 4, 5, 6}, Start: "00:00", End: "23:59"}},
			},
		}
		shorthandPlan.Days[0].Blocks = append(shorthandPlan.Days[0].Blocks, poiBlock("palace", "10:30", 60))
		constraints := domain.ConstraintSet{MustVisit: []string{"科技馆", "故宫"}}
		if c7 := violationsByCode(Validate(shorthandPlan, constraints, shorthandPOIs, nil), "C7"); len(c7) != 0 {
			t.Fatalf("C7 violations = %+v, want none", c7)
		}
	})

	t.Run("missing must-visit", func(t *testing.T) {
		constraints := domain.ConstraintSet{MustVisit: []string{"A", "故宫"}}
		c7 := violationsByCode(Validate(plan, constraints, pois, nil), "C7")
		if len(c7) != 1 {
			t.Fatalf("C7 violations = %+v, want 1", c7)
		}
		if c7[0].Code != "C7" || c7[0].Level != domain.LevelBlocking {
			t.Fatalf("violation = %+v, want blocking C7", c7[0])
		}
	})
}

func TestC8(t *testing.T) {
	t.Run("duplicate within one day", func(t *testing.T) {
		plan := domain.Itinerary{Days: []domain.DayPlan{{
			Date:   "2026-11-16",
			Blocks: []domain.Block{poiBlock("a", "09:00", 60), poiBlock("a", "11:00", 60)},
		}}}
		c8 := violationsByCode(Validate(plan, domain.ConstraintSet{}, nil, nil), "C8")
		if len(c8) != 1 {
			t.Fatalf("C8 violations = %+v, want 1", c8)
		}
		assertViolation(t, c8[0], "C8", domain.LevelBlocking)
	})

	t.Run("duplicate across days", func(t *testing.T) {
		plan := domain.Itinerary{Days: []domain.DayPlan{
			{Date: "2026-11-16", Blocks: []domain.Block{poiBlock("a", "09:00", 60)}},
			{Date: "2026-11-17", Blocks: []domain.Block{poiBlock("a", "09:00", 60)}},
		}}
		c8 := violationsByCode(Validate(plan, domain.ConstraintSet{}, nil, nil), "C8")
		if len(c8) != 1 {
			t.Fatalf("C8 violations = %+v, want 1", c8)
		}
		if c8[0].Day != 2 {
			t.Fatalf("C8 day = %d, want 2", c8[0].Day)
		}
	})

	t.Run("no duplicates", func(t *testing.T) {
		plan := poiPlan(1, "a", "b", "c", "d")
		if c8 := violationsByCode(Validate(plan, domain.ConstraintSet{}, nil, nil), "C8"); len(c8) != 0 {
			t.Fatalf("C8 violations = %+v, want none", c8)
		}
	})
}

func TestC9(t *testing.T) {
	tests := []struct {
		name      string
		pace      domain.Pace
		poiCount  int
		wantCount int
	}{
		{name: "normal three", pace: domain.PaceNormal, poiCount: 3},
		{name: "normal two", pace: domain.PaceNormal, poiCount: 2, wantCount: 1},
		{name: "relaxed two", pace: domain.PaceRelaxed, poiCount: 2},
		{name: "relaxed four", pace: domain.PaceRelaxed, poiCount: 4, wantCount: 1},
		{name: "packed five", pace: domain.PacePacked, poiCount: 5},
		{name: "packed three", pace: domain.PacePacked, poiCount: 3, wantCount: 1},
	}

	ids := []string{"a", "b", "c", "d", "e"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := poiPlan(1, ids[:tt.poiCount]...)
			c9 := violationsByCode(Validate(plan, domain.ConstraintSet{Pace: tt.pace}, nil, nil), "C9")
			if len(c9) != tt.wantCount {
				t.Fatalf("C9 violations = %+v, want %d", c9, tt.wantCount)
			}
			if tt.wantCount > 0 {
				assertViolation(t, c9[0], "C9", domain.LevelWarning)
			}
		})
	}
}

func TestC12(t *testing.T) {
	poi := openAllDayPOI("poi", "景点", "东城")
	poi.SuggestedMinutes = 120
	pois := map[string]domain.POI{poi.ID: poi}

	tests := []struct {
		name        string
		duration    int
		wantWarning bool
	}{
		{name: "full stay", duration: 120},
		{name: "at threshold", duration: 72},
		{name: "below threshold", duration: 71, wantWarning: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := domain.Itinerary{Days: []domain.DayPlan{{
				Date:   "2026-11-16",
				Blocks: []domain.Block{poiBlock(poi.ID, "09:00", tt.duration)},
			}}}
			c12 := violationsByCode(Validate(plan, domain.ConstraintSet{}, pois, nil), "C12")
			if tt.wantWarning {
				if len(c12) != 1 {
					t.Fatalf("C12 violations = %+v, want 1", c12)
				}
				assertViolation(t, c12[0], "C12", domain.LevelWarning)
				return
			}
			if len(c12) != 0 {
				t.Fatalf("C12 violations = %+v, want none", c12)
			}
		})
	}

	t.Run("meal blocks are ignored", func(t *testing.T) {
		plan := mealPlan("12:00", "13:00")
		if c12 := violationsByCode(Validate(plan, domain.ConstraintSet{}, nil, nil), "C12"); len(c12) != 0 {
			t.Fatalf("C12 violations = %+v, want none", c12)
		}
	})
}

func TestC11(t *testing.T) {
	indoor := openAllDayPOI("in", "室内", "东城")
	outdoor := openAllDayPOI("out", "户外", "东城")
	outdoor.Outdoor = true
	pois := map[string]domain.POI{indoor.ID: indoor, outdoor.ID: outdoor}

	tests := []struct {
		name      string
		ids       []string
		rainAware bool
		wantCount int
	}{
		{name: "mostly outdoor warns", ids: []string{"out", "out"}, rainAware: true, wantCount: 1},
		{name: "half outdoor passes", ids: []string{"out", "in"}, rainAware: true},
		{name: "rain aware disabled", ids: []string{"out", "out"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := make([]domain.Block, 0, len(tt.ids))
			for _, id := range tt.ids {
				blocks = append(blocks, poiBlock(id, "09:00", 60))
			}
			plan := domain.Itinerary{Days: []domain.DayPlan{{Date: "2026-11-16", Blocks: blocks}}}
			constraints := domain.ConstraintSet{Weather: domain.WeatherConstraint{RainAware: tt.rainAware}}
			c11 := violationsByCode(Validate(plan, constraints, pois, nil), "C11")
			if len(c11) != tt.wantCount {
				t.Fatalf("C11 violations = %+v, want %d", c11, tt.wantCount)
			}
			if tt.wantCount > 0 {
				assertViolation(t, c11[0], "C11", domain.LevelWarning)
			}
		})
	}
}

func TestFullRuleCoverage(t *testing.T) {
	ticket := 50
	venue := openAllDayPOI("a", "A", "东城")
	venue.Outdoor = true
	venue.SuggestedMinutes = 180
	venue.TicketYuan = &ticket
	venue.OpenHours = []domain.OpenRange{{Days: []int{1}, Start: "09:00", End: "12:00"}}
	closedOnMonday := openAllDayPOI("b", "B", "东城")
	closedOnMonday.SuggestedMinutes = 60
	closedOnMonday.OpenHours = []domain.OpenRange{{Days: []int{2}, Start: "09:00", End: "17:00"}}
	pois := map[string]domain.POI{venue.ID: venue, closedOnMonday.ID: closedOnMonday}

	transit := domain.Transport{Mode: domain.TransportWalk, DurationMin: 10, DistanceM: 500}
	plan := domain.Itinerary{
		City: "beijing",
		Days: []domain.DayPlan{{
			Date:      "2026-11-16",
			StartTime: "08:30",
			EndTime:   "21:30",
			Blocks: []domain.Block{
				{ID: "b1", Type: domain.BlockTypePOI, POIID: venue.ID, Title: venue.Name, Start: "08:30", End: "09:30", DurationMin: 60},
				{ID: "t1", Type: domain.BlockTypeTransit, Title: "前往 A", Start: "09:30", End: "09:35", DurationMin: 5, Transport: &transit},
				{ID: "b2", Type: domain.BlockTypePOI, POIID: venue.ID, Title: venue.Name, Start: "09:35", End: "10:35", DurationMin: 60},
				{ID: "b3", Type: domain.BlockTypePOI, POIID: closedOnMonday.ID, Title: closedOnMonday.Name, Start: "10:35", End: "11:05", DurationMin: 30},
				{ID: "m1", Type: domain.BlockTypeMeal, Title: "午餐", Start: "15:00", End: "15:30", DurationMin: 30, CostYuan: 50},
			},
		}},
		Totals: domain.Totals{CostYuan: 200},
	}
	constraints := domain.ConstraintSet{
		City:      "beijing",
		Days:      1,
		Budget:    domain.BudgetConstraint{TotalYuan: 100, Hard: true},
		Daily:     domain.DailyConstraint{MaxHours: 9, StartTime: "09:00", EndTime: "21:00"},
		Meals:     domain.MealConstraint{Enabled: true},
		Pace:      domain.PacePacked,
		MustVisit: []string{"故宫"},
		Weather:   domain.WeatherConstraint{RainAware: true},
	}
	routes := &fakeRouteProvider{transport: transit}

	violations := Validate(plan, constraints, pois, routes)
	seen := make(map[string]bool)
	for _, violation := range violations {
		number, ok := ruleNumber(violation.Code)
		if !ok || number < 1 || number > 12 {
			t.Fatalf("unexpected violation code %q", violation.Code)
		}
		seen[violation.Code] = true
	}
	for number := 1; number <= 12; number++ {
		code := "C" + strconv.Itoa(number)
		if !seen[code] {
			t.Fatalf("code %s not covered by violations: %+v", code, violations)
		}
	}

	blockingSeen := false
	lastLevel := domain.LevelBlocking
	lastNumber := 0
	for _, violation := range violations {
		number, _ := ruleNumber(violation.Code)
		if violation.Level != lastLevel {
			if violation.Level != domain.LevelWarning || !blockingSeen {
				t.Fatalf("violations not grouped by level: %+v", violations)
			}
			lastLevel = violation.Level
			lastNumber = 0
		}
		if violation.Level == domain.LevelBlocking {
			blockingSeen = true
		}
		if number < lastNumber {
			t.Fatalf("violations not sorted by rule number: %+v", violations)
		}
		lastNumber = number
	}

	again := Validate(plan, constraints, pois, routes)
	if !reflect.DeepEqual(violations, again) {
		t.Fatalf("validator is not deterministic:\nfirst=%+v\nsecond=%+v", violations, again)
	}
}

func ruleNumber(code string) (int, bool) {
	if len(code) < 2 || code[0] != 'C' {
		return 0, false
	}
	number, err := strconv.Atoi(code[1:])
	return number, err == nil
}

func TestC10(t *testing.T) {
	tests := []struct {
		name      string
		start     string
		end       string
		wantCount int
	}{
		{name: "starts too early", start: "08:00", end: "09:00", wantCount: 1},
		{name: "ends too late", start: "20:00", end: "21:30", wantCount: 1},
		{name: "inside window", start: "09:00", end: "21:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, pois := singleBlockPlan(tt.start, tt.end)
			constraints := domain.ConstraintSet{
				Daily: domain.DailyConstraint{StartTime: "09:00", EndTime: "21:00", MaxHours: 14},
			}
			violations := Validate(plan, constraints, pois, nil)
			c10 := violationsByCode(violations, "C10")
			if len(c10) != tt.wantCount {
				t.Fatalf("C10 violations = %+v, want %d", c10, tt.wantCount)
			}
			if tt.wantCount > 0 {
				assertViolation(t, c10[0], "C10", domain.LevelBlocking)
			}
		})
	}
}

func twoStopPlan(currentArea, currentStart string) (domain.Itinerary, map[string]domain.POI) {
	first := openAllDayPOI("a", "A", "东城")
	second := openAllDayPOI("b", "B", currentArea)
	transit := domain.Transport{Mode: domain.TransportWalk, DurationMin: 10, DistanceM: 800}
	plan := domain.Itinerary{
		City: "beijing",
		Days: []domain.DayPlan{{
			Date:      "2026-11-16",
			StartTime: "09:00",
			EndTime:   "11:20",
			Blocks: []domain.Block{
				{ID: "b1", Type: domain.BlockTypePOI, POIID: first.ID, Title: first.Name, Start: "09:00", End: "10:00", DurationMin: 60},
				{ID: "t1", Type: domain.BlockTypeTransit, Title: "前往 B", Start: "10:00", End: currentStart, Transport: &transit},
				{ID: "b2", Type: domain.BlockTypePOI, POIID: second.ID, Title: second.Name, Start: currentStart, End: "11:20", DurationMin: 60},
			},
		}},
	}
	return plan, map[string]domain.POI{first.ID: first, second.ID: second}
}

func singleBlockPlan(start, end string) (domain.Itinerary, map[string]domain.POI) {
	poi := openAllDayPOI("poi", "POI", "东城")
	plan := domain.Itinerary{
		City: "beijing",
		Days: []domain.DayPlan{{
			Date:      "2026-11-16",
			StartTime: start,
			EndTime:   end,
			Blocks: []domain.Block{{
				ID: "b1", Type: domain.BlockTypePOI, POIID: poi.ID, Title: poi.Name,
				Start: start, End: end,
			}},
		}},
	}
	return plan, map[string]domain.POI{poi.ID: poi}
}

func poiPlan(days int, ids ...string) domain.Itinerary {
	plan := domain.Itinerary{}
	for day := 0; day < days; day++ {
		blocks := make([]domain.Block, 0, len(ids))
		for _, id := range ids {
			blocks = append(blocks, poiBlock(id, "09:00", 60))
		}
		plan.Days = append(plan.Days, domain.DayPlan{Date: "2026-11-16", Blocks: blocks})
	}
	return plan
}

func poiBlock(id, start string, durationMin int) domain.Block {
	startMinutes, _ := domain.ParseClock(start)
	return domain.Block{
		ID:          "b-" + id,
		Type:        domain.BlockTypePOI,
		POIID:       id,
		Title:       id,
		Start:       start,
		End:         domain.FormatClock(startMinutes + durationMin),
		DurationMin: durationMin,
	}
}

func mealPlan(start, end string) domain.Itinerary {
	return domain.Itinerary{
		City: "beijing",
		Days: []domain.DayPlan{{
			Date:      "2026-11-16",
			StartTime: start,
			EndTime:   end,
			Blocks: []domain.Block{{
				ID: "m1", Type: domain.BlockTypeMeal, Title: "午餐",
				Start: start, End: end, DurationMin: 60, CostYuan: 50,
			}},
		}},
		Totals: domain.Totals{CostYuan: 50},
	}
}

func openAllDayPOI(id, name, area string) domain.POI {
	return domain.POI{
		ID:        id,
		Name:      name,
		Area:      area,
		OpenHours: []domain.OpenRange{{Days: []int{0, 1, 2, 3, 4, 5, 6}, Start: "00:00", End: "23:59"}},
	}
}

func violationsByCode(violations []domain.Violation, code string) []domain.Violation {
	matched := make([]domain.Violation, 0)
	for _, violation := range violations {
		if violation.Code == code {
			matched = append(matched, violation)
		}
	}
	return matched
}

type fakeRouteProvider struct {
	transport domain.Transport
	err       error
}

func (f *fakeRouteProvider) Route(_ context.Context, from, to domain.POI, mode string) (domain.Transport, error) {
	if f.err != nil {
		return domain.Transport{}, f.err
	}
	transport := f.transport
	if transport.Mode == "" {
		transport.Mode = mode
	}
	transport.FromPOIID = from.ID
	transport.ToPOIID = to.ID
	return transport, nil
}

func violationsFor(poi domain.POI, date, start, blockType string) []domain.Violation {
	plan := domain.Itinerary{
		City: "beijing",
		Days: []domain.DayPlan{{
			Date: date,
			Blocks: []domain.Block{{
				ID:    "b1",
				Type:  blockType,
				POIID: poi.ID,
				Start: start,
				End:   start,
				Title: poi.Name,
			}},
		}},
	}
	return Validate(plan, domain.ConstraintSet{}, map[string]domain.POI{poi.ID: poi}, nil)
}

func assertViolation(t *testing.T, violation domain.Violation, wantCode string, wantLevel domain.Level) {
	t.Helper()
	if !violation.Valid() {
		t.Fatalf("invalid violation: %+v", violation)
	}
	if violation.Code != wantCode || violation.Level != wantLevel {
		t.Fatalf("violation = %s/%s, want %s/%s (%+v)",
			violation.Code, violation.Level, wantCode, wantLevel, violation)
	}
	if violation.Day != 1 {
		t.Fatalf("violation day = %d, want 1: %+v", violation.Day, violation)
	}
}
