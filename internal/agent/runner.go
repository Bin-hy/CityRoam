package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"trippilot/internal/domain"
)

// RunRecord is the storage-neutral view of a run the Runner needs.
type RunRecord struct {
	ID          string
	TripID      string
	Mode        string
	Status      string
	Stage       string
	Input       string
	Constraints json.RawMessage
	State       json.RawMessage
}

// RunStore is implemented by the Postgres store (or a fake in tests).
type RunStore interface {
	GetRun(ctx context.Context, runID string) (RunRecord, error)
	ListRunsByStatus(ctx context.Context, statuses []string) ([]RunRecord, error)
	SaveRunState(ctx context.Context, runID string, state json.RawMessage) error
	UpdateRunStatus(ctx context.Context, runID, status, stage string) error
	SaveVersion(ctx context.Context, runID string, snapshot VersionSnapshot, status string) error
	AcceptVersion(ctx context.Context, runID string, seq int) error
}

type ResumeKind string

const (
	ResumeSupplement ResumeKind = "supplement"
	ResumeAccept     ResumeKind = "accept"
	ResumeReject     ResumeKind = "reject"
)

type ResumeAction struct {
	Kind  ResumeKind
	Input string
}

var ErrInvalidRunStatus = errors.New("runner: invalid run status")

type runnerOptions struct {
	concurrency int
	logger      *slog.Logger
}

type RunnerOption func(*runnerOptions)

func WithRunnerConcurrency(limit int) RunnerOption {
	return func(options *runnerOptions) {
		if limit > 0 {
			options.concurrency = limit
		}
	}
}

func WithRunnerLogger(logger *slog.Logger) RunnerOption {
	return func(options *runnerOptions) { options.logger = logger }
}

type runControl struct {
	cancel context.CancelFunc
	once   sync.Once
}

type startRecord struct {
	done chan struct{}
	err  error
}

// Runner owns run lifecycle: it executes the graph asynchronously, tracks
// cancellation and resumes awaiting runs from persisted TripState.
type Runner struct {
	store  RunStore
	events EventPublisher
	nodes  *Nodes
	graph  *Graph
	logger *slog.Logger
	sem    chan struct{}

	mu       sync.Mutex
	controls map[string]*runControl
	requests map[string]*startRecord
	wg       sync.WaitGroup
}

func NewRunner(ctx context.Context, store RunStore, events EventPublisher, nodes *Nodes, opts ...RunnerOption) (*Runner, error) {
	if store == nil {
		return nil, fmt.Errorf("agent: runner requires store")
	}
	if events == nil {
		return nil, fmt.Errorf("agent: runner requires event publisher")
	}

	options := runnerOptions{concurrency: 3, logger: slog.Default()}
	for _, opt := range opts {
		opt(&options)
	}

	runner := &Runner{
		store:    store,
		events:   events,
		nodes:    nodes,
		logger:   options.logger,
		sem:      make(chan struct{}, options.concurrency),
		controls: make(map[string]*runControl),
		requests: make(map[string]*startRecord),
	}
	graph, err := NewGraph(ctx, nodes, WithStageHook(runner.saveStage))
	if err != nil {
		return nil, err
	}
	runner.graph = graph
	return runner, nil
}

// Wait blocks until every started run goroutine has finished.
func (r *Runner) Wait() {
	r.wg.Wait()
}

// Start loads a queued run and executes it asynchronously.
func (r *Runner) Start(ctx context.Context, runID string) error {
	run, err := r.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("runner: get run: %w", err)
	}
	if run.Status != StatusQueued {
		return fmt.Errorf("%w: %s", ErrInvalidRunStatus, run.Status)
	}

	state, err := stateFromRun(run)
	if err != nil {
		return fmt.Errorf("runner: restore state: %w", err)
	}
	state.Status = StatusQueued
	state.Stage = StatusQueued
	if err := r.saveState(ctx, state); err != nil {
		return err
	}
	if err := r.events.Publish(ctx, runID, "run.started", map[string]any{
		"mode":  state.Mode,
		"input": state.Input,
	}); err != nil {
		return fmt.Errorf("runner: publish run.started: %w", err)
	}

	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.setControl(runID, &runControl{cancel: cancel})

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer r.clearControl(runID)
		r.execute(runCtx, state)
	}()
	return nil
}

// StartRequest deduplicates concurrent starts that share a request_id so one
// request can never create two executions. Cross-process dedupe relies on the
// unique runs.request_id index in storage.
func (r *Runner) StartRequest(ctx context.Context, runID, requestID string) error {
	if requestID == "" {
		return r.Start(ctx, runID)
	}

	r.mu.Lock()
	if record, exists := r.requests[requestID]; exists {
		r.mu.Unlock()
		<-record.done
		return record.err
	}
	record := &startRecord{done: make(chan struct{})}
	r.requests[requestID] = record
	r.mu.Unlock()

	err := r.Start(ctx, runID)
	record.err = err
	close(record.done)
	return err
}

// Resume supplements missing constraints, accepts or rejects an awaiting plan.
func (r *Runner) Resume(ctx context.Context, runID string, action ResumeAction) error {
	run, err := r.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("runner: get run: %w", err)
	}
	if run.Status != StatusAwaitingUser {
		return fmt.Errorf("%w: %s", ErrInvalidRunStatus, run.Status)
	}
	state, err := stateFromRun(run)
	if err != nil {
		return fmt.Errorf("runner: restore state: %w", err)
	}

	switch action.Kind {
	case ResumeSupplement:
		if strings.TrimSpace(action.Input) == "" {
			return fmt.Errorf("runner: supplement input is empty")
		}
		state.Input = strings.TrimSpace(state.Input + " " + action.Input)
		state.Missing = nil
		state.Candidates = nil
		state.Draft = domain.DraftPlan{}
		state.Plan = domain.Itinerary{}
		state.Violations = nil
		state.History = nil
		state.Best = nil
		state.BestViolations = nil
		state.StopReason = ""
		state.Suggestions = nil
		state.Snapshot = nil
		state.Previous = nil
		state.PreviousSeq = 0
		state.Status = StatusQueued
		state.Stage = StatusQueued
		state.Error = nil
		if err := r.saveState(ctx, state); err != nil {
			return err
		}
		if err := r.store.UpdateRunStatus(ctx, runID, StatusQueued, StatusQueued); err != nil {
			return fmt.Errorf("runner: requeue run: %w", err)
		}
		return r.Start(ctx, runID)

	case ResumeAccept:
		if state.Snapshot == nil {
			if len(state.Plan.Days) == 0 {
				return fmt.Errorf("runner: run has no plan to accept")
			}
			out, err := r.nodes.Present(ctx, PresentInput{
				RunID:        runID,
				Constraints:  state.Constraints,
				Plan:         state.Plan,
				Violations:   state.Violations,
				Reason:       "adjustment",
				Explanations: state.Draft.Explanations,
				Suggestions:  state.Suggestions,
				Previous:     state.Previous,
				PreviousSeq:  state.PreviousSeq,
			})
			if err != nil {
				return fmt.Errorf("runner: snapshot accepted plan: %w", err)
			}
			if err := r.store.SaveVersion(ctx, runID, out.Snapshot, "accepted"); err != nil {
				return fmt.Errorf("runner: save accepted version: %w", err)
			}
			state.Snapshot = &out.Snapshot
		}
		if err := r.store.AcceptVersion(ctx, runID, state.Snapshot.Seq); err != nil {
			return fmt.Errorf("runner: accept version: %w", err)
		}
		state.Status = StatusCompleted
		state.Stage = "accepted"
		if err := r.saveState(ctx, state); err != nil {
			return err
		}
		if err := r.store.UpdateRunStatus(ctx, runID, StatusCompleted, "accepted"); err != nil {
			return fmt.Errorf("runner: complete run: %w", err)
		}
		if err := r.events.Publish(ctx, runID, "run.completed", map[string]any{
			"seq": state.Snapshot.Seq,
		}); err != nil {
			return fmt.Errorf("runner: publish run.completed: %w", err)
		}
		return nil

	case ResumeReject:
		state.Status = StatusCancelled
		state.Stage = "rejected"
		if err := r.saveState(ctx, state); err != nil {
			return err
		}
		if err := r.store.UpdateRunStatus(ctx, runID, StatusCancelled, "rejected"); err != nil {
			return fmt.Errorf("runner: reject run: %w", err)
		}
		if err := r.events.Publish(ctx, runID, "run.cancelled", map[string]any{
			"reason": "rejected",
		}); err != nil {
			return fmt.Errorf("runner: publish run.cancelled: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("runner: unknown resume action %q", action.Kind)
	}
}

// Cancel stops a run immediately and records the terminal state once.
func (r *Runner) Cancel(ctx context.Context, runID string) error {
	run, err := r.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("runner: get run: %w", err)
	}
	switch run.Status {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return nil
	}
	r.markCancelled(ctx, runID)
	return nil
}

// Recover resumes queued runs, fails interrupted running runs and leaves
// awaiting runs untouched so the user can continue them.
func (r *Runner) Recover(ctx context.Context) error {
	var errs []error

	// Snapshot running runs first: starting queued runs immediately flips
	// them to running and must not make them look interrupted.
	running, err := r.store.ListRunsByStatus(ctx, []string{StatusRunning})
	if err != nil {
		errs = append(errs, fmt.Errorf("runner: list running: %w", err))
	}
	for _, run := range running {
		bg := context.WithoutCancel(ctx)
		if err := r.store.UpdateRunStatus(bg, run.ID, StatusFailed, "recovered"); err != nil {
			errs = append(errs, fmt.Errorf("runner: fail interrupted %s: %w", run.ID, err))
			continue
		}
		if err := r.events.Publish(bg, run.ID, "run.failed", map[string]any{
			"code":    "interrupted",
			"message": "服务重启时运行中断",
		}); err != nil {
			errs = append(errs, fmt.Errorf("runner: publish interrupted %s: %w", run.ID, err))
		}
	}

	queued, err := r.store.ListRunsByStatus(ctx, []string{StatusQueued})
	if err != nil {
		errs = append(errs, fmt.Errorf("runner: list queued: %w", err))
	}
	for _, run := range queued {
		if err := r.Start(ctx, run.ID); err != nil {
			errs = append(errs, fmt.Errorf("runner: recover %s: %w", run.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (r *Runner) execute(ctx context.Context, state *TripState) {
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		r.markCancelled(ctx, state.RunID)
		return
	}
	if ctx.Err() != nil {
		r.markCancelled(ctx, state.RunID)
		return
	}

	stage := nodeAdmit
	if state.Mode == "adjustment" {
		stage = "adjust"
	}
	if err := r.store.UpdateRunStatus(ctx, state.RunID, StatusRunning, stage); err != nil {
		r.fail(ctx, state, fmt.Errorf("mark running: %w", err))
		return
	}

	if state.Mode == "adjustment" {
		r.executeAdjustment(ctx, state)
		return
	}

	out, err := r.graph.Run(ctx, state)
	if err != nil {
		if ctx.Err() != nil {
			r.markCancelled(ctx, state.RunID)
			return
		}
		r.fail(ctx, state, err)
		return
	}
	if ctx.Err() != nil {
		r.markCancelled(ctx, state.RunID)
		return
	}

	if out.Snapshot != nil {
		if err := r.store.SaveVersion(ctx, out.RunID, *out.Snapshot, "draft"); err != nil {
			r.fail(ctx, state, err)
			return
		}
	}
	stage = nodePresent
	if out.Snapshot == nil {
		stage = nodeParse
		if out.Proposal != nil {
			stage = "proposal"
		}
	}
	if err := r.store.UpdateRunStatus(ctx, out.RunID, out.Status, stage); err != nil {
		r.logger.Error("runner: update awaiting status", "run_id", out.RunID, "error", err)
	}
	payload := map[string]any{
		"missing": out.Missing,
		"stage":   stage,
		"seq":     snapshotSeq(out.Snapshot),
	}
	if out.Proposal != nil {
		payload["proposal"] = out.Proposal
	}
	if err := r.events.Publish(ctx, out.RunID, "run.awaiting_user", payload); err != nil {
		r.logger.Error("runner: publish awaiting", "run_id", out.RunID, "error", err)
	}
}

// executeAdjustment runs one adjustment proposal and leaves the run awaiting
// the user's accept/reject decision.
func (r *Runner) executeAdjustment(ctx context.Context, state *TripState) {
	if len(state.Plan.Days) == 0 || len(state.Draft.Days) == 0 {
		r.fail(ctx, state, &NodeError{Code: "adjustment_base_missing", Message: "缺少可调整的行程版本"})
		return
	}
	out, err := r.nodes.Adjust(ctx, AdjustInput{
		RunID:          state.RunID,
		Input:          state.Input,
		Constraints:    state.Constraints,
		BasePlan:       state.Plan,
		BaseDraft:      state.Draft,
		BaseViolations: state.Violations,
		Candidates:     state.Candidates,
		POIs:           poiMap(state.Candidates),
		BaseVersion:    state.PreviousSeq,
	})
	if err != nil {
		r.fail(ctx, state, err)
		return
	}

	state.Plan = out.Plan
	state.Draft = out.Draft
	state.Violations = out.Violations
	state.Candidates = out.Alternatives
	state.Proposal = &out.Proposal
	state.Suggestions = RelaxationSuggestions(out.Violations, state.Constraints)
	state.StopReason = ""
	state.Snapshot = nil
	state.Status = StatusAwaitingUser
	state.Stage = "proposal"
	if err := r.saveState(ctx, state); err != nil {
		r.fail(ctx, state, err)
		return
	}
	if err := r.store.UpdateRunStatus(ctx, state.RunID, StatusAwaitingUser, "proposal"); err != nil {
		r.logger.Error("runner: update proposal status", "run_id", state.RunID, "error", err)
	}
	if err := r.events.Publish(ctx, state.RunID, "run.awaiting_user", map[string]any{
		"stage":    "proposal",
		"proposal": out.Proposal,
	}); err != nil {
		r.logger.Error("runner: publish proposal", "run_id", state.RunID, "error", err)
	}
}

func (r *Runner) saveStage(ctx context.Context, _ string, state *TripState) error {
	return r.saveState(ctx, state)
}

func (r *Runner) saveState(ctx context.Context, state *TripState) error {
	encoded, err := state.Encode()
	if err != nil {
		return fmt.Errorf("runner: encode state: %w", err)
	}
	if err := r.store.SaveRunState(ctx, state.RunID, encoded); err != nil {
		return fmt.Errorf("runner: save state: %w", err)
	}
	return nil
}

func (r *Runner) fail(ctx context.Context, state *TripState, cause error) {
	code := "run_failed"
	message := cause.Error()
	var nodeErr *NodeError
	if errors.As(cause, &nodeErr) {
		code = nodeErr.Code
		message = nodeErr.Message
	}
	state.Status = StatusFailed
	state.Stage = "failed"
	state.Error = &RunError{Code: code, Message: message}

	bg := context.WithoutCancel(ctx)
	if err := r.saveState(bg, state); err != nil {
		r.logger.Error("runner: save failed state", "run_id", state.RunID, "error", err)
	}
	if err := r.store.UpdateRunStatus(bg, state.RunID, StatusFailed, "failed"); err != nil {
		r.logger.Error("runner: mark failed", "run_id", state.RunID, "error", err)
	}
	if err := r.events.Publish(bg, state.RunID, "run.failed", map[string]any{
		"code":    code,
		"message": message,
	}); err != nil {
		r.logger.Error("runner: publish failed", "run_id", state.RunID, "error", err)
	}
}

func (r *Runner) markCancelled(ctx context.Context, runID string) {
	control := r.control(runID)
	if control != nil {
		control.once.Do(func() {
			control.cancel()
			r.recordCancelled(ctx, runID)
		})
		return
	}
	r.recordCancelled(ctx, runID)
}

func (r *Runner) recordCancelled(ctx context.Context, runID string) {
	bg := context.WithoutCancel(ctx)
	if err := r.store.UpdateRunStatus(bg, runID, StatusCancelled, "cancelled"); err != nil {
		r.logger.Error("runner: mark cancelled", "run_id", runID, "error", err)
	}
	if err := r.events.Publish(bg, runID, "run.cancelled", map[string]any{}); err != nil {
		r.logger.Error("runner: publish cancelled", "run_id", runID, "error", err)
	}
}

func (r *Runner) setControl(runID string, control *runControl) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.controls[runID] = control
}

func (r *Runner) control(runID string) *runControl {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.controls[runID]
}

func (r *Runner) clearControl(runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.controls, runID)
}

func stateFromRun(run RunRecord) (*TripState, error) {
	var state TripState
	if len(run.State) > 0 {
		decoded, err := DecodeTripState(run.State)
		if err != nil {
			return nil, err
		}
		state = *decoded
	}
	if state.RunID == "" {
		state.RunID = run.ID
	}
	if state.TripID == "" {
		state.TripID = run.TripID
	}
	if state.Mode == "" {
		state.Mode = run.Mode
	}
	if state.Input == "" {
		state.Input = run.Input
	}
	if len(run.Constraints) > 0 && state.Constraints.City == "" {
		if err := json.Unmarshal(run.Constraints, &state.Constraints); err != nil {
			return nil, fmt.Errorf("decode constraints: %w", err)
		}
	}
	return &state, nil
}

func snapshotSeq(snapshot *VersionSnapshot) int {
	if snapshot == nil {
		return 0
	}
	return snapshot.Seq
}
