package agent

import (
	"encoding/json"

	"trippilot/internal/domain"
	"trippilot/internal/skills"
)

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// RunError is the JSON-safe form of a node failure stored in run state.
type RunError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AdjustmentProposal is the pending change an adjustment run asks the user
// to accept or reject before a new version is written.
type AdjustmentProposal struct {
	BaseVersion int             `json:"base_version"`
	Scope       string          `json:"scope"`
	DayIndex    int             `json:"day_index"`
	Patch       domain.DayPatch `json:"patch"`
	Summary     string          `json:"summary"`
	Impact      string          `json:"impact"`
	Warnings    []string        `json:"warnings,omitempty"`
}

// TripState is the serializable payload that flows through the Eino graph and
// is persisted after every stage so an awaiting run can resume from the DB.
type TripState struct {
	TripID           string                   `json:"trip_id,omitempty"`
	RunID            string                   `json:"run_id"`
	Mode             string                   `json:"mode,omitempty"`
	Input            string                   `json:"input"`
	Constraints      domain.ConstraintSet     `json:"constraints"`
	Missing          []string                 `json:"missing,omitempty"`
	Candidates       []domain.POI             `json:"candidates,omitempty"`
	Draft            domain.DraftPlan         `json:"draft"`
	Plan             domain.Itinerary         `json:"plan"`
	Violations       []domain.Violation       `json:"violations,omitempty"`
	Round            int                      `json:"round"`
	Best             *domain.Itinerary        `json:"best,omitempty"`
	BestViolations   []domain.Violation       `json:"best_violations,omitempty"`
	History          []RoundMetrics           `json:"history,omitempty"`
	StopReason       string                   `json:"stop_reason,omitempty"`
	Suggestions      []string                 `json:"suggestions,omitempty"`
	LLM              Usage                    `json:"llm,omitzero"`
	Snapshot         *VersionSnapshot         `json:"snapshot,omitempty"`
	Research         *domain.ResearchBrief    `json:"research,omitempty"`
	ResearchQuestion *domain.ResearchQuestion `json:"research_question,omitempty"`
	Skills           []skills.Skill           `json:"skills,omitempty"`
	SkillPins        []skills.CatalogEntry    `json:"skill_pins,omitempty"`
	SkillsPinned     bool                     `json:"skills_pinned,omitempty"`
	ResolvedHotel    *domain.Hotel            `json:"resolved_hotel,omitempty"`
	ConstraintPatch  *domain.ConstraintSet    `json:"constraint_patch,omitempty"`
	Proposal         *AdjustmentProposal      `json:"proposal,omitempty"`
	Previous         *domain.Itinerary        `json:"previous,omitempty"`
	PreviousSeq      int                      `json:"previous_seq,omitempty"`
	Stage            string                   `json:"stage,omitempty"`
	Status           string                   `json:"status"`
	Error            *RunError                `json:"error,omitempty"`
}

func (s *TripState) Encode() (json.RawMessage, error) {
	return json.Marshal(s)
}

func DecodeTripState(data []byte) (*TripState, error) {
	var state TripState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}
