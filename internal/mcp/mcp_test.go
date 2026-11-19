package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"trippilot/internal/tools"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func mockHub(t *testing.T, cfg Config, handlers map[string]sdk.ToolHandler) (*Hub, context.Context) {
	t.Helper()
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.Backoff == 0 {
		cfg.Backoff = time.Millisecond
	}

	srv := sdk.NewServer(&sdk.Implementation{Name: "mock", Version: "0.1"}, nil)
	names := make([]string, 0, len(handlers))
	for name, handler := range handlers {
		srv.AddTool(&sdk.Tool{
			Name:        name,
			Description: "mock tool " + name,
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		}, handler)
		names = append(names, name)
	}

	ctx, cancel := context.WithCancel(context.Background())
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	session, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("connect mock server: %v", err)
	}

	hub := NewHub(cfg)
	if err := hub.AddServer(ctx, "mock", clientTransport, names); err != nil {
		session.Close()
		cancel()
		t.Fatalf("add server: %v", err)
	}

	t.Cleanup(func() {
		hub.Close()
		session.Close()
		cancel()
	})
	return hub, WithRunID(ctx, "run-1")
}

func textResult(text string, isError bool) *sdk.CallToolResult {
	return &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: text}},
		IsError: isError,
	}
}

func TestHubRetriesAndRecords(t *testing.T) {
	var attempts atomic.Int32
	hub, ctx := mockHub(t, Config{MaxRetries: 2}, map[string]sdk.ToolHandler{
		"poi_search": func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			if attempts.Add(1) == 1 {
				return textResult("temporary failure", true), nil
			}
			return &sdk.CallToolResult{StructuredContent: map[string]any{
				"results": []any{map[string]any{"name": "故宫", "lat": 39.9163, "lng": 116.3972}},
			}}, nil
		},
	})

	result, err := hub.Call(ctx, "mock", "poi_search", map[string]any{"query": "故宫"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	payload, err := result.JSON()
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(string(payload), "故宫") {
		t.Fatalf("payload = %s", payload)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}

	calls := hub.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	record := calls[0]
	if record.RunID != "run-1" || record.Server != "mock" || record.Tool != "poi_search" {
		t.Fatalf("record = %+v", record)
	}
	if record.Attempts != 2 || record.Err != "" || record.IsError {
		t.Fatalf("record = %+v", record)
	}
}

func TestHubAllowlistDeniesUnknownTool(t *testing.T) {
	hub, ctx := mockHub(t, Config{}, map[string]sdk.ToolHandler{
		"poi_search": func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return textResult("ok", false), nil
		},
	})

	_, err := hub.Call(ctx, "mock", "secret_tool", nil)
	if !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("err = %v, want ErrNotAllowed", err)
	}
	if _, err := hub.Call(ctx, "missing", "poi_search", nil); !errors.Is(err, ErrUnknownServer) {
		t.Fatalf("err = %v, want ErrUnknownServer", err)
	}
	if got := len(hub.Calls()); got != 0 {
		t.Fatalf("calls = %d, want 0", got)
	}
}

func TestHubRunCallLimit(t *testing.T) {
	hub, ctx := mockHub(t, Config{RunCallLimit: 2}, map[string]sdk.ToolHandler{
		"poi_search": func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return textResult(`{"ok":true}`, false), nil
		},
	})

	for i := 0; i < 2; i++ {
		if _, err := hub.Call(ctx, "mock", "poi_search", nil); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if _, err := hub.Call(ctx, "mock", "poi_search", nil); !errors.Is(err, ErrRunLimit) {
		t.Fatalf("err = %v, want ErrRunLimit", err)
	}

	other := WithRunID(context.Background(), "run-2")
	if _, err := hub.Call(other, "mock", "poi_search", nil); err != nil {
		t.Fatalf("second run should have its own budget: %v", err)
	}
}

func TestHubCircuitBreaker(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	hub, ctx := mockHub(t, Config{
		MaxRetries:       0,
		BreakerThreshold: 2,
		BreakerCooldown:  50 * time.Millisecond,
		Now:              clock.Now,
	}, map[string]sdk.ToolHandler{
		"poi_search": func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return textResult("down", true), nil
		},
	})

	for i := 0; i < 2; i++ {
		if _, err := hub.Call(ctx, "mock", "poi_search", nil); err == nil {
			t.Fatalf("call %d should fail", i)
		}
	}
	if _, err := hub.Call(ctx, "mock", "poi_search", nil); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	if got := len(hub.Calls()); got != 2 {
		t.Fatalf("calls = %d, want 2 recorded failures", got)
	}

	clock.Advance(51 * time.Millisecond)
	if _, err := hub.Call(ctx, "mock", "poi_search", nil); err == nil || errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("half-open probe should reach the tool, got %v", err)
	}
	if _, err := hub.Call(ctx, "mock", "poi_search", nil); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want reopened circuit", err)
	}
}

func TestHubTimeout(t *testing.T) {
	hub, ctx := mockHub(t, Config{Timeout: 20 * time.Millisecond, MaxRetries: 0}, map[string]sdk.ToolHandler{
		"slow": func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			time.Sleep(200 * time.Millisecond)
			return textResult("late", false), nil
		},
	})

	start := time.Now()
	_, err := hub.Call(ctx, "mock", "slow", nil)
	if err == nil {
		t.Fatal("want timeout error")
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("elapsed = %v, timeout not enforced", elapsed)
	}
	calls := hub.Calls()
	if len(calls) != 1 || calls[0].Err == "" {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestHubRequiresRunID(t *testing.T) {
	hub, _ := mockHub(t, Config{}, map[string]sdk.ToolHandler{
		"poi_search": func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return textResult("ok", false), nil
		},
	})
	if _, err := hub.Call(context.Background(), "mock", "poi_search", nil); !errors.Is(err, ErrRunIDRequired) {
		t.Fatalf("err = %v, want ErrRunIDRequired", err)
	}
}

func TestAdapter(t *testing.T) {
	hub, ctx := mockHub(t, Config{}, map[string]sdk.ToolHandler{
		"poi_search": func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return textResult(`{"results":[{"name":"天坛","location":"116.4109,39.8822"}]}`, false), nil
		},
	})

	adapter, err := NewAdapter(ctx, hub, "mock", "poi_search", "external_poi_search")
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	if adapter.Name() != "external_poi_search" {
		t.Fatalf("name = %s", adapter.Name())
	}
	var schema map[string]any
	if err := json.Unmarshal(adapter.InputSchema(), &schema); err != nil || schema["type"] != "object" {
		t.Fatalf("schema = %s (err=%v)", adapter.InputSchema(), err)
	}

	registry := tools.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}
	tool, ok := registry.Get("external_poi_search")
	if !ok {
		t.Fatal("adapter not registered")
	}

	payload, err := tool.Call(ctx, json.RawMessage(`{"query":"天坛"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	normalized, err := Normalize("mock", "poi_search", "beijing", payload)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(normalized.Candidates) != 1 {
		t.Fatalf("candidates = %+v", normalized.Candidates)
	}
	candidate := normalized.Candidates[0]
	if candidate.POI.Name != "天坛" || candidate.POI.Lat != 39.8822 || candidate.POI.Lng != 116.4109 {
		t.Fatalf("poi = %+v", candidate.POI)
	}
	if candidate.POI.Source != "mock/poi_search" || candidate.POI.City != "beijing" {
		t.Fatalf("poi = %+v", candidate.POI)
	}

	if _, err := tool.Call(context.Background(), nil); !errors.Is(err, ErrRunIDRequired) {
		t.Fatalf("err = %v, want ErrRunIDRequired", err)
	}
}

func TestNormalize(t *testing.T) {
	payload := json.RawMessage(`{
		"results": [
			{"poi_id": "p1", "name": "故宫", "category": "历史", "lat": 39.9163, "lng": 116.3972, "confidence": 1.4},
			{"name": "天坛", "location": "116.4109,39.8822", "summary": "祈年殿值得一看", "url": "https://example.com/temple"},
			{"id": "p3", "name": "无坐标地点", "claim": "只在笔记中出现"},
			{"name": "坐标错误", "lat": 999, "lng": 116.4},
			{"lat": 39.9, "lng": 116.4}
		]
	}`)

	normalized, err := Normalize("mock", "poi_search", "beijing", payload)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(normalized.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(normalized.Candidates))
	}
	if len(normalized.Evidence) != 4 {
		t.Fatalf("evidence = %d, want 4", len(normalized.Evidence))
	}

	first := normalized.Candidates[0]
	if first.POI.ID != "p1" || first.Evidence.Confidence != 1 {
		t.Fatalf("first = %+v", first)
	}
	second := normalized.Candidates[1]
	if second.POI.ID == "" || !strings.HasPrefix(second.POI.ID, "mcp-") {
		t.Fatalf("generated id = %q", second.POI.ID)
	}
	if second.Evidence.SourceURL != "https://example.com/temple" {
		t.Fatalf("evidence = %+v", second.Evidence)
	}
	if second.Evidence.Claim != "祈年殿值得一看" {
		t.Fatalf("evidence = %+v", second.Evidence)
	}
	if normalized.Candidates[0].POI.CoordSystem != "GCJ-02" {
		t.Fatalf("coord system = %s", normalized.Candidates[0].POI.CoordSystem)
	}

	if _, err := Normalize("mock", "poi_search", "beijing", json.RawMessage(`{}`)); err == nil {
		t.Fatal("want error for unrecognizable payload")
	}

	arrayPayload := json.RawMessage(`[{"name":"颐和园","latitude":39.9999,"longitude":116.2755}]`)
	arrayNormalized, err := Normalize("mock", "poi_search", "beijing", arrayPayload)
	if err != nil || len(arrayNormalized.Candidates) != 1 {
		t.Fatalf("array normalize: %+v, %v", arrayNormalized, err)
	}
}
