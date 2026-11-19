package eval

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"trippilot/internal/repo"
)

// Service adapts a loaded case set and a Runner to the admin HTTP API.
type Service struct {
	store  Store
	runner *Runner
	cases  []Case
	logger *slog.Logger

	mu      sync.Mutex
	running bool
}

func NewService(store Store, runner *Runner, cases []Case, logger *slog.Logger) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("eval: service requires store")
	}
	if runner == nil {
		return nil, fmt.Errorf("eval: service requires runner")
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("eval: service requires cases")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, runner: runner, cases: cases, logger: logger}, nil
}

func (s *Service) ListCases(_ context.Context) ([]repo.EvalCase, error) {
	cases := make([]repo.EvalCase, 0, len(s.cases))
	for _, item := range s.cases {
		_, _, expected, tags, err := item.RepoCase()
		if err != nil {
			return nil, err
		}
		cases = append(cases, repo.EvalCase{
			ID:       item.CaseID,
			Input:    item.Input,
			Expected: expected,
			Tags:     tags,
			Enabled:  true,
		})
	}
	return cases, nil
}

func (s *Service) ListRuns(ctx context.Context) ([]repo.EvalRun, error) {
	return s.store.ListEvalRuns(ctx)
}

// Start launches one batch in the background and returns immediately. A
// second call while a batch is running is ignored.
func (s *Service) Start(_ context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
		}()
		timeout := time.Duration(len(s.cases))*defaultCaseTimeout + 5*time.Minute
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		report, err := s.runner.Run(ctx, s.cases)
		if err != nil {
			s.logger.Error("eval batch stopped", "error", err, "cases_done", len(report.Results))
			return
		}
		s.logger.Info("eval batch finished",
			"cases", report.Summary.Cases,
			"passed", report.Summary.Passed,
			"hard_pass_rate", report.Summary.HardPassRate,
			"p95_latency_ms", report.Summary.P95LatencyMS,
		)
	}()
	return nil
}
