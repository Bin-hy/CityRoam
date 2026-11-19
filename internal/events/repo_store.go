package events

import (
	"context"
	"encoding/json"

	"trippilot/internal/repo"
)

// RepoStore adapts the Postgres store to the Broker's persistence interface.
type RepoStore struct {
	store *repo.Store
}

func NewRepoStore(store *repo.Store) *RepoStore {
	return &RepoStore{store: store}
}

func (s *RepoStore) AppendEvent(ctx context.Context, runID, typ string, payload json.RawMessage) (Event, error) {
	row, err := s.store.AppendEvent(ctx, repo.AppendEventParams{
		RunID:   runID,
		Type:    typ,
		Payload: payload,
	})
	if err != nil {
		return Event{}, err
	}
	return fromRepoEvent(row), nil
}

func (s *RepoStore) ListEventsAfter(ctx context.Context, runID string, afterSeq int32) ([]Event, error) {
	rows, err := s.store.ListEventsAfter(ctx, runID, afterSeq)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromRepoEvent(row))
	}
	return out, nil
}

func fromRepoEvent(row repo.RunEvent) Event {
	return Event{
		ID:        row.ID,
		RunID:     row.RunID,
		Seq:       row.Seq,
		Type:      row.Type,
		Payload:   row.Payload,
		CreatedAt: row.CreatedAt,
	}
}
