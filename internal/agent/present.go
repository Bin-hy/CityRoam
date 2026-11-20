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
		CreatedAt:    n.now().UTC(),
	}

	err := n.events.Publish(ctx, in.RunID, "plan.updated", map[string]any{
		"seq":          snapshot.Seq,
		"plan":         snapshot.Plan,
		"violations":   snapshot.Violations,
		"reason":       snapshot.Reason,
		"explanations": snapshot.Explanations,
		"diff":         snapshot.Diff,
		"suggestions":  snapshot.Suggestions,
	})
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
