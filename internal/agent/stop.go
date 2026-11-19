package agent

import (
	"fmt"
	"strings"

	"trippilot/internal/domain"
)

const defaultMaxRepairRounds = 3

type StopReason string

const (
	StopCleared       StopReason = "cleared"
	StopMaxRounds     StopReason = "max_rounds"
	StopNoImprovement StopReason = "no_improvement"
	StopCancelled     StopReason = "cancelled"
)

// RoundMetrics is the comparable summary of one repair round.
type RoundMetrics struct {
	Round     int     `json:"round"`
	Blocking  int     `json:"blocking"`
	Warnings  int     `json:"warnings"`
	CostYuan  int     `json:"cost_yuan"`
	WalkingKM float64 `json:"walking_km"`
}

type repairEntry struct {
	metrics    RoundMetrics
	plan       domain.Itinerary
	violations []domain.Violation
}

// RepairTracker feeds every round's plan and violations, remembers the best
// one and decides when the repair loop must stop.
type RepairTracker struct {
	maxRounds int
	entries   []repairEntry
	bestIndex int
	stagnant  int
}

func NewRepairTracker(maxRounds int) *RepairTracker {
	if maxRounds <= 0 {
		maxRounds = defaultMaxRepairRounds
	}
	return &RepairTracker{maxRounds: maxRounds, bestIndex: -1}
}

// MetricsFor summarizes a plan and its violations for comparison.
func MetricsFor(round int, plan domain.Itinerary, violations []domain.Violation) RoundMetrics {
	counts := domain.CountByLevel(violations)
	return RoundMetrics{
		Round:     round,
		Blocking:  counts[domain.LevelBlocking],
		Warnings:  counts[domain.LevelWarning],
		CostYuan:  plan.Totals.CostYuan,
		WalkingKM: plan.Totals.WalkingKM,
	}
}

// Record stores one round. Round 0 is the initial draft, each repair adds one.
func (t *RepairTracker) Record(plan domain.Itinerary, violations []domain.Violation) RoundMetrics {
	metrics := MetricsFor(len(t.entries), plan, violations)
	entry := repairEntry{metrics: metrics, plan: plan, violations: append([]domain.Violation(nil), violations...)}
	t.entries = append(t.entries, entry)

	if t.bestIndex < 0 || better(metrics, t.entries[t.bestIndex].metrics) {
		t.bestIndex = len(t.entries) - 1
		t.stagnant = 0
	} else {
		t.stagnant++
	}
	return metrics
}

// ShouldStop applies, in order: user cancel, cleared, no improvement for two
// consecutive rounds, then the three-round cap.
func (t *RepairTracker) ShouldStop(cancelled bool) (bool, StopReason) {
	return stopFromHistory(t.History(), t.maxRounds, cancelled)
}

// stopFromHistory is the stateless form used when the history is restored from
// a persisted TripState.
func stopFromHistory(history []RoundMetrics, maxRounds int, cancelled bool) (bool, StopReason) {
	if cancelled {
		return true, StopCancelled
	}
	if len(history) == 0 {
		return false, ""
	}
	if history[len(history)-1].Blocking == 0 {
		return true, StopCleared
	}

	best := history[0]
	stagnant := 0
	for _, metrics := range history[1:] {
		if better(metrics, best) {
			best = metrics
			stagnant = 0
			continue
		}
		stagnant++
	}
	if stagnant >= 2 {
		return true, StopNoImprovement
	}
	if maxRounds <= 0 {
		maxRounds = defaultMaxRepairRounds
	}
	if len(history)-1 >= maxRounds {
		return true, StopMaxRounds
	}
	return false, ""
}

// Best returns the strongest plan seen so far together with its violations.
func (t *RepairTracker) Best() (domain.Itinerary, []domain.Violation, bool) {
	if t.bestIndex < 0 {
		return domain.Itinerary{}, nil, false
	}
	best := t.entries[t.bestIndex]
	return best.plan, append([]domain.Violation(nil), best.violations...), true
}

func (t *RepairTracker) History() []RoundMetrics {
	out := make([]RoundMetrics, len(t.entries))
	for i, entry := range t.entries {
		out[i] = entry.metrics
	}
	return out
}

func better(a, b RoundMetrics) bool {
	if a.Blocking != b.Blocking {
		return a.Blocking < b.Blocking
	}
	if a.Warnings != b.Warnings {
		return a.Warnings < b.Warnings
	}
	if a.CostYuan != b.CostYuan {
		return a.CostYuan < b.CostYuan
	}
	return a.WalkingKM+0.01 < b.WalkingKM
}

// RelaxationSuggestions derives human-readable options from what is still
// wrong. Validator suggestions win; code-based templates fill the gaps.
func RelaxationSuggestions(violations []domain.Violation, constraints domain.ConstraintSet) []string {
	seen := make(map[string]struct{}, len(violations))
	out := make([]string, 0, len(violations))
	for _, violation := range violations {
		var suggestion string
		if strings.TrimSpace(violation.Suggestion) != "" {
			suggestion = strings.TrimSpace(violation.Suggestion)
		} else {
			suggestion = relaxationTemplate(violation, constraints)
		}
		if suggestion == "" {
			continue
		}
		if _, dup := seen[suggestion]; dup {
			continue
		}
		seen[suggestion] = struct{}{}
		out = append(out, suggestion)
	}
	return out
}

func relaxationTemplate(violation domain.Violation, constraints domain.ConstraintSet) string {
	switch violation.Code {
	case "C1", "C2":
		return "可以换个日期避开闭馆日，或替换为同类别且当天开放的地点"
	case "C3":
		return "可以减少相邻地点之间的距离，或放宽每天的交通时间"
	case "C4", "C10":
		return fmt.Sprintf("可以缩短单日安排，或把每日时长放宽到 %d 小时以上", constraints.Daily.MaxHours+1)
	case "C5":
		return "可以提高总预算，或减少付费景点和长途交通"
	case "C6":
		if constraints.Meals.Enabled {
			return "可以关闭餐饮安排，或放宽午餐/晚餐时间窗"
		}
		return "可以调整餐饮时间窗"
	case "C7":
		return "必去地点无法满足时，可以移除它或更换出行日期"
	case "C8":
		return "可以去掉重复地点，把名额让给其他候选"
	case "C9", "C12":
		return "可以减少每天景点数量，或延长每个地点的停留时间"
	case "C11":
		return "可以增加室内景点比例，或选择雨天友好的方案"
	default:
		return violation.Suggestion
	}
}
