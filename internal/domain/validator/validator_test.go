package validator

import (
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
			violations := violationsFor(museum, "2026-11-16", tt.start, domain.BlockTypePOI)
			if len(violations) != tt.wantCount {
				t.Fatalf("violations = %+v, want %d", violations, tt.wantCount)
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
	if violation.Day != 1 || violation.BlockID != "b1" || violation.POIID == "" {
		t.Fatalf("violation is missing location fields: %+v", violation)
	}
}
