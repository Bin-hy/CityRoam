package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"
	"trippilot/internal/domain"
)

type statusUpdate struct {
	runID  string
	status string
	stage  string
}

type savedVersion struct {
	runID    string
	snapshot VersionSnapshot
	status   string
}

type acceptedVersion struct {
	runID string
	seq   int
}

type fakeRunStore struct {
	mu           sync.Mutex
	runs         map[string]RunRecord
	states       map[string][]byte
	stages       map[string][]string
	statuses     []statusUpdate
	versions     []savedVersion
	accepted     []acceptedVersion
	planVersions map[string]map[int]VersionState
}

func newFakeRunStore(runs ...RunRecord) *fakeRunStore {
	store := &fakeRunStore{
		runs:         make(map[string]RunRecord, len(runs)),
		states:       make(map[string][]byte),
		stages:       make(map[string][]string),
		planVersions: make(map[string]map[int]VersionState),
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

func (s *fakeRunStore) UpdateRunMode(_ context.Context, runID, mode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return errors.New("run not found")
	}
	run.Mode = mode
	s.runs[runID] = run
	return nil
}

func (s *fakeRunStore) GetPlanVersion(_ context.Context, tripID string, seq int) (VersionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bySeq, ok := s.planVersions[tripID]
	if !ok {
		return VersionState{}, errors.New("trip not found")
	}
	state, ok := bySeq[seq]
	if !ok {
		return VersionState{}, errors.New("version not found")
	}
	return state, nil
}

func (s *fakeRunStore) seedPlanVersion(tripID string, state VersionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bySeq, ok := s.planVersions[tripID]
	if !ok {
		bySeq = make(map[int]VersionState)
		s.planVersions[tripID] = bySeq
	}
	bySeq[state.Seq] = state
}

func (s *fakeRunStore) SaveVersion(_ context.Context, runID string, snapshot VersionSnapshot, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versions = append(s.versions, savedVersion{runID: runID, snapshot: snapshot, status: status})
	return nil
}

func (s *fakeRunStore) AcceptVersion(_ context.Context, runID string, seq int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accepted = append(s.accepted, acceptedVersion{runID: runID, seq: seq})
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

func researchQuestionFixture() *domain.ResearchQuestion {
	return &domain.ResearchQuestion{
		Code:    "hotel_unconfirmed",
		Message: "未找到指定酒店",
		Options: []domain.ResearchOption{
			{ID: "hotel:h1", Label: "全季酒店", Hotel: &domain.Hotel{ID: "h1", Name: "全季酒店", Lat: 39.9, Lng: 116.4}},
			{ID: "retry_search", Label: "重新搜索酒店"},
			{ID: "continue_without_lodging", Label: "继续规划，但不计住宿费用"},
		},
	}
}

func awaitingResearchStore(t *testing.T, state *TripState) *fakeRunStore {
	t.Helper()
	raw, err := state.Encode()
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}
	return newFakeRunStore(RunRecord{
		ID: state.RunID, TripID: "trip-1", Status: StatusAwaitingUser, Input: state.Input, State: raw,
	})
}

func researchAwaitingState() *TripState {
	return &TripState{
		RunID:  "run-r",
		TripID: "trip-1",
		Input:  "北京两天，住华尔道夫",
		Status: StatusAwaitingUser,
		Stage:  "research",
		Constraints: domain.ConstraintSet{
			City:    "beijing",
			Days:    2,
			Lodging: domain.LodgingConstraint{Mentioned: true, Hotels: []string{"华尔道夫"}},
			Budget:  domain.BudgetConstraint{TotalYuan: 1200},
		},
		ResearchQuestion: researchQuestionFixture(),
	}
}

func TestResumeResearch(t *testing.T) {
	t.Run("requires an awaiting run", func(t *testing.T) {
		store := newFakeRunStore(runnerRun("run-r", StatusQueued, "北京两天"))
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{})
		err := runner.Resume(context.Background(), "run-r", ResumeAction{
			Kind: ResumeResearch, ResearchChoice: &ResearchChoice{OptionID: "retry_search"},
		})
		if !errors.Is(err, ErrInvalidRunStatus) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("requires a research question", func(t *testing.T) {
		state := researchAwaitingState()
		state.ResearchQuestion = nil
		store := awaitingResearchStore(t, state)
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{})
		err := runner.Resume(context.Background(), "run-r", ResumeAction{
			Kind: ResumeResearch, ResearchChoice: &ResearchChoice{OptionID: "retry_search"},
		})
		if !errors.Is(err, ErrInvalidRunStatus) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestResolveResearchChoice(t *testing.T) {
	replies := func() []json.RawMessage {
		return []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":2,"lodging":{"mentioned":true,"hotels":["华尔道夫"]}}`),
			json.RawMessage(`{"days":[{"items":[{"poi_id":"poi-0","type":"poi","duration_min":90}]},{"items":[{"poi_id":"poi-1","type":"poi","duration_min":60}]}]}`),
		}
	}

	t.Run("selects a hotel and requeues", func(t *testing.T) {
		store := awaitingResearchStore(t, researchAwaitingState())
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{replies: replies()})
		err := runner.Resume(context.Background(), "run-r", ResumeAction{
			Kind: ResumeResearch,
			ResearchChoice: &ResearchChoice{
				OptionID: "hotel:h1",
				Hotel:    &domain.Hotel{ID: "h1", Name: "全季酒店", Lat: 39.9, Lng: 116.4},
			},
		})
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		runner.Wait()
		state := store.stateOf(t, "run-r")
		if state.ResolvedHotel == nil || state.ResolvedHotel.ID != "h1" {
			t.Fatalf("resolved hotel = %+v", state.ResolvedHotel)
		}
		if state.ResearchQuestion != nil {
			t.Fatalf("question = %+v, want nil", state.ResearchQuestion)
		}
		if state.Snapshot == nil || store.statusOf("run-r") != StatusAwaitingUser {
			t.Fatalf("state = %+v status = %s", state, store.statusOf("run-r"))
		}
	})

	t.Run("continue without lodging disables it", func(t *testing.T) {
		store := awaitingResearchStore(t, researchAwaitingState())
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{replies: replies()})
		err := runner.Resume(context.Background(), "run-r", ResumeAction{
			Kind:           ResumeResearch,
			ResearchChoice: &ResearchChoice{OptionID: "continue_without_lodging"},
		})
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		runner.Wait()
		state := store.stateOf(t, "run-r")
		if state.ConstraintPatch == nil {
			t.Fatal("constraint patch = nil")
		}
		if state.ConstraintPatch.Lodging.Mentioned {
			t.Fatal("patch lodging still mentioned")
		}
		if state.ConstraintPatch.Budget.IncludeLodging == nil || *state.ConstraintPatch.Budget.IncludeLodging {
			t.Fatalf("include_lodging = %v, want false", state.ConstraintPatch.Budget.IncludeLodging)
		}
		if state.Constraints.Lodging.Mentioned || state.Constraints.LodgingIncluded() {
			t.Fatalf("merged constraints = %+v, want lodging off", state.Constraints)
		}
		if state.ResolvedHotel != nil {
			t.Fatalf("resolved hotel = %+v, want nil", state.ResolvedHotel)
		}
	})

	t.Run("retry search clears prior selection", func(t *testing.T) {
		state := researchAwaitingState()
		state.ResolvedHotel = &domain.Hotel{ID: "old", Name: "旧酒店"}
		store := awaitingResearchStore(t, state)
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{replies: replies()})
		if err := runner.Resume(context.Background(), "run-r", ResumeAction{
			Kind: ResumeResearch, ResearchChoice: &ResearchChoice{OptionID: "retry_search"},
		}); err != nil {
			t.Fatalf("resume: %v", err)
		}
		runner.Wait()
		after := store.stateOf(t, "run-r")
		if after.ResolvedHotel != nil {
			t.Fatalf("resolved hotel = %+v, want nil", after.ResolvedHotel)
		}
		if after.ResearchQuestion != nil || after.ConstraintPatch != nil {
			t.Fatalf("state = %+v, want fresh research", after)
		}
	})

	t.Run("rejects an unknown option", func(t *testing.T) {
		store := awaitingResearchStore(t, researchAwaitingState())
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{})
		err := runner.Resume(context.Background(), "run-r", ResumeAction{
			Kind: ResumeResearch, ResearchChoice: &ResearchChoice{OptionID: "hotel:missing"},
		})
		var choiceErr *ResearchChoiceError
		if !errors.As(err, &choiceErr) || choiceErr.Code != "invalid_research_choice" {
			t.Fatalf("err = %v", err)
		}
		if store.statusOf("run-r") != StatusAwaitingUser {
			t.Fatalf("status = %s, want awaiting", store.statusOf("run-r"))
		}
	})

	t.Run("rejects a hotel that differs from the option", func(t *testing.T) {
		store := awaitingResearchStore(t, researchAwaitingState())
		runner := newTestRunner(t, store, &eventRecorder{}, &fakeLLM{})
		err := runner.Resume(context.Background(), "run-r", ResumeAction{
			Kind: ResumeResearch,
			ResearchChoice: &ResearchChoice{
				OptionID: "hotel:h1",
				Hotel:    &domain.Hotel{ID: "h1", Name: "伪造酒店", Lat: 39.9, Lng: 116.4},
			},
		})
		var choiceErr *ResearchChoiceError
		if !errors.As(err, &choiceErr) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRunnerAwaitingResearch(t *testing.T) {
	llm := &fakeLLM{reply: json.RawMessage(`{"city":"北京","days":1,"lodging":{"mentioned":true,"hotels":["华尔道夫"]}}`)}
	hotelTool := &fakeResearchTool{name: researchHotelTool, fn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return hotelToolOutput(t, domain.Hotel{ID: "h9", Name: "全季酒店", PricePerNightYuan: 300}), nil
	}}
	toolLLM := &fakeToolLLM{responses: []ToolCompletion{{Content: briefJSON(t, `{"status":"ok"}`)}}}
	store := newFakeRunStore(runnerRun("run-q", StatusQueued, "北京一天，住华尔道夫"))
	events := &eventRecorder{}
	nodes := graphNodesWithOptions(llm, events, WithToolLLM(toolLLM), WithResearchTools(hotelTool))
	runner, err := NewRunner(context.Background(), store, events, nodes)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if err := runner.Start(context.Background(), "run-q"); err != nil {
		t.Fatalf("start: %v", err)
	}
	runner.Wait()

	if got := store.statusOf("run-q"); got != StatusAwaitingUser {
		t.Fatalf("status = %s", got)
	}
	state := store.stateOf(t, "run-q")
	if state.ResearchQuestion == nil || state.ResearchQuestion.Code != "hotel_unconfirmed" {
		t.Fatalf("question = %+v", state.ResearchQuestion)
	}
	if state.Snapshot != nil {
		t.Fatalf("snapshot = %+v, want none", state.Snapshot)
	}
	if len(store.versions) != 0 {
		t.Fatalf("versions = %d, want 0", len(store.versions))
	}
	var awaiting map[string]any
	for _, event := range events.events {
		if event.typ == "run.awaiting_user" {
			awaiting, _ = event.payload.(map[string]any)
		}
	}
	if awaiting == nil {
		t.Fatalf("awaiting payload missing: %v", events.types())
	}
	if awaiting["stage"] != "research" {
		t.Fatalf("stage = %v, want research", awaiting["stage"])
	}
	question, ok := awaiting["research_question"].(*domain.ResearchQuestion)
	if !ok || question.Code != "hotel_unconfirmed" {
		t.Fatalf("research_question = %T %+v", awaiting["research_question"], awaiting["research_question"])
	}
}

func TestRunnerReplan(t *testing.T) {
	previous := graphPlan()
	raw, err := (&TripState{
		RunID:           "run-replan",
		TripID:          "trip-1",
		Mode:            "replan",
		Input:           "按新的约束重排",
		Status:          StatusQueued,
		ConstraintPatch: &domain.ConstraintSet{City: "beijing", Days: 1},
		Previous:        &previous,
		PreviousSeq:     2,
	}).Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	store := newFakeRunStore(RunRecord{
		ID: "run-replan", TripID: "trip-1", Mode: "replan", Status: StatusQueued, Input: "按新的约束重排", State: raw,
	})
	events := &eventRecorder{}
	llm := &fakeLLM{replies: []json.RawMessage{
		json.RawMessage(`{"city":"","days":0}`),
		graphDraftReply(),
	}}
	runner := newTestRunner(t, store, events, llm)
	if err := runner.Start(context.Background(), "run-replan"); err != nil {
		t.Fatalf("start: %v", err)
	}
	runner.Wait()

	if got := store.statusOf("run-replan"); got != StatusAwaitingUser {
		t.Fatalf("status = %s", got)
	}
	state := store.stateOf(t, "run-replan")
	if state.Constraints.City != "beijing" || state.Constraints.Days != 1 {
		t.Fatalf("constraints = %+v", state.Constraints)
	}
	if state.Snapshot == nil || state.Snapshot.Seq != 3 || state.Snapshot.Reason != "replan" {
		t.Fatalf("snapshot = %+v", state.Snapshot)
	}
	if state.Proposal != nil {
		t.Fatalf("proposal = %+v, want none", state.Proposal)
	}
	if state.Previous == nil {
		t.Fatal("previous version lost")
	}
	if len(store.versions) != 1 || store.versions[0].status != "draft" {
		t.Fatalf("versions = %+v", store.versions)
	}
	if !containsString(events.types(), "run.started") {
		t.Fatalf("events = %v", events.types())
	}
}
