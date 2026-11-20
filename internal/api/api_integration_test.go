package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
	"trippilot/internal/events"
	"trippilot/internal/repo"
)

type integrationLLM struct {
	mu      sync.Mutex
	replies []json.RawMessage
}

func (l *integrationLLM) GenerateJSON(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, agent.Usage, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.replies) == 0 {
		return nil, agent.Usage{}, fmt.Errorf("no queued llm reply")
	}
	reply := l.replies[0]
	l.replies = l.replies[1:]
	return reply, agent.Usage{}, nil
}

type integrationSource struct {
	pois []domain.POI
}

func (s *integrationSource) SearchCandidates(_ context.Context, query domain.POIQuery) ([]domain.POI, error) {
	out := make([]domain.POI, 0, len(s.pois))
	for _, poi := range s.pois {
		if query.City == "" || poi.City == query.City {
			out = append(out, poi)
		}
	}
	return out, nil
}

func (s *integrationSource) Get(id string) (domain.POI, bool) {
	for _, poi := range s.pois {
		if poi.ID == id {
			return poi, true
		}
	}
	return domain.POI{}, false
}

type integrationValidator struct{}

func (integrationValidator) Validate(domain.Itinerary, domain.ConstraintSet, map[string]domain.POI, domain.RouteProvider) []domain.Violation {
	return nil
}

type runStoreAdapter struct {
	store *fakeTripStore
}

func (a runStoreAdapter) GetRun(_ context.Context, id string) (agent.RunRecord, error) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	run, found := a.store.findRun(id)
	if !found {
		return agent.RunRecord{}, pgx.ErrNoRows
	}
	return agent.RunRecord{
		ID:          run.ID,
		TripID:      run.TripID,
		Mode:        run.Mode,
		Status:      run.Status,
		Stage:       run.Stage,
		Input:       run.Input,
		Constraints: run.Constraints,
		State:       run.State,
	}, nil
}

func (a runStoreAdapter) ListRunsByStatus(_ context.Context, statuses []string) ([]agent.RunRecord, error) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	wanted := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		wanted[status] = struct{}{}
	}
	out := make([]agent.RunRecord, 0)
	for _, runs := range a.store.runs {
		for _, run := range runs {
			if _, ok := wanted[run.Status]; !ok {
				continue
			}
			out = append(out, agent.RunRecord{ID: run.ID, TripID: run.TripID, Mode: run.Mode, Status: run.Status, Stage: run.Stage, Input: run.Input, Constraints: run.Constraints, State: run.State})
		}
	}
	return out, nil
}

func (a runStoreAdapter) SaveRunState(_ context.Context, runID string, state json.RawMessage) error {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	for tripID, runs := range a.store.runs {
		for index := range runs {
			if runs[index].ID == runID {
				runs[index].State = state
				a.store.runs[tripID] = runs
				return nil
			}
		}
	}
	return fmt.Errorf("run %s not found", runID)
}

func (a runStoreAdapter) UpdateRunStatus(_ context.Context, runID, status, stage string) error {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	for tripID, runs := range a.store.runs {
		for index := range runs {
			if runs[index].ID != runID {
				continue
			}
			runs[index].Status = status
			runs[index].Stage = stage
			if terminalRunStatus(status) {
				now := time.Unix(1700000100, 0).UTC()
				runs[index].FinishedAt = &now
			} else if status == agent.StatusRunning && runs[index].StartedAt == nil {
				now := time.Unix(1700000001, 0).UTC()
				runs[index].StartedAt = &now
			}
			a.store.runs[tripID] = runs
			return nil
		}
	}
	return fmt.Errorf("run %s not found", runID)
}

func (a runStoreAdapter) SaveVersion(_ context.Context, runID string, snapshot agent.VersionSnapshot, status string) error {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	run, found := a.store.findRun(runID)
	if !found {
		return fmt.Errorf("run %s not found", runID)
	}
	plan, _ := json.Marshal(snapshot.Plan)
	constraints, _ := json.Marshal(snapshot.Constraints)
	violations, _ := json.Marshal(snapshot.Violations)
	research := json.RawMessage(`{}`)
	if snapshot.Research != nil {
		if encoded, err := json.Marshal(snapshot.Research); err == nil {
			research = encoded
		}
	}
	version := repo.PlanVersion{
		ID:          fmt.Sprintf("%s-v%d", run.TripID, snapshot.Seq),
		TripID:      run.TripID,
		RunID:       run.ID,
		Seq:         int32(snapshot.Seq),
		Plan:        plan,
		Constraints: constraints,
		Violations:  violations,
		Status:      status,
		CreatedAt:   time.Unix(1700000200, 0).UTC(),
		Research:    research,
	}
	a.store.versions[run.TripID] = append([]repo.PlanVersion{version}, a.store.versions[run.TripID]...)
	return nil
}

func (a runStoreAdapter) AcceptVersion(_ context.Context, runID string, seq int) error {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	run, found := a.store.findRun(runID)
	if !found {
		return fmt.Errorf("run %s not found", runID)
	}
	versions := a.store.versions[run.TripID]
	for index := range versions {
		if int(versions[index].Seq) == seq {
			versions[index].Status = "accepted"
		} else if versions[index].Status == "accepted" {
			versions[index].Status = "superseded"
		}
	}
	a.store.versions[run.TripID] = versions
	return nil
}

func (s *fakeTripStore) findRun(runID string) (repo.Run, bool) {
	for _, runs := range s.runs {
		for _, run := range runs {
			if run.ID == runID {
				return run, true
			}
		}
	}
	return repo.Run{}, false
}

func waitRun(t *testing.T, store *fakeTripStore, runID, status, stage string) repo.Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		run, found := store.findRun(runID)
		store.mu.Unlock()
		if found && run.Status == status && (stage == "" || run.Stage == stage) {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	store.mu.Lock()
	run, found := store.findRun(runID)
	store.mu.Unlock()
	t.Fatalf("run %s did not reach %s/%s: found=%v run=%+v", runID, status, stage, found, run)
	return repo.Run{}
}

func integrationRouter(t *testing.T) (http.Handler, *fakeTripStore, *events.Broker, *integrationLLM) {
	t.Helper()
	ticket := 20
	pois := []domain.POI{
		{ID: "poi-0", City: "beijing", Name: "景点 0", Category: "历史", TicketYuan: &ticket, Lat: 39.9, Lng: 116.4},
		{ID: "poi-1", City: "beijing", Name: "景点 1", Category: "历史", TicketYuan: &ticket, Lat: 39.91, Lng: 116.41},
		{ID: "poi-2", City: "beijing", Name: "景点 2", Category: "历史", TicketYuan: &ticket, Lat: 39.92, Lng: 116.42},
	}
	source := &integrationSource{pois: pois}
	llm := &integrationLLM{replies: []json.RawMessage{
		json.RawMessage(`{"city":"北京","days":2}`),
		json.RawMessage(`{"days":[{"date":"2026-11-20","items":[{"poi_id":"poi-0","type":"poi","duration_min":90}]},{"date":"2026-11-21","items":[{"poi_id":"poi-1","type":"poi","duration_min":90}]}]}`),
		json.RawMessage(`{"scope":"day","affected_days":[0],"summary":"换掉第一天"}`),
		json.RawMessage(`{"day_index":0,"items":[{"poi_id":"poi-2","type":"poi","duration_min":90}],"reason":"换成景点 2"}`),
		json.RawMessage(`{"scope":"day","affected_days":[0],"summary":"换回第一天"}`),
		json.RawMessage(`{"day_index":0,"items":[{"poi_id":"poi-0","type":"poi","duration_min":90}],"reason":"换成景点 0"}`),
	}}

	store := newFakeTripStore()
	eventStore := &fakeEventStore{}
	broker, err := events.NewBroker(eventStore)
	if err != nil {
		t.Fatalf("new broker: %v", err)
	}
	t.Cleanup(broker.Close)

	nodes := agent.NewNodes(llm, broker,
		agent.WithSnapshotSource(source),
		agent.WithPOILookup(source),
		agent.WithValidator(integrationValidator{}),
		agent.WithNodeLogger(discardLogger()),
	)
	runner, err := agent.NewRunner(context.Background(), runStoreAdapter{store: store}, broker, nodes,
		agent.WithRunnerConcurrency(1),
		agent.WithRunnerLogger(discardLogger()),
	)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	t.Cleanup(runner.Wait)

	router := baseRouter(Deps{
		AdminToken: "secret",
		Store:      store,
		Runner:     runner,
		Broker:     broker,
	})
	return router, store, broker, llm
}

func TestAPIIntegration(t *testing.T) {
	router, store, _, _ := integrationRouter(t)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips", strings.NewReader(`{"input":"北京两天，预算800元"}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created createTripResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	run := waitRun(t, store, created.RunID, agent.StatusAwaitingUser, "present")
	if len(run.State) == 0 {
		t.Fatal("run state was not persisted")
	}

	resp, reader := openStream(t, server, "/api/runs/"+created.RunID+"/events?after=0", nil)
	seen := make(map[string]bool)
	for i := 0; i < 12 && !seen["run.awaiting_user"]; i++ {
		_, typ := readSSEEvent(t, reader)
		seen[typ] = true
	}
	resp.Body.Close()
	for _, want := range []string{"run.started", "constraint.parsed", "plan.updated", "run.awaiting_user"} {
		if !seen[want] {
			t.Fatalf("missing event %s in %v", want, seen)
		}
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+created.RunID+"/resume", strings.NewReader(`{"action":"accept"}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("accept status = %d body=%s", rec.Code, rec.Body.String())
	}
	waitRun(t, store, created.RunID, agent.StatusCompleted, "accepted")

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+created.TripID+"/share", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("share status = %d body=%s", rec.Code, rec.Body.String())
	}
	var shared shareResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &shared); err != nil {
		t.Fatalf("decode share: %v", err)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, shared.URL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get share status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	body := `{"input":"第一天换个景点","base_version":1}`
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+created.TripID+"/adjustments", strings.NewReader(body)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("adjustment status = %d body=%s", rec.Code, rec.Body.String())
	}
	var adjustment adjustmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &adjustment); err != nil {
		t.Fatalf("decode adjustment: %v", err)
	}
	waitRun(t, store, adjustment.RunID, agent.StatusAwaitingUser, "proposal")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+adjustment.RunID+"/resume", strings.NewReader(`{"action":"accept"}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("adjustment accept status = %d body=%s", rec.Code, rec.Body.String())
	}
	waitRun(t, store, adjustment.RunID, agent.StatusCompleted, "accepted")

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+created.TripID+"/versions", nil))
	store.mu.Lock()
	versionCount := len(store.versions[created.TripID])
	accepted := 0
	for _, version := range store.versions[created.TripID] {
		if version.Status == "accepted" {
			accepted++
		}
	}
	store.mu.Unlock()
	if rec.Code != http.StatusOK || versionCount != 2 || accepted != 1 {
		t.Fatalf("versions status=%d count=%d accepted=%d body=%s", rec.Code, versionCount, accepted, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	body = `{"input":"再换回原景点","base_version":2}`
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+created.TripID+"/adjustments", strings.NewReader(body)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("second adjustment status = %d body=%s", rec.Code, rec.Body.String())
	}
	var rejected adjustmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &rejected); err != nil {
		t.Fatalf("decode rejected adjustment: %v", err)
	}
	waitRun(t, store, rejected.RunID, agent.StatusAwaitingUser, "proposal")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+rejected.RunID+"/resume", strings.NewReader(`{"action":"reject"}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("reject status = %d body=%s", rec.Code, rec.Body.String())
	}
	waitRun(t, store, rejected.RunID, agent.StatusCancelled, "rejected")

	store.mu.Lock()
	versionCount = len(store.versions[created.TripID])
	accepted = 0
	for _, version := range store.versions[created.TripID] {
		if version.Status == "accepted" {
			accepted++
		}
	}
	store.mu.Unlock()
	if versionCount != 2 || accepted != 1 {
		t.Fatalf("reject changed versions: count=%d accepted=%d", versionCount, accepted)
	}
}
