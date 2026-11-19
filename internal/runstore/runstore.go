package runstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/repo"
)

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
