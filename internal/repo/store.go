package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	q    *Queries
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping database: %w", err)
	}
	return NewStore(pool), nil
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: New(pool)}
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) CreateTrip(ctx context.Context, arg CreateTripParams) (Trip, error) {
	if arg.RequestID != nil {
		existing, err := s.q.GetTripByRequestID(ctx, arg.RequestID)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Trip{}, fmt.Errorf("store: lookup trip by request id: %w", err)
		}
	}
	trip, err := s.q.CreateTrip(ctx, arg)
	if err != nil {
		return Trip{}, fmt.Errorf("store: create trip: %w", err)
	}
	return trip, nil
}

func (s *Store) GetTrip(ctx context.Context, id string) (Trip, error) {
	return s.q.GetTrip(ctx, id)
}

func (s *Store) GetTripByRequestID(ctx context.Context, requestID string) (Trip, error) {
	return s.q.GetTripByRequestID(ctx, &requestID)
}

func (s *Store) UpdateTripStatus(ctx context.Context, id, status string) error {
	return s.q.UpdateTripStatus(ctx, UpdateTripStatusParams{ID: id, Status: status})
}

func (s *Store) SoftDeleteTrip(ctx context.Context, id string) error {
	return s.q.SoftDeleteTrip(ctx, id)
}

func (s *Store) CreateRun(ctx context.Context, arg CreateRunParams) (Run, error) {
	if arg.RequestID != nil {
		existing, err := s.q.GetRunByRequestID(ctx, arg.RequestID)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Run{}, fmt.Errorf("store: lookup run by request id: %w", err)
		}
	}
	run, err := s.q.CreateRun(ctx, arg)
	if err != nil {
		return Run{}, fmt.Errorf("store: create run: %w", err)
	}
	return run, nil
}

func (s *Store) GetRun(ctx context.Context, id string) (Run, error) {
	return s.q.GetRun(ctx, id)
}

func (s *Store) ListRunsByTrip(ctx context.Context, tripID string) ([]Run, error) {
	return s.q.ListRunsByTrip(ctx, tripID)
}

func (s *Store) SaveRunState(ctx context.Context, arg UpdateRunStateParams) error {
	if err := s.q.UpdateRunState(ctx, arg); err != nil {
		return fmt.Errorf("store: save run state: %w", err)
	}
	return nil
}

func (s *Store) UpdateRunStatus(ctx context.Context, arg UpdateRunStatusParams) error {
	if err := s.q.UpdateRunStatus(ctx, arg); err != nil {
		return fmt.Errorf("store: update run status: %w", err)
	}
	return nil
}

// SaveVersion allocates the next sequence inside a transaction so concurrent
// writers cannot produce duplicate version numbers.
func (s *Store) SaveVersion(ctx context.Context, arg CreatePlanVersionParams) (PlanVersion, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PlanVersion{}, fmt.Errorf("store: begin version: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.q.WithTx(tx)
	seq, err := q.NextPlanVersionSeq(ctx, arg.TripID)
	if err != nil {
		return PlanVersion{}, fmt.Errorf("store: next version seq: %w", err)
	}
	arg.Seq = seq
	version, err := q.CreatePlanVersion(ctx, arg)
	if err != nil {
		return PlanVersion{}, fmt.Errorf("store: create version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return PlanVersion{}, fmt.Errorf("store: commit version: %w", err)
	}
	return version, nil
}

func (s *Store) AppendEvent(ctx context.Context, arg AppendEventParams) (RunEvent, error) {
	event, err := s.q.AppendEvent(ctx, arg)
	if err != nil {
		return RunEvent{}, fmt.Errorf("store: append event: %w", err)
	}
	return event, nil
}

func (s *Store) ListEventsAfter(ctx context.Context, runID string, afterSeq int32) ([]RunEvent, error) {
	return s.q.ListEventsAfter(ctx, ListEventsAfterParams{RunID: runID, Seq: afterSeq})
}
