package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"
)

type statusUpdate struct {
	runID  string
	status string
	stage  string
}

type fakeRunStore struct {
	mu       sync.Mutex
	runs     map[string]RunRecord
	states   map[string][]byte
	stages   map[string][]string
	statuses []statusUpdate
}

func newFakeRunStore(runs ...RunRecord) *fakeRunStore {
	store := &fakeRunStore{
		runs:   make(map[string]RunRecord, len(runs)),
		states: make(map[string][]byte),
		stages: make(map[string][]string),
	}
	for _, run := range runs {
		store.runs[run.ID] = run
	}
	return store
}

func (s *fakeRunStore) GetRun(_ context.Context, runID string) (RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return RunRecord{}, errors.New("run not found")
	}
	return run, nil
}

func (s *fakeRunStore) ListRunsByStatus(_ context.Context, statuses []string) ([]RunRecord, error) {
	wanted := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		wanted[status] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.runs))
	for id := range s.runs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]RunRecord, 0)
	for _, id := range ids {
		if _, ok := wanted[s.runs[id].Status]; ok {
			out = append(out, s.runs[id])
		}
	}
	return out, nil
}

func (s *fakeRunStore) SaveRunState(_ context.Context, runID string, state json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[runID] = append([]byte(nil), state...)
	if run, ok := s.runs[runID]; ok {
		run.State = append([]byte(nil), state...)
		s.runs[runID] = run
	}
	decoded, err := DecodeTripState(state)
	if err == nil {
		s.stages[runID] = append(s.stages[runID], decoded.Stage)
	}
	return nil
}

func (s *fakeRunStore) UpdateRunStatus(_ context.Context, runID, status, stage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return errors.New("run not found")
	}
	run.Status = status
	run.Stage = stage
	s.runs[runID] = run
	s.statuses = append(s.statuses, statusUpdate{runID: runID, status: status, stage: stage})
	return nil
}

func (s *fakeRunStore) statusOf(runID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[runID].Status
}

func (s *fakeRunStore) stateOf(t *testing.T, runID string) *TripState {
	t.Helper()
	s.mu.Lock()
	raw := append([]byte(nil), s.states[runID]...)
	s.mu.Unlock()
	if len(raw) == 0 {
		t.Fatalf("no state saved for %s", runID)
	}
	state, err := DecodeTripState(raw)
	if err != nil {
		t.Fatalf("decode state: %v", err)
	}
	return state
}

func (s *fakeRunStore) stagesOf(runID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.stages[runID]...)
}

func (s *fakeRunStore) setRun(run RunRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
}

func runnerRun(id, status, input string) RunRecord {
	return RunRecord{ID: id, TripID: "trip-" + id, Mode: "initial", Status: status, Input: input}
}

func newTestRunner(t *testing.T, store RunStore, events *eventRecorder, llm LLMClient, opts ...RunnerOption) *Runner {
	t.Helper()
	validator := &fakeValidator{}
	nodes := graphNodes(llm, events, validator)
	runner, err := NewRunner(context.Background(), store, events, nodes, opts...)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	return runner
}

func TestRunner(t *testing.T) {
	t.Run("start executes the graph and awaits the user", func(t *testing.T) {
		store := newFakeRunStore(runnerRun("run-1", StatusQueued, "北京一天"))
		events := &eventRecorder{}
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
		}}
		runner := newTestRunner(t, store, events, llm)

		if err := runner.Start(context.Background(), "run-1"); err != nil {
			t.Fatalf("start: %v", err)
		}
		runner.Wait()

		if got := store.statusOf("run-1"); got != StatusAwaitingUser {
			t.Fatalf("status = %s", got)
		}
		state := store.stateOf(t, "run-1")
		if state.Snapshot == nil || state.Status != StatusAwaitingUser {
			t.Fatalf("state = %+v", state)
		}
		stages := store.stagesOf("run-1")
		for _, want := range []string{nodeParse, nodeRetrieve, nodeDraft, nodeSchedule, nodeValidate, nodePresent} {
			if !containsString(stages, want) {
				t.Fatalf("missing stage %s in %v", want, stages)
			}
		}
		types := events.types()
		if !containsString(types, "run.started") || !containsString(types, "run.awaiting_user") {
			t.Fatalf("events = %v", types)
		}
	})

	t.Run("start rejects a run that is not queued", func(t *testing.T) {
		store := newFakeRunStore(runnerRun("run-1", StatusAwaitingUser, "北京一天"))
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{})
		err := runner.Start(context.Background(), "run-1")
		if !errors.Is(err, ErrInvalidRunStatus) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("cancel stops a running graph once", func(t *testing.T) {
		store := newFakeRunStore(runnerRun("run-1", StatusQueued, "北京一天"))
		events := &eventRecorder{}
		llm := &blockingLLM{started: make(chan struct{})}
		runner := newTestRunner(t, store, events, llm)

		if err := runner.Start(context.Background(), "run-1"); err != nil {
			t.Fatalf("start: %v", err)
		}
		select {
		case <-llm.started:
		case <-time.After(5 * time.Second):
			t.Fatal("graph did not start")
		}
		if err := runner.Cancel(context.Background(), "run-1"); err != nil {
			t.Fatalf("cancel: %v", err)
		}
		runner.Wait()

		if got := store.statusOf("run-1"); got != StatusCancelled {
			t.Fatalf("status = %s", got)
		}
		types := events.types()
		if countString(types, "run.cancelled") != 1 {
			t.Fatalf("events = %v", types)
		}
		if containsString(types, "run.failed") {
			t.Fatalf("events = %v", types)
		}
	})

	t.Run("recover requeues queued, fails running and keeps awaiting", func(t *testing.T) {
		store := newFakeRunStore(
			runnerRun("queued", StatusQueued, "北京一天"),
			runnerRun("running", StatusRunning, "北京一天"),
			runnerRun("awaiting", StatusAwaitingUser, "北京一天"),
		)
		events := &eventRecorder{}
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
		}}
		runner := newTestRunner(t, store, events, llm)

		if err := runner.Recover(context.Background()); err != nil {
			t.Fatalf("recover: %v", err)
		}
		runner.Wait()

		if got := store.statusOf("queued"); got != StatusAwaitingUser {
			t.Fatalf("queued status = %s", got)
		}
		if got := store.statusOf("running"); got != StatusFailed {
			t.Fatalf("running status = %s", got)
		}
		if got := store.statusOf("awaiting"); got != StatusAwaitingUser {
			t.Fatalf("awaiting status = %s", got)
		}
		types := events.types()
		if !containsString(types, "run.failed") || !containsString(types, "run.started") {
			t.Fatalf("events = %v", types)
		}
	})

	t.Run("resume supplements input, then accepts", func(t *testing.T) {
		missingRaw, err := (&TripState{
			RunID:   "run-1",
			TripID:  "trip-run-1",
			Input:   "去玩",
			Missing: []string{"city", "days"},
			Status:  StatusAwaitingUser,
			Stage:   nodeParse,
		}).Encode()
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		store := newFakeRunStore(RunRecord{
			ID: "run-1", TripID: "trip-run-1", Status: StatusAwaitingUser, Input: "去玩", State: missingRaw,
		})
		events := &eventRecorder{}
		llm := &fakeLLM{replies: []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":1}`),
			graphDraftReply(),
		}}
		runner := newTestRunner(t, store, events, llm)

		if err := runner.Resume(context.Background(), "run-1", ResumeAction{Kind: ResumeSupplement, Input: "北京一天"}); err != nil {
			t.Fatalf("supplement: %v", err)
		}
		runner.Wait()
		if got := store.statusOf("run-1"); got != StatusAwaitingUser {
			t.Fatalf("status = %s", got)
		}
		state := store.stateOf(t, "run-1")
		if state.Snapshot == nil || state.Input != "去玩 北京一天" {
			t.Fatalf("state = %+v", state)
		}

		if err := runner.Resume(context.Background(), "run-1", ResumeAction{Kind: ResumeAccept}); err != nil {
			t.Fatalf("accept: %v", err)
		}
		if got := store.statusOf("run-1"); got != StatusCompleted {
			t.Fatalf("status = %s", got)
		}
		if !containsString(events.types(), "run.completed") {
			t.Fatalf("events = %v", events.types())
		}
	})

	t.Run("resume rejects an awaiting plan", func(t *testing.T) {
		raw, err := (&TripState{
			RunID:    "run-1",
			Input:    "北京一天",
			Status:   StatusAwaitingUser,
			Snapshot: &VersionSnapshot{Seq: 1},
		}).Encode()
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		store := newFakeRunStore(RunRecord{ID: "run-1", Status: StatusAwaitingUser, State: raw})
		events := &eventRecorder{}
		runner := newTestRunner(t, store, events, &fakeLLM{})

		if err := runner.Resume(context.Background(), "run-1", ResumeAction{Kind: ResumeReject}); err != nil {
			t.Fatalf("reject: %v", err)
		}
		if got := store.statusOf("run-1"); got != StatusCancelled {
			t.Fatalf("status = %s", got)
		}
		types := events.types()
		if countString(types, "run.cancelled") != 1 {
			t.Fatalf("events = %v", types)
		}
	})

	t.Run("resume requires an awaiting run", func(t *testing.T) {
		store := newFakeRunStore(runnerRun("run-1", StatusQueued, "北京一天"))
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{})
		err := runner.Resume(context.Background(), "run-1", ResumeAction{Kind: ResumeAccept})
		if !errors.Is(err, ErrInvalidRunStatus) {
			t.Fatalf("err = %v", err)
		}
	})
}

type blockingLLM struct {
	started chan struct{}
	once    sync.Once
}

func (b *blockingLLM) GenerateJSON(ctx context.Context, _ string, _ json.RawMessage) (json.RawMessage, Usage, error) {
	b.once.Do(func() { close(b.started) })
	<-ctx.Done()
	return nil, Usage{}, ctx.Err()
}
