package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"trippilot/internal/domain"
	"trippilot/internal/domain/diff"
)

const StatusAwaitingUser = "awaiting_user"

// VersionSnapshot is the immutable record behind one plan version.
type VersionSnapshot struct {
	Seq          int                   `json:"seq"`
	Plan         domain.Itinerary      `json:"plan"`
	Constraints  domain.ConstraintSet  `json:"constraints"`
	Violations   []domain.Violation    `json:"violations"`
	Reason       string                `json:"reason,omitempty"`
	Explanations []domain.Explanation  `json:"explanations"`
	Diff         diff.Result           `json:"diff"`
	Suggestions  []string              `json:"suggestions,omitempty"`
	Research     *domain.ResearchBrief `json:"research,omitempty"`
	CreatedAt    time.Time             `json:"created_at"`
}

type PresentInput struct {
	RunID        string
	Constraints  domain.ConstraintSet
	Plan         domain.Itinerary
	Violations   []domain.Violation
	Reason       string
	Explanations []domain.Explanation
	Suggestions  []string
	Previous     *domain.Itinerary
	PreviousSeq  int
	Research     *domain.ResearchBrief
}

type PresentOutput struct {
	Snapshot VersionSnapshot
	Status   string
}

// Present snapshots the accepted plan, diffs it against the previous version
// and fills any missing explanation with a deterministic template.
func (n *Nodes) Present(ctx context.Context, in PresentInput) (PresentOutput, error) {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "initial"
		if in.Previous != nil {
			reason = "updated"
		}
	}

	before := domain.Itinerary{}
	if in.Previous != nil {
		before = *in.Previous
	}

	snapshot := VersionSnapshot{
		Seq:          in.PreviousSeq + 1,
		Plan:         in.Plan,
		Constraints:  in.Constraints,
		Violations:   append([]domain.Violation(nil), in.Violations...),
		Reason:       reason,
		Explanations: mergeExplanations(in.Plan, in.Explanations),
		Diff:         diff.Diff(before, in.Plan),
		Suggestions:  append([]string(nil), in.Suggestions...),
		Research:     cloneResearchBrief(in.Research),
		CreatedAt:    n.now().UTC(),
	}

	payload := map[string]any{
		"seq":          snapshot.Seq,
		"plan":         snapshot.Plan,
		"violations":   snapshot.Violations,
		"reason":       snapshot.Reason,
		"explanations": snapshot.Explanations,
		"diff":         snapshot.Diff,
		"suggestions":  snapshot.Suggestions,
	}
	if in.Research != nil {
		payload["research_status"] = in.Research.Status
		payload["lodging_summary"] = buildLodgingEventSummary(in.Research, in.Plan)
		payload["guide_summary"] = buildGuideEventSummary(in.Research)
	}

	err := n.events.Publish(ctx, in.RunID, "plan.updated", payload)
	if err != nil {
		return PresentOutput{}, &NodeError{Code: "event_failed", Message: "publish plan.updated", Err: err}
	}

	return PresentOutput{Snapshot: snapshot, Status: StatusAwaitingUser}, nil
}

// mergeExplanations keeps the LLM's text for surviving POIs and falls back to
// a template when a POI has none, so the frontend always has something to show.
func mergeExplanations(plan domain.Itinerary, provided []domain.Explanation) []domain.Explanation {
	texts := make(map[string]string, len(provided))
	for _, explanation := range provided {
		id := strings.TrimSpace(explanation.POIID)
		text := strings.TrimSpace(explanation.Text)
		if id == "" || text == "" {
			continue
		}
		if _, exists := texts[id]; exists {
			continue
		}
		texts[id] = text
	}

	out := make([]domain.Explanation, 0)
	seen := make(map[string]struct{})
	for dayIndex, day := range plan.Days {
		for _, block := range day.Blocks {
			if block.Type != domain.BlockTypePOI || block.POIID == "" {
				continue
			}
			if _, exists := seen[block.POIID]; exists {
				continue
			}
			seen[block.POIID] = struct{}{}
			text, ok := texts[block.POIID]
			if !ok {
				text = explanationFallback(dayIndex+1, block)
			}
			out = append(out, domain.Explanation{POIID: block.POIID, Text: text})
		}
	}
	return out
}

func explanationFallback(dayNumber int, block domain.Block) string {
	title := strings.TrimSpace(block.Title)
	if title == "" {
		title = "该地点"
	}
	text := fmt.Sprintf("第 %d 天游览%s，建议停留 %d 分钟", dayNumber, title, block.DurationMin)
	if block.CostYuan > 0 {
		text += fmt.Sprintf("，门票约 %d 元", block.CostYuan)
	}
	return text + "。"
}

func cloneResearchBrief(brief *domain.ResearchBrief) *domain.ResearchBrief {
	if brief == nil {
		return nil
	}
	out := *brief
	out.Notes = append([]string(nil), brief.Notes...)
	out.Warnings = append([]string(nil), brief.Warnings...)
	out.Guides = append([]domain.GuideSignal(nil), brief.Guides...)
	for i := range out.Guides {
		out.Guides[i].Citations = append([]domain.GuideCitation(nil), brief.Guides[i].Citations...)
	}
	if brief.Accommodation != nil {
		accommodation := *brief.Accommodation
		accommodation.Specified = append([]string(nil), brief.Accommodation.Specified...)
		accommodation.Options = append([]domain.Hotel(nil), brief.Accommodation.Options...)
		accommodation.AnchorDays = append([]int(nil), brief.Accommodation.AnchorDays...)
		if brief.Accommodation.Selected != nil {
			hotel := *brief.Accommodation.Selected
			accommodation.Selected = &hotel
		}
		out.Accommodation = &accommodation
	}
	return &out
}

type lodgingEventSummary struct {
	HotelName       string `json:"hotel_name,omitempty"`
	AnchorDays      []int  `json:"anchor_days,omitempty"`
	Nights          int    `json:"nights"`
	TotalYuan       int    `json:"total_yuan"`
	IncludeInBudget bool   `json:"include_in_budget"`
	PriceEstimated  bool   `json:"price_estimated"`
}

type guideEventSummary struct {
	Matches int      `json:"matches"`
	POIIDs  []string `json:"poi_ids,omitempty"`
}

func buildLodgingEventSummary(research *domain.ResearchBrief, plan domain.Itinerary) *lodgingEventSummary {
	if research == nil || research.Accommodation == nil || research.Accommodation.Selected == nil {
		return nil
	}
	accommodation := research.Accommodation
	summary := &lodgingEventSummary{
		HotelName:       accommodation.Selected.Name,
		AnchorDays:      append([]int(nil), accommodation.AnchorDays...),
		Nights:          accommodation.Nights,
		TotalYuan:       accommodation.TotalYuan,
		IncludeInBudget: accommodation.IncludeInBudget,
		PriceEstimated:  accommodation.PriceEstimated,
	}
	if plan.Lodging != nil {
		summary.Nights = plan.Lodging.Nights
		summary.TotalYuan = plan.Lodging.TotalYuan
		summary.PriceEstimated = plan.Lodging.PriceEstimated
	}
	return summary
}

func buildGuideEventSummary(research *domain.ResearchBrief) *guideEventSummary {
	if research == nil || len(research.Guides) == 0 {
		return nil
	}
	summary := &guideEventSummary{Matches: len(research.Guides)}
	for _, signal := range research.Guides {
		if signal.POIID == "" {
			continue
		}
		summary.POIIDs = append(summary.POIIDs, signal.POIID)
		if len(summary.POIIDs) == 5 {
			break
		}
	}
	return summary
}
