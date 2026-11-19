package validator

import (
	"context"
	"errors"
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
		violations := violationsFor(unknown, "2026-11-16", "10:00", domain.BlockTypePOI)
		if len(violations) != 1 {
			t.Fatalf("violations = %+v, want 1", violations)
		}
		assertViolation(t, violations[0], "C1", domain.LevelWarning)
	})

	t.Run("invalid date becomes warning", func(t *testing.T) {
		violations := violationsFor(museum, "", "10:00", domain.BlockTypePOI)
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
		if violations := violationsFor(museum, "2026-11-16", "10:00", domain.BlockTypePOI); len(violations) != 0 {
			t.Fatalf("violations = %+v, want none", violations)
		}
	})

	t.Run("closed weekday", func(t *testing.T) {
		violations := violationsFor(museum, "2026-11-17", "10:00", domain.BlockTypePOI)
		if len(violations) != 1 {
			t.Fatalf("violations = %+v, want 1", violations)
		}
		assertViolation(t, violations[0], "C2", domain.LevelBlocking)
	})

	t.Run("unknown hours does not claim closed day", func(t *testing.T) {
		unknown := domain.POI{ID: "unknown", Name: "未知地点"}
		violations := violationsFor(unknown, "2026-11-17", "10:00", domain.BlockTypePOI)
		if len(violations) != 1 {
			t.Fatalf("violations = %+v, want only a C1 warning", violations)
		}
		assertViolation(t, violations[0], "C1", domain.LevelWarning)
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
