package repo

import "context"

func (s *Store) ListEvalCases(ctx context.Context) ([]EvalCase, error) {
	return s.q.ListEvalCases(ctx)
}

func (s *Store) GetEvalCase(ctx context.Context, id string) (EvalCase, error) {
	return s.q.GetEvalCase(ctx, id)
}

func (s *Store) UpsertEvalCase(ctx context.Context, arg UpsertEvalCaseParams) error {
	return s.q.UpsertEvalCase(ctx, arg)
}

func (s *Store) InsertEvalRun(ctx context.Context, arg InsertEvalRunParams) (EvalRun, error) {
	return s.q.InsertEvalRun(ctx, arg)
}

func (s *Store) ListEvalRuns(ctx context.Context) ([]EvalRun, error) {
	return s.q.ListEvalRuns(ctx)
}

func (s *Store) ListEvalRunsByCase(ctx context.Context, caseID string) ([]EvalRun, error) {
	return s.q.ListEvalRunsByCase(ctx, caseID)
}
