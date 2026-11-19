package agent

import (
	"encoding/json"

	"trippilot/internal/domain"
)

const (
	StatusRunning  = "running"
	StatusComplete = "completed"
)

// RunError is the JSON-safe form of a node failure stored in run state.
type RunError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// TripState is the serializable payload that flows through the Eino graph and
// is persisted after every stage so an awaiting run can resume from the DB.
type TripState struct {
	TripID         string               `json:"trip_id,omitempty"`
	RunID          string               `json:"run_id"`
	Mode           string               `json:"mode,omitempty"`
	Input          string               `json:"input"`
	Constraints    domain.ConstraintSet `json:"constraints"`
	Missing        []string             `json:"missing,omitempty"`
	Candidates     []domain.POI         `json:"candidates,omitempty"`
	Draft          domain.DraftPlan     `json:"draft"`
	Plan           domain.Itinerary     `json:"plan"`
	Violations     []domain.Violation   `json:"violations,omitempty"`
	Round          int                  `json:"round"`
	Best           *domain.Itinerary    `json:"best,omitempty"`
	BestViolations []domain.Violation   `json:"best_violations,omitempty"`
	History        []RoundMetrics       `json:"history,omitempty"`
	StopReason     string               `json:"stop_reason,omitempty"`
	Suggestions    []string             `json:"suggestions,omitempty"`
	Snapshot       *VersionSnapshot     `json:"snapshot,omitempty"`
	Previous       *domain.Itinerary    `json:"previous,omitempty"`
	PreviousSeq    int                  `json:"previous_seq,omitempty"`
	Status         string               `json:"status"`
	Error          *RunError            `json:"error,omitempty"`
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
