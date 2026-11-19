package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/repo"
)

const (
	defaultCaseTimeout = 3 * time.Minute
	defaultPoll        = 250 * time.Millisecond
)

// Store is the persistence subset the evaluator needs.
type Store interface {
	CreateTripAndRun(ctx context.Context, trip repo.CreateTripParams, run repo.CreateRunParams) (repo.Trip, repo.Run, bool, error)
	GetRun(ctx context.Context, id string) (repo.Run, error)
	InsertEvalRun(ctx context.Context, arg repo.InsertEvalRunParams) (repo.EvalRun, error)
	ListEvalRuns(ctx context.Context) ([]repo.EvalRun, error)
}

// AgentRunner is implemented by agent.Runner.
type AgentRunner interface {
	Start(ctx context.Context, runID string) error
	Resume(ctx context.Context, runID string, action agent.ResumeAction) error
}

// Runner executes one case at a time so a batch never hides concurrency in
// its latency numbers.
type Runner struct {
	store   Store
	agent   AgentRunner
	logger  *slog.Logger
	timeout time.Duration
	poll    time.Duration
}

type RunnerOption func(*Runner)

func WithRunnerLogger(logger *slog.Logger) RunnerOption {
	return func(r *Runner) { r.logger = logger }
}

func WithCaseTimeout(timeout time.Duration) RunnerOption {
	return func(r *Runner) {
		if timeout > 0 {
			r.timeout = timeout
		}
	}
}

func WithPollInterval(interval time.Duration) RunnerOption {
	return func(r *Runner) {
		if interval > 0 {
			r.poll = interval
		}
	}
}

func NewRunner(store Store, runner AgentRunner, opts ...RunnerOption) *Runner {
	r := &Runner{
		store:   store,
		agent:   runner,
		logger:  slog.Default(),
		timeout: defaultCaseTimeout,
		poll:    defaultPoll,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run executes every case in order and aggregates the summary.
func (r *Runner) Run(ctx context.Context, cases []Case) (Report, error) {
	report := Report{
		GeneratedAt: time.Now(),
		Results:     make([]RunResult, 0, len(cases)),
	}
	for _, item := range cases {
		if err := ctx.Err(); err != nil {
			report.Summary = Summarize(report.Results)
			return report, err
		}
		result, err := r.RunCase(ctx, item)
		if err != nil {
			r.logger.Warn("eval case failed to execute", "case_id", item.CaseID, "error", err)
			if result.Reason == "" {
				result.Reason = err.Error()
			}
		}
		report.Results = append(report.Results, result)
		r.logger.Info("eval case finished",
			"case_id", item.CaseID,
			"passed", result.Passed,
			"latency_ms", result.Metrics.LatencyMS,
		)
	}
	report.Summary = Summarize(report.Results)
	return report, nil
}

// RunCase creates one trip/run, lets the agent finish, accepts the plan and
// records the resulting metrics.
func (r *Runner) RunCase(ctx context.Context, item Case) (RunResult, error) {
	started := time.Now()
	result := RunResult{CaseID: item.CaseID}

	caseCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	constraints, err := json.Marshal(map[string]any{
		"city": "beijing",
		"days": item.Expected.Days,
	})
	if err != nil {
		return finish(result, started, err)
	}

	_, run, _, err := r.store.CreateTripAndRun(caseCtx, repo.CreateTripParams{
		City:        "beijing",
		Days:        int32(item.Expected.Days),
		Constraints: constraints,
		Status:      "planning",
	}, repo.CreateRunParams{
		Mode:        "eval",
		Status:      agent.StatusQueued,
		Stage:       "admit",
		Input:       item.Input,
		Constraints: constraints,
	})
	if err != nil {
		return finish(result, started, fmt.Errorf("create eval run: %w", err))
	}
	result.RunID = run.ID

	if err := r.agent.Start(caseCtx, run.ID); err != nil {
		return finish(result, started, fmt.Errorf("start eval run: %w", err))
	}

	current, err := r.wait(caseCtx, run.ID, true)
	if err != nil {
		return finish(result, started, err)
	}

	if current.Status == agent.StatusAwaitingUser {
		state, decodeErr := agent.DecodeTripState(current.State)
		if decodeErr != nil {
			return finish(result, started, fmt.Errorf("decode awaiting state: %w", decodeErr))
		}
		if state.Snapshot == nil && len(state.Plan.Days) == 0 {
			result.Metrics = RunMetrics{LatencyMS: time.Since(started).Milliseconds()}
			result.Reason = "等待用户补充约束：" + strings.Join(state.Missing, "、")
			r.record(ctx, result)
			return result, nil
		}
		if err := r.agent.Resume(caseCtx, run.ID, agent.ResumeAction{Kind: agent.ResumeAccept}); err != nil {
			return finish(result, started, fmt.Errorf("accept eval plan: %w", err))
		}
		if current, err = r.wait(caseCtx, run.ID, false); err != nil {
			return finish(result, started, err)
		}
	}

	state, err := agent.DecodeTripState(current.State)
	if err != nil {
		return finish(result, started, fmt.Errorf("decode final state: %w", err))
	}

	metrics, reasons := Evaluate(item, state)
	metrics.LatencyMS = time.Since(started).Milliseconds()
	result.Metrics = metrics
	result.Completed = current.Status == agent.StatusCompleted
	result.Passed = result.Completed && metrics.HardPass
	if !result.Completed {
		reasons = append(reasons, "运行未完成："+current.Status)
	}
	result.Reason = strings.Join(reasons, "；")
	r.record(ctx, result)
	return result, nil
}

func finish(result RunResult, started time.Time, err error) (RunResult, error) {
	result.Metrics.LatencyMS = time.Since(started).Milliseconds()
	if result.Reason == "" {
		result.Reason = err.Error()
	}
	return result, err
}

func (r *Runner) wait(ctx context.Context, runID string, stopAtAwaiting bool) (repo.Run, error) {
	ticker := time.NewTicker(r.poll)
	defer ticker.Stop()
	for {
		run, err := r.store.GetRun(ctx, runID)
		if err != nil {
			return repo.Run{}, fmt.Errorf("get eval run: %w", err)
		}
		switch run.Status {
		case agent.StatusCompleted, agent.StatusFailed, agent.StatusCancelled:
			return run, nil
		case agent.StatusAwaitingUser:
			if stopAtAwaiting {
				return run, nil
			}
		}
		select {
		case <-ctx.Done():
			return repo.Run{}, fmt.Errorf("eval run %s timed out: %w", runID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (r *Runner) record(ctx context.Context, result RunResult) {
	metrics, err := json.Marshal(result.Metrics)
	if err != nil {
		r.logger.Warn("encode eval metrics", "case_id", result.CaseID, "error", err)
		return
	}
	var runID *string
	if result.RunID != "" {
		value := result.RunID
		runID = &value
	}
	duration := int32(result.Metrics.LatencyMS)
	if _, err := r.store.InsertEvalRun(ctx, repo.InsertEvalRunParams{
		CaseID:     result.CaseID,
		RunID:      runID,
		Metrics:    metrics,
		Passed:     result.Passed,
		DurationMs: &duration,
	}); err != nil {
		r.logger.Warn("insert eval run", "case_id", result.CaseID, "error", err)
	}
}
