package agent

import (
	"strings"
	"testing"

	"trippilot/internal/domain"
)

func blockingViolation(code string, day int) domain.Violation {
	return domain.Violation{Code: code, Level: domain.LevelBlocking, Day: day, Message: code + " violated"}
}

func planWith(cost int, walking float64) domain.Itinerary {
	return domain.Itinerary{
		City:   "beijing",
		Days:   []domain.DayPlan{{Date: "2026-11-20"}},
		Totals: domain.Totals{CostYuan: cost, WalkingKM: walking},
	}
}

func TestStopConditions(t *testing.T) {
	t.Run("cleared stops immediately", func(t *testing.T) {
		tracker := NewRepairTracker(3)
		tracker.Record(planWith(300, 4), nil)
		stop, reason := tracker.ShouldStop(false)
		if !stop || reason != StopCleared {
			t.Fatalf("stop=%v reason=%s", stop, reason)
		}
	})

	t.Run("cancelled wins over cleared", func(t *testing.T) {
		tracker := NewRepairTracker(3)
		tracker.Record(planWith(300, 4), nil)
		stop, reason := tracker.ShouldStop(true)
		if !stop || reason != StopCancelled {
			t.Fatalf("stop=%v reason=%s", stop, reason)
		}
	})

	t.Run("stops after three repair rounds", func(t *testing.T) {
		tracker := NewRepairTracker(3)
		steps := []int{5, 4, 3, 2}
		for _, blocking := range steps {
			violations := make([]domain.Violation, blocking)
			for i := range violations {
				violations[i] = blockingViolation("C4", 1)
			}
			tracker.Record(planWith(500, 10), violations)
			if stop, _ := tracker.ShouldStop(false); stop && len(tracker.History()) < len(steps) {
				t.Fatalf("stopped early at %d rounds", len(tracker.History()))
			}
		}
		stop, reason := tracker.ShouldStop(false)
		if !stop || reason != StopMaxRounds {
			t.Fatalf("stop=%v reason=%s", stop, reason)
		}
		if got := len(tracker.History()); got != 4 {
			t.Fatalf("history = %d", got)
		}
	})

	t.Run("two stagnant rounds stop before cap", func(t *testing.T) {
		tracker := NewRepairTracker(3)
		tracker.Record(planWith(500, 10), []domain.Violation{blockingViolation("C4", 1)})
		tracker.Record(planWith(500, 10), []domain.Violation{blockingViolation("C4", 1)})
		if stop, _ := tracker.ShouldStop(false); stop {
			t.Fatal("stopped after one stagnant round")
		}
		tracker.Record(planWith(500, 10), []domain.Violation{blockingViolation("C4", 1)})
		stop, reason := tracker.ShouldStop(false)
		if !stop || reason != StopNoImprovement {
			t.Fatalf("stop=%v reason=%s", stop, reason)
		}
	})

	t.Run("improvement resets stagnation", func(t *testing.T) {
		tracker := NewRepairTracker(3)
		tracker.Record(planWith(500, 10), []domain.Violation{blockingViolation("C4", 1), blockingViolation("C5", 0)})
		tracker.Record(planWith(500, 10), []domain.Violation{blockingViolation("C4", 1), blockingViolation("C5", 0)})
		tracker.Record(planWith(500, 10), []domain.Violation{blockingViolation("C4", 1)})
		if stop, _ := tracker.ShouldStop(false); stop {
			t.Fatal("improving round should reset the stagnant counter")
		}
	})

	t.Run("best prefers fewer blocking then warnings then cost", func(t *testing.T) {
		tracker := NewRepairTracker(3)
		warning := domain.Violation{Code: "C9", Level: domain.LevelWarning, Message: "too many"}
		tracker.Record(planWith(400, 10), []domain.Violation{blockingViolation("C4", 1)})
		tracker.Record(planWith(900, 20), []domain.Violation{warning, warning})
		tracker.Record(planWith(200, 5), []domain.Violation{warning})
		tracker.Record(planWith(300, 15), []domain.Violation{blockingViolation("C6", 2)})

		plan, violations, ok := tracker.Best()
		if !ok {
			t.Fatal("no best")
		}
		if plan.Totals.CostYuan != 200 {
			t.Fatalf("best cost = %d", plan.Totals.CostYuan)
		}
		if len(violations) != 1 || violations[0].Code != "C9" {
			t.Fatalf("best violations = %+v", violations)
		}
	})

	t.Run("walking breaks the final tie", func(t *testing.T) {
		tracker := NewRepairTracker(3)
		tracker.Record(planWith(300, 9.5), nil)
		tracker.Record(planWith(300, 3.2), nil)
		tracker.Record(planWith(300, 4.0), nil)
		plan, _, _ := tracker.Best()
		if plan.Totals.WalkingKM != 3.2 {
			t.Fatalf("best walking = %v", plan.Totals.WalkingKM)
		}
	})

	t.Run("unusable tracker never stops and has no best", func(t *testing.T) {
		tracker := NewRepairTracker(3)
		if stop, reason := tracker.ShouldStop(false); stop || reason != "" {
			t.Fatalf("stop=%v reason=%s", stop, reason)
		}
		if _, _, ok := tracker.Best(); ok {
			t.Fatal("empty tracker returned a best")
		}
	})

	t.Run("loop is bounded", func(t *testing.T) {
		tracker := NewRepairTracker(3)
		rounds := 0
		for rounds = 0; rounds < 50; rounds++ {
			tracker.Record(planWith(500, 10), []domain.Violation{blockingViolation("C4", 1)})
			if stop, _ := tracker.ShouldStop(false); stop {
				break
			}
		}
		if rounds >= 3 {
			t.Fatalf("loop ran %d repairs without stopping", rounds)
		}
	})
}

func TestRelaxationSuggestions(t *testing.T) {
	constraints := domain.ConstraintSet{}
	constraints.Daily.MaxHours = 8
	constraints.Meals.Enabled = true

	violations := []domain.Violation{
		{Code: "C5", Level: domain.LevelBlocking, Message: "over budget"},
		{Code: "C5", Level: domain.LevelBlocking, Message: "over budget again"},
		{Code: "C6", Level: domain.LevelWarning, Message: "meal late"},
		{Code: "C4", Level: domain.LevelWarning, Message: "too long"},
		{Code: "C99", Level: domain.LevelWarning, Message: "odd", Suggestion: "换成另一套方案"},
	}

	suggestions := RelaxationSuggestions(violations, constraints)
	if len(suggestions) != 4 {
		t.Fatalf("suggestions = %v", suggestions)
	}
	joined := strings.Join(suggestions, "\n")
	for _, want := range []string{"预算", "关闭餐饮", "9 小时", "换成另一套方案"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}

func TestRelaxationSuggestionsEmpty(t *testing.T) {
	if got := RelaxationSuggestions(nil, domain.ConstraintSet{}); len(got) != 0 {
		t.Fatalf("suggestions = %v", got)
	}
}
