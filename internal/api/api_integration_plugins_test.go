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

	"trippilot/internal/agent"
	"trippilot/internal/domain"
	"trippilot/internal/events"
	"trippilot/internal/plugins"
)

type pluginTool struct {
	mu      sync.Mutex
	outputs []json.RawMessage
	calls   int
}

func (t *pluginTool) Name() string { return "search_hotels" }

func (t *pluginTool) Description() string { return "fake hotel search" }

func (t *pluginTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }

func (t *pluginTool) Call(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.outputs) == 0 {
		return json.RawMessage(`{"hotels":[]}`), nil
	}
	index := t.calls
	if index >= len(t.outputs) {
		index = len(t.outputs) - 1
	}
	t.calls++
	return t.outputs[index], nil
}

type pluginToolLLM struct {
	mu      sync.Mutex
	turns   int
	brief   string
	baseURL string
}

func (l *pluginToolLLM) CompleteWithTools(context.Context, agent.ToolCompletionRequest) (agent.ToolCompletion, agent.Usage, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.turns++
	if l.turns%2 == 1 {
		arguments := json.RawMessage(`{"city":"beijing","check_in":"2026-11-20","check_out":"2026-11-22"}`)
		return agent.ToolCompletion{ToolCalls: []agent.ToolCall{{
			ID:        fmt.Sprintf("call-%d", l.turns),
			Name:      "search_hotels",
			Arguments: arguments,
		}}}, agent.Usage{}, nil
	}
	return agent.ToolCompletion{Content: l.brief}, agent.Usage{}, nil
}

func pluginIntegrationRouter(t *testing.T, replies []json.RawMessage, tool agent.ResearchTool, toolLLM agent.ToolLLM) (http.Handler, *fakeTripStore, *events.Broker) {
	t.Helper()
	ticket := 20
	source := &integrationSource{pois: []domain.POI{
		{ID: "poi-0", City: "beijing", Name: "景点 0", Category: "历史", TicketYuan: &ticket, Lat: 39.9, Lng: 116.4},
		{ID: "poi-1", City: "beijing", Name: "景点 1", Category: "历史", TicketYuan: &ticket, Lat: 39.91, Lng: 116.41},
	}}
	store := newFakeTripStore()
	eventStore := &fakeEventStore{}
	broker, err := events.NewBroker(eventStore)
	if err != nil {
		t.Fatalf("new broker: %v", err)
	}
	t.Cleanup(broker.Close)

	governor := plugins.NewGovernor(nil, nil, nil, broker, plugins.WithGovernorLogger(discardLogger()))
	nodes := agent.NewNodes(&integrationLLM{replies: replies}, broker,
		agent.WithSnapshotSource(source),
		agent.WithPOILookup(source),
		agent.WithValidator(integrationValidator{}),
		agent.WithNodeLogger(discardLogger()),
		agent.WithToolLLM(toolLLM),
		agent.WithResearchTools(governor.Wrap(tool)),
		agent.WithResearchContext(plugins.WithRun),
		agent.WithResearchLimits(5*time.Second, 4),
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
		PluginStatuses: []plugins.Status{
			{Name: "hotel", Enabled: true},
			{Name: "guide", Enabled: false, DisabledReason: "GUIDE_SEARCH_API_KEY is empty"},
		},
	})
	return router, store, broker
}

func pluginDraftReply() json.RawMessage {
	return json.RawMessage(`{"days":[{"date":"2026-11-20","items":[{"poi_id":"poi-0","type":"poi","duration_min":90}]},{"date":"2026-11-21","items":[{"poi_id":"poi-1","type":"poi","duration_min":90}]}]}`)
}

func readEventTypes(t *testing.T, router http.Handler, runID string, limit int) []string {
	t.Helper()
	server := httptest.NewServer(router)
	defer server.Close()
	resp, reader := openStream(t, server, "/api/runs/"+runID+"/events?after=0", nil)
	defer resp.Body.Close()
	types := make([]string, 0, limit)
	for len(types) < limit {
		_, typ := readSSEEvent(t, reader)
		types = append(types, typ)
		switch typ {
		case "run.awaiting_user", "run.failed", "run.cancelled":
			return types
		}
	}
	return types
}

func eventIndex(types []string, want string) int {
	for index, typ := range types {
		if typ == want {
			return index
		}
	}
	return -1
}

func TestAPIIntegrationPlugins(t *testing.T) {
	t.Run("replan exposes research and tool events", func(t *testing.T) {
		hotel := &pluginTool{outputs: []json.RawMessage{
			json.RawMessage(`{"hotels":[{"id":"h1","name":"测试酒店","lat":39.8,"lng":116.3,"price_per_night_yuan":500,"provider":"fake"}]}`),
		}}
		brief := `{"status":"ok","accommodation":{"mentioned":true,"selected":{"id":"h1","name":"测试酒店","lat":39.8,"lng":116.3,"price_per_night_yuan":500,"provider":"fake"},"anchor_days":[0]},"guides":[]}`
		toolLLM := &pluginToolLLM{brief: brief}
		router, store, _ := pluginIntegrationRouter(t, []json.RawMessage{
			json.RawMessage(`{"city":"北京","days":2,"lodging":{"mentioned":true}}`),
			pluginDraftReply(),
			json.RawMessage(`{"city":"北京","days":2,"lodging":{"mentioned":true}}`),
			pluginDraftReply(),
		}, hotel, toolLLM)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips", strings.NewReader(`{"input":"北京两天，预算2000含住宿"}`)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
		}
		var created createTripResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("decode create: %v", err)
		}
		waitRun(t, store, created.RunID, agent.StatusAwaitingUser, "present")

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+created.RunID+"/resume", strings.NewReader(`{"action":"accept"}`)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("accept status = %d body=%s", rec.Code, rec.Body.String())
		}
		waitRun(t, store, created.RunID, agent.StatusCompleted, "accepted")

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+created.TripID+"/versions", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("versions status = %d body=%s", rec.Code, rec.Body.String())
		}
		var listed struct {
			Versions []repoVersion `json:"versions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
			t.Fatalf("decode versions: %v", err)
		}
		if len(listed.Versions) == 0 || !strings.Contains(string(listed.Versions[0].Research), "测试酒店") {
			t.Fatalf("accepted version research = %+v", listed.Versions)
		}

		body := `{"base_version":1,"constraints":{"city":"beijing","days":2,"budget":{"total_yuan":2000,"include_lodging":false},"lodging":{"mentioned":true}},"client_request_id":"replan-int"}`
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+created.TripID+"/replan", strings.NewReader(body)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("replan status = %d body=%s", rec.Code, rec.Body.String())
		}
		var replan adjustmentResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &replan); err != nil {
			t.Fatalf("decode replan: %v", err)
		}
		waitRun(t, store, replan.RunID, agent.StatusAwaitingUser, "present")

		store.mu.Lock()
		run, _ := store.findRun(replan.RunID)
		store.mu.Unlock()
		if run.Mode != "replan" {
			t.Fatalf("replan mode = %s", run.Mode)
		}

		types := readEventTypes(t, router, replan.RunID, 40)
		for _, want := range []string{"research.started", "plugin.tool_called", "plugin.tool_completed", "research.completed", "plan.updated", "run.awaiting_user"} {
			if eventIndex(types, want) < 0 {
				t.Fatalf("missing %s in %v", want, types)
			}
		}
		if !(eventIndex(types, "research.started") < eventIndex(types, "plugin.tool_called") &&
			eventIndex(types, "plugin.tool_called") < eventIndex(types, "plugin.tool_completed") &&
			eventIndex(types, "plugin.tool_completed") < eventIndex(types, "research.completed") &&
			eventIndex(types, "research.completed") < eventIndex(types, "plan.updated")) {
			t.Fatalf("event order = %v", types)
		}

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
		meta := rec.Body.String()
		if !strings.Contains(meta, `"plugins":{"hotels":{"enabled":true},"guides":{"enabled":false,"disabled_reason":"GUIDE_SEARCH_API_KEY is empty"}}`) {
			t.Fatalf("meta = %s", meta)
		}

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+created.TripID+"/replan", strings.NewReader(body)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("replan retry status = %d body=%s", rec.Code, rec.Body.String())
		}
		var retried adjustmentResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &retried); err != nil {
			t.Fatalf("decode retried replan: %v", err)
		}
		if retried.RunID != replan.RunID {
			t.Fatalf("idempotent replan started %s, want %s", retried.RunID, replan.RunID)
		}

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+created.TripID+"/replan", strings.NewReader(`{"base_version":99,"constraints":{"city":"beijing","days":2}}`)))
		bodyErr := decodeError(t, rec)
		if rec.Code != http.StatusNotFound || bodyErr.Code != "version_not_found" || bodyErr.RequestID == "" {
			t.Fatalf("error = %d %+v", rec.Code, bodyErr)
		}

		rec = httptest.NewRecorder()
		badID := fmt.Sprintf(`{"base_version":1,"constraints":{"city":"beijing","days":2},"client_request_id":"%s"}`, strings.Repeat("a", 200))
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips/"+created.TripID+"/replan", strings.NewReader(badID)))
		bodyErr = decodeError(t, rec)
		if rec.Code != http.StatusBadRequest || bodyErr.Code != "invalid_request_id" || bodyErr.RequestID == "" {
			t.Fatalf("invalid request id = %d %+v", rec.Code, bodyErr)
		}
	})

	t.Run("research question resumes without lodging", func(t *testing.T) {
		hotel := &pluginTool{outputs: []json.RawMessage{json.RawMessage(`{"hotels":[]}`)}}
		toolLLM := &pluginToolLLM{brief: `{"status":"ok"}`}
		parse := json.RawMessage(`{"city":"北京","days":2,"lodging":{"mentioned":true,"hotels":["全季"]}}`)
		router, store, _ := pluginIntegrationRouter(t, []json.RawMessage{
			parse,
			parse,
			pluginDraftReply(),
		}, hotel, toolLLM)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/trips", strings.NewReader(`{"input":"北京两天，住全季"}`)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
		}
		var created createTripResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("decode create: %v", err)
		}
		waitRun(t, store, created.RunID, agent.StatusAwaitingUser, "research")

		types := readEventTypes(t, router, created.RunID, 30)
		if eventIndex(types, "research.needs_input") < 0 {
			t.Fatalf("missing research.needs_input in %v", types)
		}

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+created.RunID+"/resume", strings.NewReader(
			`{"action":"resolve_research","research_choice":{"option_id":"missing"}}`,
		)))
		bodyErr := decodeError(t, rec)
		if rec.Code != http.StatusBadRequest || bodyErr.Code != "invalid_research_choice" || bodyErr.RequestID == "" {
			t.Fatalf("unknown option = %d %+v", rec.Code, bodyErr)
		}

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/runs/"+created.RunID+"/resume", strings.NewReader(
			`{"action":"resolve_research","research_choice":{"option_id":"continue_without_lodging"}}`,
		)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("resolve status = %d body=%s", rec.Code, rec.Body.String())
		}
		waitRun(t, store, created.RunID, agent.StatusAwaitingUser, "present")

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trips/"+created.TripID+"/versions", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("versions status = %d body=%s", rec.Code, rec.Body.String())
		}
		var listed struct {
			Versions []repoVersion `json:"versions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
			t.Fatalf("decode versions: %v", err)
		}
		if len(listed.Versions) == 0 || strings.Contains(string(listed.Versions[0].Research), `"accommodation"`) {
			t.Fatalf("lodging leaked after opting out: %+v", listed.Versions)
		}
	})
}

// repoVersion mirrors the fields the plugin integration assertions read.
type repoVersion struct {
	Seq      int32           `json:"seq"`
	Research json.RawMessage `json:"research"`
}
