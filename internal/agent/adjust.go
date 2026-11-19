package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"trippilot/internal/domain"
)

const adjustAttempts = 2

type AdjustmentIntent struct {
	Scope        string `json:"scope"`
	AffectedDays []int  `json:"affected_days"`
	Summary      string `json:"summary,omitempty"`
}

type AdjustInput struct {
	RunID          string
	Input          string
	Constraints    domain.ConstraintSet
	BasePlan       domain.Itinerary
	BaseDraft      domain.DraftPlan
	BaseViolations []domain.Violation
	Candidates     []domain.POI
	POIs           map[string]domain.POI
	BaseVersion    int
}

type AdjustOutput struct {
	Intent       AdjustmentIntent
	Proposal     AdjustmentProposal
	Plan         domain.Itinerary
	Draft        domain.DraftPlan
	Violations   []domain.Violation
	Alternatives []domain.POI
	Usage        Usage
}

// Adjust parses the adjustment intent, retrieves replacements only for the
// affected days and returns a proposal without touching other days.
func (n *Nodes) Adjust(ctx context.Context, in AdjustInput) (AdjustOutput, error) {
	if len(in.BasePlan.Days) == 0 {
		return AdjustOutput{}, &NodeError{Code: "adjustment_base_missing", Message: "没有可调整的行程"}
	}
	if len(in.BaseDraft.Days) < len(in.BasePlan.Days) {
		return AdjustOutput{}, &NodeError{Code: "adjustment_base_missing", Message: "缺少基础草案，无法重排"}
	}

	intent, usage, err := n.parseAdjustmentIntent(ctx, in)
	if err != nil {
		return AdjustOutput{}, err
	}
	affected, err := resolveAffectedDays(intent, len(in.BasePlan.Days))
	if err != nil {
		return AdjustOutput{}, err
	}
	intent.AffectedDays = affected

	retrieved, err := n.adjustmentCandidates(ctx, in.Constraints, in.BasePlan, affected)
	if err != nil {
		return AdjustOutput{}, err
	}
	candidates := mergeCandidates(maxCandidates, in.Candidates, retrieved)
	allowed := adjustmentPOIs(in, candidates)
	alternatives := adjustmentAlternatives(in.BasePlan, candidates, affected)

	alternativesJSON, err := json.Marshal(alternatives)
	if err != nil {
		return AdjustOutput{}, &NodeError{Code: "encode_failed", Message: "编码替代点失败", Err: err}
	}
	constraintsJSON, err := json.Marshal(in.Constraints)
	if err != nil {
		return AdjustOutput{}, &NodeError{Code: "encode_failed", Message: "编码约束失败", Err: err}
	}
	planJSON, err := json.Marshal(in.BasePlan)
	if err != nil {
		return AdjustOutput{}, &NodeError{Code: "encode_failed", Message: "编码基础行程失败", Err: err}
	}

	basePrompt, err := RenderPrompt("adjust", AdjustPromptData{
		Input:        in.Input,
		Constraints:  constraintsJSON,
		BasePlan:     planJSON,
		Scope:        intent.Scope,
		AffectedDays: affected,
		Alternatives: alternativesJSON,
	})
	if err != nil {
		return AdjustOutput{}, &NodeError{Code: "prompt_failed", Message: "渲染调整提示词失败", Err: err}
	}

	prompt := basePrompt
	var issues []string
	var patch domain.DayPatch
	for attempt := 0; attempt < adjustAttempts; attempt++ {
		raw, callUsage, err := n.llm.GenerateJSON(ctx, prompt, DayPatchSchema)
		usage = addUsage(usage, callUsage)
		if err != nil {
			return AdjustOutput{}, &NodeError{Code: "adjust_llm_failed", Message: "生成调整方案失败", Err: err}
		}
		var candidate domain.DayPatch
		if err := json.Unmarshal(raw, &candidate); err != nil {
			issues = []string{"输出不是合法的 DayPatch JSON：" + err.Error()}
			prompt = retryPrompt(basePrompt, issues)
			continue
		}
		issues = validateAdjustmentPatch(candidate, affected, in.Constraints, allowed)
		if len(issues) == 0 {
			patch = candidate
			break
		}
		prompt = retryPrompt(basePrompt, issues)
	}
	if len(issues) > 0 {
		return AdjustOutput{}, &NodeError{
			Code:    "adjust_invalid",
			Message: "调整方案未通过校验：" + strings.Join(issues, "；"),
		}
	}

	merged := in.BaseDraft
	merged.Days = append([]domain.DraftDay(nil), in.BaseDraft.Days...)
	merged.Days[patch.DayIndex] = domain.DraftDay{Date: merged.Days[patch.DayIndex].Date, Items: patch.Items}
	plan, err := n.scheduler.Build(merged, in.Constraints, allowed, n.routes)
	if err != nil {
		return AdjustOutput{}, &NodeError{Code: "schedule_failed", Message: "调整后重新排期失败", Err: err}
	}
	if err := assertUntouchedDays(in.BasePlan, plan, affected); err != nil {
		return AdjustOutput{}, err
	}
	violations := n.validator.Validate(plan, in.Constraints, allowed, n.routes)

	proposal := AdjustmentProposal{
		BaseVersion: in.BaseVersion,
		Scope:       intent.Scope,
		DayIndex:    patch.DayIndex,
		Patch:       patch,
		Summary:     adjustmentSummary(intent, patch),
		Impact:      adjustmentImpact(in.BasePlan, plan, in.BaseViolations, violations),
		Warnings:    violationMessages(violations, domain.LevelWarning),
	}

	n.logger.Info("adjustment proposed",
		"run_id", in.RunID, "scope", intent.Scope, "affected", affected)

	return AdjustOutput{
		Intent:       intent,
		Proposal:     proposal,
		Plan:         plan,
		Draft:        merged,
		Violations:   violations,
		Alternatives: candidates,
		Usage:        usage,
	}, nil
}

func (n *Nodes) parseAdjustmentIntent(ctx context.Context, in AdjustInput) (AdjustmentIntent, Usage, error) {
	planJSON, err := json.Marshal(in.BasePlan)
	if err != nil {
		return AdjustmentIntent{}, Usage{}, &NodeError{Code: "encode_failed", Message: "编码基础行程失败", Err: err}
	}
	constraintsJSON, err := json.Marshal(in.Constraints)
	if err != nil {
		return AdjustmentIntent{}, Usage{}, &NodeError{Code: "encode_failed", Message: "编码约束失败", Err: err}
	}
	prompt, err := RenderPrompt("intent", AdjustIntentPromptData{
		Input:       in.Input,
		Constraints: constraintsJSON,
		BasePlan:    planJSON,
		Days:        len(in.BasePlan.Days),
	})
	if err != nil {
		return AdjustmentIntent{}, Usage{}, &NodeError{Code: "prompt_failed", Message: "渲染调整意图提示词失败", Err: err}
	}
	raw, usage, err := n.llm.GenerateJSON(ctx, prompt, AdjustmentIntentSchema)
	if err != nil {
		return AdjustmentIntent{}, Usage{}, &NodeError{Code: "adjust_llm_failed", Message: "解析调整意图失败", Err: err}
	}
	var intent AdjustmentIntent
	if err := json.Unmarshal(raw, &intent); err != nil {
		return AdjustmentIntent{}, Usage{}, &NodeError{Code: "adjust_intent_invalid", Message: "调整意图不是合法 JSON", Err: err}
	}
	return intent, usage, nil
}

func resolveAffectedDays(intent AdjustmentIntent, dayCount int) ([]int, error) {
	scope := intent.Scope
	if scope == "" || scope == "unknown" {
		return nil, &NodeError{Code: "adjustment_needs_input", Message: "请说明要调整哪一天或哪部分行程"}
	}

	days := make([]int, 0, len(intent.AffectedDays))
	seen := make(map[int]struct{}, len(intent.AffectedDays))
	for _, day := range intent.AffectedDays {
		if day < 0 || day >= dayCount {
			return nil, &NodeError{
				Code:    "adjustment_needs_input",
				Message: fmt.Sprintf("调整的天序号 %d 超出 1-%d 天范围", day+1, dayCount),
			}
		}
		if _, dup := seen[day]; dup {
			continue
		}
		seen[day] = struct{}{}
		days = append(days, day)
	}
	if len(days) == 0 {
		switch scope {
		case "pace", "budget":
			for day := 0; day < dayCount; day++ {
				days = append(days, day)
			}
		default:
			return nil, &NodeError{Code: "adjustment_needs_input", Message: "请说明要调整哪一天（0-based）"}
		}
	}
	sort.Ints(days)
	return days, nil
}

// adjustmentCandidates queries the snapshot and external source only for the
// affected days, using their POI names and categories as keywords.
func (n *Nodes) adjustmentCandidates(ctx context.Context, constraints domain.ConstraintSet, plan domain.Itinerary, days []int) ([]domain.POI, error) {
	if n.snapshot == nil {
		return nil, &NodeError{Code: "source_unavailable", Message: "候选数据源未配置"}
	}
	groups := make([][]domain.POI, 0, len(days)*2)
	for _, day := range days {
		query := domain.POIQuery{
			City:     constraints.City,
			Keywords: dayKeywords(plan, day),
		}
		pois, err := n.snapshot.SearchCandidates(ctx, query)
		if err != nil {
			return nil, &NodeError{Code: "retrieve_failed", Message: "受影响天的候选检索失败", Err: err}
		}
		groups = append(groups, pois)
		if n.external == nil {
			continue
		}
		external, err := n.external.SearchCandidates(ctx, query)
		if err != nil {
			n.logger.Warn("external adjustment search failed", "day", day, "error", err)
			continue
		}
		groups = append(groups, external)
	}
	return mergeCandidates(maxCandidates, groups...), nil
}

func dayKeywords(plan domain.Itinerary, day int) []string {
	if day < 0 || day >= len(plan.Days) {
		return nil
	}
	keywords := make([]string, 0, 4)
	seen := make(map[string]struct{})
	for _, block := range plan.Days[day].Blocks {
		for _, candidate := range []string{block.Title, block.POIID} {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			if _, dup := seen[candidate]; dup {
				continue
			}
			seen[candidate] = struct{}{}
			keywords = append(keywords, candidate)
			if len(keywords) == 4 {
				return keywords
			}
		}
	}
	return keywords
}

func adjustmentAlternatives(plan domain.Itinerary, candidates []domain.POI, days []int) []promptAlternative {
	used := make(map[string]struct{})
	for _, day := range plan.Days {
		for _, block := range day.Blocks {
			if block.POIID != "" {
				used[block.POIID] = struct{}{}
			}
		}
	}
	date := ""
	if len(days) > 0 && days[0] < len(plan.Days) {
		date = plan.Days[days[0]].Date
	}
	out := make([]promptAlternative, 0, maxAlternatives)
	for _, poi := range candidates {
		if _, exists := used[poi.ID]; exists {
			continue
		}
		out = append(out, promptAlternative{
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
		if len(out) == maxAlternatives {
			break
		}
	}
	return out
}

func adjustmentPOIs(in AdjustInput, candidates []domain.POI) map[string]domain.POI {
	pois := make(map[string]domain.POI, len(in.POIs)+len(candidates))
	for id, poi := range in.POIs {
		pois[id] = poi
	}
	for _, poi := range candidates {
		if _, dup := pois[poi.ID]; dup {
			continue
		}
		pois[poi.ID] = poi
	}
	for _, day := range in.BasePlan.Days {
		for _, block := range day.Blocks {
			if block.POIID == "" {
				continue
			}
			if _, exists := pois[block.POIID]; exists {
				continue
			}
			pois[block.POIID] = domain.POI{ID: block.POIID, Name: block.Title, City: in.Constraints.City}
		}
	}
	return pois
}

func validateAdjustmentPatch(patch domain.DayPatch, affected []int, constraints domain.ConstraintSet, pois map[string]domain.POI) []string {
	issues := make([]string, 0)
	if !containsInt(affected, patch.DayIndex) {
		issues = append(issues, fmt.Sprintf("day_index 必须是受影响的天之一 %v，实际 %d", affected, patch.DayIndex))
	}
	if strings.TrimSpace(patch.Reason) == "" {
		issues = append(issues, "reason 不能为空")
	}
	issues = append(issues, validateDayItems(patch.DayIndex, patch.Items, constraints.Meals.Enabled, pois)...)
	return issues
}

func assertUntouchedDays(base, updated domain.Itinerary, affected []int) error {
	touched := make(map[int]struct{}, len(affected))
	for _, day := range affected {
		touched[day] = struct{}{}
	}
	for day := 0; day < len(base.Days); day++ {
		if _, ok := touched[day]; ok {
			continue
		}
		if day >= len(updated.Days) || !reflect.DeepEqual(base.Days[day], updated.Days[day]) {
			return &NodeError{
				Code:    "adjustment_scope_violation",
				Message: fmt.Sprintf("未受影响的天 %d 被改动", day+1),
			}
		}
	}
	return nil
}

func adjustmentSummary(intent AdjustmentIntent, patch domain.DayPatch) string {
	if strings.TrimSpace(intent.Summary) != "" {
		return strings.TrimSpace(intent.Summary)
	}
	return strings.TrimSpace(patch.Reason)
}

func adjustmentImpact(before, after domain.Itinerary, beforeViolations, afterViolations []domain.Violation) string {
	beforeCounts := domain.CountByLevel(beforeViolations)
	afterCounts := domain.CountByLevel(afterViolations)
	return fmt.Sprintf("阻断 %d→%d，警告 %d→%d，费用 %d→%d 元，步行 %.1f→%.1f 公里",
		beforeCounts[domain.LevelBlocking], afterCounts[domain.LevelBlocking],
		beforeCounts[domain.LevelWarning], afterCounts[domain.LevelWarning],
		before.Totals.CostYuan, after.Totals.CostYuan,
		before.Totals.WalkingKM, after.Totals.WalkingKM,
	)
}

func violationMessages(violations []domain.Violation, level domain.Level) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, violation := range violations {
		if violation.Level != level || violation.Message == "" {
			continue
		}
		if _, dup := seen[violation.Message]; dup {
			continue
		}
		seen[violation.Message] = struct{}{}
		out = append(out, violation.Message)
		if len(out) == 5 {
			break
		}
	}
	return out
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
