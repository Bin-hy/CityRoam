package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"trippilot/internal/domain"
)

const (
	repairAttempts  = 2
	maxAlternatives = 15
	fitAttempts     = 3
)

type RepairInput struct {
	RunID       string
	Constraints domain.ConstraintSet
	Draft       domain.DraftPlan
	Plan        domain.Itinerary
	POIs        map[string]domain.POI
	Candidates  []domain.POI
	Violations  []domain.Violation
	Round       int
}

type RepairOutput struct {
	Patch        domain.DayPatch
	Draft        domain.DraftPlan
	Plan         domain.Itinerary
	Violations   []domain.Violation
	Alternatives int
	Usage        Usage
}

// promptAlternative is one replacement option with its open status on the
// target date so the LLM does not pick a closed POI.
type promptAlternative struct {
	ID               string   `json:"poi_id"`
	Name             string   `json:"name"`
	Category         string   `json:"category"`
	Area             string   `json:"area,omitempty"`
	SuggestedMinutes int      `json:"suggested_minutes"`
	TicketYuan       *int     `json:"ticket_yuan,omitempty"`
	Outdoor          bool     `json:"outdoor"`
	OpenStatus       string   `json:"open_status"`
	Tags             []string `json:"tags,omitempty"`
}

// Repair patches one affected day, reschedules and reruns the full validation.
// Untouched days keep their exact items.
func (n *Nodes) Repair(ctx context.Context, in RepairInput) (RepairOutput, error) {
	if len(in.Violations) == 0 {
		return RepairOutput{}, &NodeError{Code: "nothing_to_repair", Message: "没有需要修复的违规"}
	}
	target, ok := targetDay(in.Violations, len(in.Draft.Days))
	if !ok {
		return RepairOutput{}, &NodeError{Code: "nothing_to_repair", Message: "违规无法定位到具体某一天"}
	}

	if err := n.events.Publish(ctx, in.RunID, "repair.started", map[string]any{
		"round":     in.Round,
		"day_index": target,
		"blocking":  domain.CountByLevel(in.Violations)[domain.LevelBlocking],
	}); err != nil {
		return RepairOutput{}, &NodeError{Code: "event_failed", Message: "发布修复开始事件失败", Err: err}
	}

	relaxCategory := hasClosedViolation(in.Violations)
	alternatives := buildAlternatives(in.Plan, target, in.POIs, in.Candidates, relaxCategory)
	alternativesJSON, err := json.Marshal(alternatives)
	if err != nil {
		return RepairOutput{}, &NodeError{Code: "encode_failed", Message: "编码替代点失败", Err: err}
	}
	constraintsJSON, err := json.Marshal(in.Constraints)
	if err != nil {
		return RepairOutput{}, &NodeError{Code: "encode_failed", Message: "编码约束失败", Err: err}
	}
	dayJSON := json.RawMessage(`{}`)
	if target < len(in.Plan.Days) {
		if encoded, err := json.Marshal(in.Plan.Days[target]); err == nil {
			dayJSON = encoded
		}
	}
	violationsJSON, err := json.Marshal(violationsForDay(in.Violations, target))
	if err != nil {
		return RepairOutput{}, &NodeError{Code: "encode_failed", Message: "编码违规失败", Err: err}
	}

	basePrompt, err := RenderPrompt("repair", RepairPromptData{
		Constraints:  constraintsJSON,
		DayIndex:     target,
		CurrentDay:   dayJSON,
		Violations:   violationsJSON,
		Alternatives: alternativesJSON,
	})
	if err != nil {
		return RepairOutput{}, &NodeError{Code: "prompt_failed", Message: "渲染修复提示词失败", Err: err}
	}

	prompt := basePrompt
	var total Usage
	var issues []string
	var patch domain.DayPatch
	for attempt := 0; attempt < repairAttempts; attempt++ {
		raw, usage, err := n.llm.GenerateJSON(ctx, prompt, DayPatchSchema)
		total = addUsage(total, usage)
		if err != nil {
			return RepairOutput{}, &NodeError{Code: "repair_llm_failed", Message: "生成修复方案失败", Err: err}
		}

		var candidate domain.DayPatch
		if err := json.Unmarshal(raw, &candidate); err != nil {
			issues = []string{"输出不是合法的 DayPatch JSON：" + err.Error()}
			prompt = retryPrompt(basePrompt, issues)
			continue
		}
		issues = validatePatch(candidate, target, in.Constraints, in.POIs)
		if len(issues) == 0 {
			patch = candidate
			break
		}
		prompt = retryPrompt(basePrompt, issues)
	}
	if len(issues) > 0 {
		return RepairOutput{}, &NodeError{
			Code:    "repair_invalid",
			Message: "修复方案未通过校验：" + strings.Join(issues, "；"),
		}
	}

	merged := in.Draft
	merged.Days[target] = domain.DraftDay{Date: merged.Days[target].Date, Items: patch.Items}
	plan, err := n.scheduler.Build(merged, in.Constraints, in.POIs, n.routes)
	if err != nil {
		return RepairOutput{}, &NodeError{Code: "schedule_failed", Message: "修复后重新排期失败", Err: err}
	}
	violations := n.validator.Validate(plan, in.Constraints, in.POIs, n.routes)
	merged, plan, violations = n.fitDailyHours(merged, plan, violations, in.Constraints, in.POIs)
	counts := domain.CountByLevel(violations)

	if err := n.events.Publish(ctx, in.RunID, "repair.completed", map[string]any{
		"round":     in.Round,
		"day_index": target,
		"patch":     patch,
		"blocking":  counts[domain.LevelBlocking],
		"warnings":  counts[domain.LevelWarning],
	}); err != nil {
		return RepairOutput{}, &NodeError{Code: "event_failed", Message: "发布修复完成事件失败", Err: err}
	}

	n.logger.Info("repair applied",
		"run_id", in.RunID, "round", in.Round, "day_index", target,
		"blocking", counts[domain.LevelBlocking], "warnings", counts[domain.LevelWarning])

	return RepairOutput{
		Patch:        patch,
		Draft:        merged,
		Plan:         plan,
		Violations:   violations,
		Alternatives: len(alternatives),
		Usage:        total,
	}, nil
}

func targetDay(violations []domain.Violation, dayCount int) (int, bool) {
	if dayCount == 0 {
		return 0, false
	}
	// Repair the day with the most blocking violations first so a late day
	// does not starve while an earlier day keeps consuming repair rounds.
	blocking := make([]int, dayCount)
	for _, violation := range violations {
		if violation.Level != domain.LevelBlocking {
			continue
		}
		if violation.Day >= 1 && violation.Day <= dayCount {
			blocking[violation.Day-1]++
		}
	}
	best := -1
	for day, count := range blocking {
		if count > 0 && (best == -1 || count > blocking[best]) {
			best = day
		}
	}
	if best >= 0 {
		return best, true
	}

	for _, violation := range violations {
		if violation.Level != domain.LevelWarning {
			continue
		}
		if violation.Day >= 1 && violation.Day <= dayCount {
			return violation.Day - 1, true
		}
	}
	for _, violation := range violations {
		if violation.Day == 0 {
			return 0, true
		}
	}
	return 0, false
}

func violationsForDay(violations []domain.Violation, dayIndex int) []domain.Violation {
	filtered := make([]domain.Violation, 0, len(violations))
	for _, violation := range violations {
		if violation.Day == 0 || violation.Day == dayIndex+1 {
			filtered = append(filtered, violation)
		}
	}
	return filtered
}

// fitDailyHours shortens stays on days that exceed the daily hour limit so a
// single repair round is not consumed by arithmetic that the scheduler can
// settle deterministically.
func (n *Nodes) fitDailyHours(
	draft domain.DraftPlan,
	plan domain.Itinerary,
	violations []domain.Violation,
	constraints domain.ConstraintSet,
	pois map[string]domain.POI,
) (domain.DraftPlan, domain.Itinerary, []domain.Violation) {
	maxMinutes := constraints.ApplyDefaults().Daily.MaxHours * 60
	if maxMinutes <= 0 {
		return draft, plan, violations
	}

	for attempt := 0; attempt < fitAttempts; attempt++ {
		trimmed := false
		for _, violation := range violations {
			if violation.Code != "C4" || violation.Level != domain.LevelBlocking {
				continue
			}
			dayIndex := violation.Day - 1
			if dayIndex < 0 || dayIndex >= len(plan.Days) || dayIndex >= len(draft.Days) {
				continue
			}
			start, startErr := domain.ParseClock(plan.Days[dayIndex].StartTime)
			end, endErr := domain.ParseClock(plan.Days[dayIndex].EndTime)
			if startErr != nil || endErr != nil {
				continue
			}
			over := end - start
			if over < 0 {
				over += domain.MinutesPerDay
			}
			over -= maxMinutes
			items := draft.Days[dayIndex].Items
			for itemIndex := len(items) - 1; itemIndex >= 0 && over > 0; itemIndex-- {
				item := &items[itemIndex]
				if item.DurationMin <= minItemMinutes {
					continue
				}
				cut := min(item.DurationMin-minItemMinutes, over)
				item.DurationMin -= cut
				over -= cut
				trimmed = true
			}
		}
		if !trimmed {
			break
		}
		built, err := n.scheduler.Build(draft, constraints, pois, n.routes)
		if err != nil {
			break
		}
		plan = built
		violations = n.validator.Validate(plan, constraints, pois, n.routes)
		if !hasBlockingCode(violations, "C4") {
			break
		}
	}
	return draft, plan, violations
}

func hasBlockingCode(violations []domain.Violation, code string) bool {
	for _, violation := range violations {
		if violation.Code == code && violation.Level == domain.LevelBlocking {
			return true
		}
	}
	return false
}

func buildAlternatives(plan domain.Itinerary, dayIndex int, pois map[string]domain.POI, candidates []domain.POI, relaxCategory bool) []promptAlternative {
	used := make(map[string]struct{})
	for _, day := range plan.Days {
		for _, block := range day.Blocks {
			if block.POIID != "" {
				used[block.POIID] = struct{}{}
			}
		}
	}

	categories := make(map[string]struct{})
	for _, block := range planDayBlocks(plan, dayIndex) {
		if poi, found := pois[block.POIID]; found && poi.Category != "" {
			categories[poi.Category] = struct{}{}
		}
	}

	date := ""
	if dayIndex < len(plan.Days) {
		date = plan.Days[dayIndex].Date
	}

	alternatives := make([]promptAlternative, 0, maxAlternatives)
	for _, poi := range candidates {
		if _, alreadyUsed := used[poi.ID]; alreadyUsed {
			continue
		}
		if !relaxCategory && len(categories) > 0 {
			if _, sameCategory := categories[poi.Category]; !sameCategory {
				continue
			}
		}
		alternatives = append(alternatives, promptAlternative{
			ID:               poi.ID,
			Name:             poi.Name,
			Category:         poi.Category,
			Area:             poi.Area,
			SuggestedMinutes: poi.SuggestedMinutes,
			TicketYuan:       poi.TicketYuan,
			Outdoor:          poi.Outdoor,
			OpenStatus:       openOnDate(poi, date),
			Tags:             poi.Tags,
		})
	}

	sort.SliceStable(alternatives, func(i, j int) bool {
		return statusRank(alternatives[i].OpenStatus) < statusRank(alternatives[j].OpenStatus)
	})
	if len(alternatives) > maxAlternatives {
		alternatives = alternatives[:maxAlternatives]
	}
	return alternatives
}

func planDayBlocks(plan domain.Itinerary, dayIndex int) []domain.Block {
	if dayIndex < 0 || dayIndex >= len(plan.Days) {
		return nil
	}
	return plan.Days[dayIndex].Blocks
}

// hasClosedViolation reports whether the day contains a blocking closure, in
// which case relevance filters must yield to any open replacement.
func hasClosedViolation(violations []domain.Violation) bool {
	for _, violation := range violations {
		if violation.Level == domain.LevelBlocking && violation.Code == "C2" {
			return true
		}
	}
	return false
}

func openOnDate(poi domain.POI, date string) string {
	if len(poi.OpenHours) == 0 {
		return "unknown"
	}
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "unknown"
	}
	weekday := int(parsed.Weekday())
	hasValidRange := false
	for _, openRange := range poi.OpenHours {
		start, startOK := parseClock(openRange.Start)
		end, endOK := parseClock(openRange.End)
		if !startOK || !endOK || end <= start {
			continue
		}
		hasValidRange = true
		for _, day := range openRange.Days {
			if day == weekday {
				return "open " + openRange.Start + "-" + openRange.End
			}
		}
	}
	if hasValidRange {
		return "closed"
	}
	return "unknown"
}

func parseClock(value string) (int, bool) {
	minutes, err := domain.ParseClock(value)
	return minutes, err == nil
}

func statusRank(status string) int {
	switch {
	case strings.HasPrefix(status, "open"):
		return 0
	case strings.HasPrefix(status, "unknown"):
		return 1
	default:
		return 2
	}
}

func validatePatch(patch domain.DayPatch, target int, constraints domain.ConstraintSet, pois map[string]domain.POI) []string {
	issues := make([]string, 0)
	if patch.DayIndex != target {
		issues = append(issues, fmt.Sprintf("day_index 必须是 %d，实际 %d", target, patch.DayIndex))
	}
	if strings.TrimSpace(patch.Reason) == "" {
		issues = append(issues, "reason 不能为空")
	}
	issues = append(issues, validateDayItems(target, patch.Items, constraints.Meals.Enabled, pois)...)
	return issues
}
