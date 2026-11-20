package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"trippilot/internal/agent"
)

func runCtx() context.Context {
	return WithRun(context.Background(), "run-1", "trip-1")
}

func newTestGovernor(calls ToolCallStore, evidence EvidenceStore, events EventPublisher, cache Cache, opts ...GovernorOption) *Governor {
	return NewGovernor(cache, calls, evidence, events, append([]GovernorOption{WithGovernorLogger(cacheLogger())}, opts...)...)
}

func codeOf(payload json.RawMessage) string {
	var decoded struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(payload, &decoded) != nil {
		return ""
	}
	return decoded.Error.Code
}

func decodeCode(t *testing.T, payload json.RawMessage) string {
	t.Helper()
	var decoded struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode envelope %s: %v", payload, err)
	}
	return decoded.Error.Code
}

func TestGovernorMissingRunContext(t *testing.T) {
	tool := &stubTool{name: "search_hotels"}
	governor := newTestGovernor(nil, nil, nil, nil)
	payload, err := governor.Wrap(tool).Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if code := decodeCode(t, payload); code != CodeMissingRun {
		t.Fatalf("code = %s payload=%s", code, payload)
	}
	if tool.Calls() != 0 {
		t.Fatal("tool must not be called without a run context")
	}
}

func TestGovernorPersists(t *testing.T) {
	calls := &fakeCallStore{}
	evidence := &fakeEvidenceStore{}
	events := &fakePublisher{}
	tool := &stubTool{
		name:     "search_hotels",
		provider: "fliggy",
		call: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"items":[{"id":"h1"}]}`), nil
		},
		evidence: []EvidenceRecord{{
			Kind: "hotel", Claim: "示例酒店", Source: "fliggy", SourceURL: "https://example.com/h1",
		}},
		querySummary:  "北京 · 2 晚",
		resultPreview: []EventPreview{{Title: "示例酒店", Subtitle: "4 星", PriceYuan: 600}},
	}
	governor := newTestGovernor(calls, evidence, events, nil)
	payload, err := governor.Wrap(tool).Call(runCtx(), json.RawMessage(`{"city":"北京"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if string(payload) != `{"items":[{"id":"h1"}]}` {
		t.Fatalf("payload = %s", payload)
	}

	stored := calls.snapshot()
	if len(stored) != 1 {
		t.Fatalf("calls = %+v", stored)
	}
	if stored[0].RunID != "run-1" || stored[0].TripID != "trip-1" {
		t.Fatalf("run context = %+v", stored[0])
	}
	if stored[0].Provider != "plugin" || stored[0].Tool != "search_hotels" || stored[0].Status != "ok" {
		t.Fatalf("stored = %+v", stored[0])
	}
	if stored[0].LatencyMs == nil || stored[0].Error != nil {
		t.Fatalf("stored = %+v", stored[0])
	}

	records := evidence.snapshot()
	if len(records) != 1 || records[0].Kind != "hotel" || records[0].TripID != "trip-1" {
		t.Fatalf("evidence = %+v", records)
	}
	if records[0].RunID == nil || *records[0].RunID != "run-1" {
		t.Fatalf("evidence run = %+v", records[0])
	}
	if records[0].SourceUrl == nil || *records[0].SourceUrl != "https://example.com/h1" {
		t.Fatalf("evidence url = %+v", records[0])
	}

	eventTypes := events.list()
	if len(eventTypes) != 2 || eventTypes[0] != "plugin.tool_called" || eventTypes[1] != "plugin.tool_completed" {
		t.Fatalf("events = %+v", eventTypes)
	}
	started, _ := events.items[0].(toolEvent)
	if started.Query != "北京 · 2 晚" {
		t.Fatalf("started event = %+v", started)
	}
	completed, _ := events.items[1].(toolEvent)
	if completed.Name != "search_hotels" || completed.Provider != "fliggy" || completed.Status != "ok" || completed.ResultCount != 1 {
		t.Fatalf("completed event = %+v", completed)
	}
	if completed.Query != "北京 · 2 晚" || len(completed.Preview) != 1 || completed.Preview[0].Title != "示例酒店" {
		t.Fatalf("completed preview = %+v", completed)
	}
}

func TestGovernorFailureKeepsQueryWithoutPreview(t *testing.T) {
	events := &fakePublisher{}
	tool := &stubTool{
		name:          "search_hotels",
		querySummary:  "北京 · 2 晚",
		resultPreview: []EventPreview{{Title: "不应出现"}},
		call: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return nil, &ToolError{Code: CodeUpstreamFailure, Message: "upstream is unavailable"}
		},
	}
	governor := newTestGovernor(nil, nil, events, nil)
	if _, err := governor.Wrap(tool).Call(runCtx(), json.RawMessage(`{"city":"北京"}`)); err != nil {
		t.Fatalf("call: %v", err)
	}
	completed, _ := events.items[1].(toolEvent)
	if completed.Status != "error" || completed.Query != "北京 · 2 晚" {
		t.Fatalf("completed = %+v", completed)
	}
	if len(completed.Preview) != 0 {
		t.Fatalf("failure preview = %+v", completed.Preview)
	}
}

func TestGovernorRetry(t *testing.T) {
	calls := &fakeCallStore{}
	var attempts int
	secret := "super-secret-token"
	tool := &stubTool{
		name: "search_hotels",
		call: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			attempts++
			if attempts == 1 {
				return nil, &ToolError{Code: CodeUpstreamFailure, Message: "upstream is unavailable", Retryable: true}
			}
			return json.RawMessage(`{"ok":true}`), nil
		},
	}
	governor := newTestGovernor(calls, nil, nil, nil, WithGovernorRetryDelay(0))
	payload, err := governor.Wrap(tool).Call(runCtx(), nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if string(payload) != `{"ok":true}` || attempts != 2 {
		t.Fatalf("payload=%s attempts=%d", payload, attempts)
	}
	stored := calls.snapshot()
	if len(stored) != 1 || stored[0].Status != "ok" || stored[0].Error != nil {
		t.Fatalf("stored = %+v", stored)
	}
	if strings.Contains(string(payload), secret) {
		t.Fatalf("post-call payload leaked secret: %s", payload)
	}

	attempts = 0
	authTool := &stubTool{
		name: "search_hotels",
		call: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			attempts++
			return nil, &ToolError{Code: CodeAuthFailed, Message: "credentials were rejected"}
		},
	}
	authGovernor := newTestGovernor(nil, nil, nil, nil)
	payload, _ = authGovernor.Wrap(authTool).Call(runCtx(), nil)
	if decodeCode(t, payload) != CodeAuthFailed || attempts != 1 {
		t.Fatalf("auth payload=%s attempts=%d", payload, attempts)
	}
}

func TestGovernorTimeout(t *testing.T) {
	tool := &stubTool{
		name: "search_hotels",
		call: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	governor := newTestGovernor(nil, nil, nil, nil, WithGovernorTimeout(25*time.Millisecond))
	start := time.Now()
	payload, err := governor.Wrap(tool).Call(runCtx(), nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if code := decodeCode(t, payload); code != CodeTimeout {
		t.Fatalf("code = %s", code)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("elapsed = %v", elapsed)
	}
	if tool.Calls() != 1 {
		t.Fatalf("calls = %d", tool.Calls())
	}
}

func TestGovernorCallLimit(t *testing.T) {
	governor := newTestGovernor(nil, nil, nil, nil)
	ctx := runCtx()
	hotel := governor.Wrap(&stubTool{name: "search_hotels"})
	guide := governor.Wrap(&stubTool{name: "search_travel_notes"})
	for i := 0; i < 2; i++ {
		if payload, _ := hotel.Call(ctx, nil); decodeCode(t, payload) != "" {
			t.Fatalf("hotel call %d rejected: %s", i+1, payload)
		}
	}
	payload, _ := hotel.Call(ctx, nil)
	if decodeCode(t, payload) != CodeToolCallLimit || !strings.Contains(string(payload), "tool_limit") {
		t.Fatalf("hotel third = %s", payload)
	}
	for i := 0; i < 3; i++ {
		if payload, _ := guide.Call(ctx, nil); decodeCode(t, payload) != "" {
			t.Fatalf("guide call %d rejected: %s", i+1, payload)
		}
	}
	payload, _ = guide.Call(ctx, nil)
	if decodeCode(t, payload) != CodeToolCallLimit || !strings.Contains(string(payload), "tool_limit") {
		t.Fatalf("guide fourth = %s", payload)
	}

	otherRun := WithRun(context.Background(), "run-2", "trip-2")
	if payload, _ := hotel.Call(otherRun, nil); decodeCode(t, payload) != "" {
		t.Fatalf("other run rejected: %s", payload)
	}
}

func TestGovernorRunLimit(t *testing.T) {
	governor := newTestGovernor(nil, nil, nil, nil, WithGovernorMaxCalls(2))
	tool := governor.Wrap(&stubTool{name: "custom"})
	ctx := runCtx()
	for i := 0; i < 2; i++ {
		if payload, _ := tool.Call(ctx, nil); decodeCode(t, payload) != "" {
			t.Fatalf("call %d rejected: %s", i+1, payload)
		}
	}
	payload, _ := tool.Call(ctx, nil)
	if decodeCode(t, payload) != CodeToolCallLimit || !strings.Contains(string(payload), "run_limit") {
		t.Fatalf("third = %s", payload)
	}
}

func TestGovernorConcurrentLimits(t *testing.T) {
	governor := newTestGovernor(nil, nil, nil, nil, WithGovernorMaxCalls(6))
	tool := governor.Wrap(&stubTool{name: "custom"})
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	limited := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload, _ := tool.Call(runCtx(), nil)
			mu.Lock()
			defer mu.Unlock()
			if codeOf(payload) == CodeToolCallLimit {
				limited++
			} else {
				allowed++
			}
		}()
	}
	wg.Wait()
	if allowed != 6 || limited != 14 {
		t.Fatalf("allowed=%d limited=%d", allowed, limited)
	}
}

func TestGovernorCache(t *testing.T) {
	calls := &fakeCallStore{}
	events := &fakePublisher{}
	cache := &fakeCache{values: map[string]json.RawMessage{"hotel:key": json.RawMessage(`{"items":[1]}`)}}
	tool := &stubTool{
		name: "search_hotels", cacheKey: "hotel:key", cacheTTL: time.Minute,
		querySummary:  "北京 · 1 晚",
		resultPreview: []EventPreview{{Title: "缓存酒店", PriceYuan: 500}},
	}
	governor := newTestGovernor(calls, nil, events, cache)
	payload, err := governor.Wrap(tool).Call(runCtx(), json.RawMessage(`{"city":"北京"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if string(payload) != `{"items":[1]}` || tool.Calls() != 0 {
		t.Fatalf("payload=%s calls=%d", payload, tool.Calls())
	}
	stored := calls.snapshot()
	if len(stored) != 1 || stored[0].Status != "ok" || !strings.Contains(string(stored[0].Output), `"cached":true`) {
		t.Fatalf("stored = %+v", stored)
	}
	completed, _ := events.items[1].(toolEvent)
	if !completed.Cached || completed.ResultCount != 1 {
		t.Fatalf("completed = %+v", completed)
	}
	if completed.Query != "北京 · 1 晚" || len(completed.Preview) != 1 || completed.Preview[0].Title != "缓存酒店" {
		t.Fatalf("cached preview = %+v", completed)
	}

	missCache := &fakeCache{}
	missCalls := &fakeCallStore{}
	missTool := &stubTool{
		name: "search_hotels", cacheKey: "hotel:key", cacheTTL: 30 * time.Minute,
		call: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"items":[1]}`), nil
		},
	}
	missGovernor := newTestGovernor(missCalls, nil, nil, missCache)
	if _, err := missGovernor.Wrap(missTool).Call(runCtx(), nil); err != nil {
		t.Fatalf("miss call: %v", err)
	}
	if missCache.sets != 1 || missCache.lastTTL != 30*time.Minute || missTool.Calls() != 1 {
		t.Fatalf("cache sets=%d ttl=%v calls=%d", missCache.sets, missCache.lastTTL, missTool.Calls())
	}
	if len(missCalls.snapshot()) != 1 || strings.Contains(string(missCalls.snapshot()[0].Output), "cached") {
		t.Fatalf("miss stored = %+v", missCalls.snapshot())
	}
}

func TestGovernorQuotaCache(t *testing.T) {
	cache := &fakeCache{}
	events := &fakePublisher{}
	first := &stubTool{
		name: "get_weather", cacheKey: "weather:cached", cacheTTL: time.Minute,
		call: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"days":[{"date":"2026-12-01"}]}`), nil
		},
	}
	second := &stubTool{name: "get_weather", cacheKey: "weather:other", cacheTTL: time.Minute}
	governor := newTestGovernor(nil, nil, events, cache, WithGovernorMaxCalls(1))
	wrapped := governor.Wrap(first)

	if payload, _ := wrapped.Call(runCtx(), nil); codeOf(payload) != "" {
		t.Fatalf("first call = %s", payload)
	}
	payload, _ := wrapped.Call(runCtx(), nil)
	if codeOf(payload) != "" {
		t.Fatalf("cached call = %s", payload)
	}
	if first.Calls() != 1 {
		t.Fatalf("upstream calls = %d", first.Calls())
	}
	completed, _ := events.items[len(events.items)-1].(toolEvent)
	if !completed.Cached || completed.Status != "ok" {
		t.Fatalf("completed = %+v", completed)
	}
	if payload, _ := governor.Wrap(second).Call(runCtx(), nil); codeOf(payload) != CodeToolCallLimit {
		t.Fatalf("new key should hit quota: %s", payload)
	}
}

func TestGovernorCoalesce(t *testing.T) {
	cache := &fakeCache{}
	events := &fakePublisher{}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	tool := &stubTool{
		name: "get_weather", cacheKey: "weather:coalesce", cacheTTL: time.Minute,
		querySummary:  "北京 · 1 天",
		resultPreview: []EventPreview{{Title: "12/1", Subtitle: "晴 · 6~15°C"}},
		call: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			once.Do(func() { close(started) })
			select {
			case <-release:
				return json.RawMessage(`{"days":[{"date":"2026-12-01"}]}`), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}
	governor := newTestGovernor(nil, nil, events, cache,
		WithGovernorMaxCalls(1), WithGovernorToolTimeout("get_weather", 2*time.Second))
	wrapped := governor.Wrap(tool)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []string
	call := func() {
		defer wg.Done()
		payload, err := wrapped.Call(runCtx(), json.RawMessage(`{"city":"北京"}`))
		if err != nil || codeOf(payload) != "" {
			mu.Lock()
			failures = append(failures, string(payload))
			mu.Unlock()
		}
	}

	wg.Add(1)
	go call()
	<-started
	for i := 0; i < 9; i++ {
		wg.Add(1)
		go call()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	if len(failures) > 0 {
		t.Fatalf("failures = %v", failures)
	}
	if tool.Calls() != 1 {
		t.Fatalf("upstream calls = %d", tool.Calls())
	}
	if cache.sets != 1 {
		t.Fatalf("cache sets = %d", cache.sets)
	}
	coalesced := 0
	for _, item := range events.items {
		event, ok := item.(toolEvent)
		if !ok || !event.Coalesced || event.Status != "ok" {
			continue
		}
		coalesced++
		if event.Query != "北京 · 1 天" || len(event.Preview) != 1 || event.Preview[0].Title != "12/1" {
			t.Fatalf("coalesced event = %+v", event)
		}
	}
	if coalesced == 0 {
		t.Fatal("no coalesced events published")
	}
}

func TestGovernorCoalesceFollowerCancel(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	tool := &stubTool{
		name: "get_weather", cacheKey: "weather:cancel", cacheTTL: time.Minute,
		call: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			once.Do(func() { close(started) })
			<-release
			return json.RawMessage(`{"days":[]}`), nil
		},
	}
	governor := newTestGovernor(nil, nil, nil, &fakeCache{}, WithGovernorToolTimeout("get_weather", 2*time.Second))
	wrapped := governor.Wrap(tool)

	leaderDone := make(chan json.RawMessage, 1)
	go func() {
		payload, _ := wrapped.Call(runCtx(), nil)
		leaderDone <- payload
	}()
	<-started

	followerCtx, cancel := context.WithCancel(runCtx())
	followerDone := make(chan json.RawMessage, 1)
	go func() {
		payload, _ := wrapped.Call(followerCtx, nil)
		followerDone <- payload
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case payload := <-followerDone:
		if codeOf(payload) != CodeTimeout {
			t.Fatalf("follower = %s", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("follower did not return after cancel")
	}
	select {
	case payload := <-leaderDone:
		t.Fatalf("leader finished early: %s", payload)
	default:
	}

	close(release)
	select {
	case payload := <-leaderDone:
		if codeOf(payload) != "" {
			t.Fatalf("leader = %s", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("leader did not finish")
	}
	if tool.Calls() != 1 {
		t.Fatalf("upstream calls = %d", tool.Calls())
	}
}

func TestGovernorErrorCodes(t *testing.T) {
	codes := []string{CodeAuthFailed, CodeTimeout, CodeRateLimited, CodeBadResponse, CodeEmptyResult, CodeInvalidInput}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			tool := &stubTool{
				name: "custom",
				call: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
					return nil, &ToolError{Code: code, Message: "safe message"}
				},
			}
			governor := newTestGovernor(nil, nil, nil, nil)
			payload, err := governor.Wrap(tool).Call(runCtx(), nil)
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if got := decodeCode(t, payload); got != code {
				t.Fatalf("code = %s payload=%s", got, payload)
			}
		})
	}
}

func TestGovernorDoesNotLeakUnknownError(t *testing.T) {
	secret := "sk-live-123456"
	tool := &stubTool{
		name: "custom",
		call: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
			return nil, fmt.Errorf("connect failed with %s", secret)
		},
	}
	governor := newTestGovernor(nil, nil, nil, nil)
	payload, _ := governor.Wrap(tool).Call(runCtx(), nil)
	if decodeCode(t, payload) != CodeUpstreamFailure || strings.Contains(string(payload), secret) {
		t.Fatalf("payload = %s", payload)
	}
}

func TestGovernorPersistenceFailureDoesNotBreakCall(t *testing.T) {
	calls := &fakeCallStore{fail: errors.New("db down")}
	events := &fakePublisher{fail: errors.New("broker down")}
	tool := &stubTool{name: "custom"}
	governor := newTestGovernor(calls, nil, events, nil)
	payload, err := governor.Wrap(tool).Call(runCtx(), nil)
	if err != nil || string(payload) != `{"ok":true}` {
		t.Fatalf("payload=%s err=%v", payload, err)
	}
}

func TestGovernorWrapForwardsIdentity(t *testing.T) {
	tool := &stubTool{name: "search_hotels"}
	wrapped := newTestGovernor(nil, nil, nil, nil).Wrap(tool)
	if wrapped.Name() != tool.Name() || wrapped.Description() != tool.Description() {
		t.Fatal("identity methods not forwarded")
	}
	if string(wrapped.InputSchema()) != string(tool.InputSchema()) {
		t.Fatal("schema not forwarded")
	}
	var _ agent.ResearchTool = wrapped
}
