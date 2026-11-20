package runstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
	"trippilot/internal/repo"
)

// planSnapshot keeps explanations inside the version plan JSON without a schema
// change. Older readers ignore the extra field.
type planSnapshot struct {
	domain.Itinerary
	Explanations []domain.Explanation `json:"explanations,omitempty"`
}

func encodeVersionPlan(snapshot agent.VersionSnapshot) (json.RawMessage, error) {
	return json.Marshal(planSnapshot{
		Itinerary:    snapshot.Plan,
		Explanations: snapshot.Explanations,
	})
}

// Store adapts the Postgres repository to agent.RunStore.
type Store struct {
	store *repo.Store
	now   func() time.Time
}

func New(store *repo.Store) *Store {
	return &Store{store: store, now: time.Now}
}

func (s *Store) GetRun(ctx context.Context, runID string) (agent.RunRecord, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return agent.RunRecord{}, err
	}
	return record(run), nil
}

func (s *Store) ListRunsByStatus(ctx context.Context, statuses []string) ([]agent.RunRecord, error) {
	runs, err := s.store.ListRunsByStatus(ctx, statuses)
	if err != nil {
		return nil, err
	}
	out := make([]agent.RunRecord, 0, len(runs))
	for _, run := range runs {
		out = append(out, record(run))
	}
	return out, nil
}

func (s *Store) SaveRunState(ctx context.Context, runID string, raw json.RawMessage) error {
	state, err := agent.DecodeTripState(raw)
	if err != nil {
		return fmt.Errorf("runstore: decode state: %w", err)
	}
	params := repo.UpdateRunStateParams{
		ID:     runID,
		State:  raw,
		Stage:  state.Stage,
		Round:  int32(state.Round),
		Status: state.Status,
	}
	if state.Status == "" {
		return fmt.Errorf("runstore: state has no status")
	}
	if state.Proposal != nil {
		proposal, err := json.Marshal(state.Proposal)
		if err != nil {
			return fmt.Errorf("runstore: encode proposal: %w", err)
		}
		params.Proposal = proposal
	}
	if state.Error != nil {
		code, message := state.Error.Code, state.Error.Message
		params.ErrorCode = &code
		params.ErrorMessage = &message
	}
	if terminal(state.Status) {
		finished := s.now()
		params.FinishedAt = &finished
	}
	if err := s.store.SaveRunState(ctx, params); err != nil {
		return fmt.Errorf("runstore: save state: %w", err)
	}
	return nil
}

func (s *Store) UpdateRunStatus(ctx context.Context, runID, status, stage string) error {
	params := repo.UpdateRunStatusParams{ID: runID, Status: status, Stage: stage}
	if terminal(status) {
		finished := s.now()
		params.FinishedAt = &finished
	}
	if err := s.store.UpdateRunStatus(ctx, params); err != nil {
		return fmt.Errorf("runstore: update status: %w", err)
	}
	return nil
}

func (s *Store) UpdateRunMode(ctx context.Context, runID, mode string) error {
	if err := s.store.UpdateRunMode(ctx, runID, mode); err != nil {
		return fmt.Errorf("runstore: update mode: %w", err)
	}
	return nil
}

func (s *Store) GetPlanVersion(ctx context.Context, tripID string, seq int) (agent.VersionState, error) {
	version, err := s.store.GetPlanVersionBySeq(ctx, tripID, int32(seq))
	if err != nil {
		return agent.VersionState{}, err
	}
	var state agent.VersionState
	state.Seq = int(version.Seq)
	if err := json.Unmarshal(version.Plan, &state.Plan); err != nil {
		return agent.VersionState{}, fmt.Errorf("runstore: decode plan: %w", err)
	}
	if err := json.Unmarshal(version.Constraints, &state.Constraints); err != nil {
		return agent.VersionState{}, fmt.Errorf("runstore: decode constraints: %w", err)
	}
	if len(version.Violations) > 0 {
		if err := json.Unmarshal(version.Violations, &state.Violations); err != nil {
			return agent.VersionState{}, fmt.Errorf("runstore: decode violations: %w", err)
		}
	}
	return state, nil
}

func record(run repo.Run) agent.RunRecord {
	return agent.RunRecord{
		ID:          run.ID,
		TripID:      run.TripID,
		Mode:        run.Mode,
		Status:      run.Status,
		Stage:       run.Stage,
		Input:       run.Input,
		Constraints: run.Constraints,
		State:       run.State,
	}
}

func terminal(status string) bool {
	switch status {
	case agent.StatusCompleted, agent.StatusFailed, agent.StatusCancelled:
		return true
	default:
		return false
	}
}

// SaveVersion persists a presented snapshot. The version sequence comes from
// the snapshot so the client-visible numbering matches storage.
func (s *Store) SaveVersion(ctx context.Context, runID string, snapshot agent.VersionSnapshot, status string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("runstore: get run for version: %w", err)
	}
	plan, err := encodeVersionPlan(snapshot)
	if err != nil {
		return fmt.Errorf("runstore: encode plan: %w", err)
	}
	constraints, err := json.Marshal(snapshot.Constraints)
	if err != nil {
		return fmt.Errorf("runstore: encode constraints: %w", err)
	}
	violations, err := json.Marshal(snapshot.Violations)
	if err != nil {
		return fmt.Errorf("runstore: encode violations: %w", err)
	}
	diffSummary, err := json.Marshal(snapshot.Diff)
	if err != nil {
		return fmt.Errorf("runstore: encode diff: %w", err)
	}
	research := json.RawMessage(`{}`)
	if snapshot.Research != nil {
		encoded, err := json.Marshal(snapshot.Research)
		if err != nil {
			return fmt.Errorf("runstore: encode research: %w", err)
		}
		research = encoded
	}
	var reason *string
	if snapshot.Reason != "" {
		value := snapshot.Reason
		reason = &value
	}
	_, err = s.store.SaveVersion(ctx, repo.CreatePlanVersionParams{
		TripID:      run.TripID,
		RunID:       runID,
		Seq:         int32(snapshot.Seq),
		Plan:        plan,
		Constraints: constraints,
		Violations:  violations,
		DiffSummary: diffSummary,
		Reason:      reason,
		Status:      status,
		Research:    research,
	})
	if err != nil {
		return fmt.Errorf("runstore: save version: %w", err)
	}
	return nil
}

func (s *Store) AcceptVersion(ctx context.Context, runID string, seq int) error {
	if err := s.store.AcceptPlanVersion(ctx, runID, int32(seq)); err != nil {
		return fmt.Errorf("runstore: accept version: %w", err)
	}
	return nil
}
