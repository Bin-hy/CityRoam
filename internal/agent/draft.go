package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"trippilot/internal/domain"
)

const (
	draftAttempts  = 2
	minItemMinutes = 15
	maxItemMinutes = 480
)

type DraftInput struct {
	RunID       string
	Constraints domain.ConstraintSet
	Candidates  []domain.POI
}

type DraftOutput struct {
	Draft domain.DraftPlan
	POIs  map[string]domain.POI
	Usage Usage
}

// promptCandidate is the compact candidate shape shown to the LLM.
type promptCandidate struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Category         string   `json:"category"`
	Area             string   `json:"area,omitempty"`
	Outdoor          bool     `json:"outdoor"`
	SuggestedMinutes int      `json:"suggested_minutes"`
	TicketYuan       *int     `json:"ticket_yuan,omitempty"`
	Tags             []string `json:"tags,omitempty"`
}

// Draft asks the LLM to pick only candidate IDs. Unknown IDs or missing meals
// trigger one corrective retry, then fail so the closed-set guarantee holds.
func (n *Nodes) Draft(ctx context.Context, in DraftInput) (DraftOutput, error) {
	if len(in.Candidates) == 0 {
		return DraftOutput{}, &NodeError{Code: "no_candidates", Message: "候选集为空"}
	}

	pois := make(map[string]domain.POI, len(in.Candidates))
	compact := make([]promptCandidate, 0, len(in.Candidates))
	for _, poi := range in.Candidates {
		if _, dup := pois[poi.ID]; dup {
			continue
		}
		pois[poi.ID] = poi
		compact = append(compact, promptCandidate{
			ID:               poi.ID,
			Name:             poi.Name,
			Category:         poi.Category,
			Area:             poi.Area,
			Outdoor:          poi.Outdoor,
			SuggestedMinutes: poi.SuggestedMinutes,
			TicketYuan:       poi.TicketYuan,
			Tags:             poi.Tags,
		})
	}

	constraintsJSON, err := json.Marshal(in.Constraints)
	if err != nil {
		return DraftOutput{}, &NodeError{Code: "encode_failed", Message: "编码约束失败", Err: err}
	}
	candidatesJSON, err := json.Marshal(compact)
	if err != nil {
		return DraftOutput{}, &NodeError{Code: "encode_failed", Message: "编码候选失败", Err: err}
	}

	basePrompt, err := RenderPrompt("draft", DraftPromptData{
		Constraints: constraintsJSON,
		Candidates:  candidatesJSON,
		Days:        in.Constraints.Days,
	})
	if err != nil {
		return DraftOutput{}, &NodeError{Code: "prompt_failed", Message: "渲染选点提示词失败", Err: err}
	}
	basePrompt = withDateInstruction(basePrompt, in.Constraints, n.tripStart(in.Constraints))

	prompt := basePrompt
	var total Usage
	var issues []string
	for attempt := 0; attempt < draftAttempts; attempt++ {
		raw, usage, err := n.llm.GenerateJSON(ctx, prompt, DraftPlanSchema)
		total = addUsage(total, usage)
		if err != nil {
			return DraftOutput{}, &NodeError{Code: "draft_llm_failed", Message: "生成行程草案失败", Err: err}
		}

		var draft domain.DraftPlan
		if err := json.Unmarshal(raw, &draft); err != nil {
			issues = []string{"输出不是合法的 DraftPlan JSON：" + err.Error()}
			prompt = retryPrompt(basePrompt, issues)
			continue
		}
		assignDates(&draft, n.tripStart(in.Constraints))
		issues = validateDraft(draft, in.Constraints, pois)
		if len(issues) == 0 {
			return DraftOutput{Draft: draft, POIs: pois, Usage: total}, nil
		}
		prompt = retryPrompt(basePrompt, issues)
	}

	return DraftOutput{}, &NodeError{
		Code:    "draft_invalid",
		Message: "行程草案未通过闭集校验：" + strings.Join(issues, "；"),
	}
}

func validateDraft(draft domain.DraftPlan, constraints domain.ConstraintSet, pois map[string]domain.POI) []string {
	issues := make([]string, 0)
	if len(draft.Days) != constraints.Days {
		issues = append(issues, fmt.Sprintf("需要恰好 %d 天，实际 %d 天", constraints.Days, len(draft.Days)))
	}
	for dayIndex, day := range draft.Days {
		issues = append(issues, validateDayItems(dayIndex, day.Items, constraints.Meals.Enabled, pois)...)
	}
	return issues
}

func validateDayItems(dayIndex int, items []domain.DraftItem, mealsEnabled bool, pois map[string]domain.POI) []string {
	issues := make([]string, 0)
	if len(items) == 0 {
		return append(issues, fmt.Sprintf("第 %d 天没有任何安排", dayIndex+1))
	}
	meals := 0
	for _, item := range items {
		if _, found := pois[item.POIID]; !found {
			issues = append(issues, fmt.Sprintf("第 %d 天使用了允许范围之外的 poi_id %q", dayIndex+1, item.POIID))
		}
		switch item.Type {
		case domain.ItemTypePOI:
		case domain.ItemTypeMeal:
			meals++
		default:
			issues = append(issues, fmt.Sprintf("第 %d 天的 %q type 必须是 poi 或 meal", dayIndex+1, item.POIID))
		}
		if item.DurationMin < minItemMinutes || item.DurationMin > maxItemMinutes {
			issues = append(issues, fmt.Sprintf("第 %d 天的 %q duration_min 必须在 %d-%d 之间",
				dayIndex+1, item.POIID, minItemMinutes, maxItemMinutes))
		}
	}
	if mealsEnabled && meals == 0 {
		issues = append(issues, fmt.Sprintf("第 %d 天缺少餐饮安排", dayIndex+1))
	}
	return issues
}

func assignDates(draft *domain.DraftPlan, start time.Time) {
	for i := range draft.Days {
		draft.Days[i].Date = start.AddDate(0, 0, i).Format("2006-01-02")
	}
}

func (n *Nodes) tripStart(constraints domain.ConstraintSet) time.Time {
	if parsed, err := time.Parse("2006-01-02", constraints.StartDate); err == nil {
		return parsed
	}
	return n.now().AddDate(0, 0, 1)
}

func withDateInstruction(prompt string, constraints domain.ConstraintSet, start time.Time) string {
	if constraints.StartDate != "" {
		return prompt
	}
	return prompt + "\n\n行程从 " + start.Format("2006-01-02") + " 开始，按天依次排布。"
}

func retryPrompt(base string, issues []string) string {
	return base + "\n\n上次输出存在以下问题：\n- " + strings.Join(issues, "\n- ") +
		"\n请修正这些问题后重新输出完整的 JSON。"
}

func addUsage(total, add Usage) Usage {
	total.PromptTokens += add.PromptTokens
	total.CompletionTokens += add.CompletionTokens
	total.TotalTokens += add.TotalTokens
	total.Attempts += add.Attempts
	total.Duration += add.Duration
	if add.Model != "" {
		total.Model = add.Model
	}
	return total
}
