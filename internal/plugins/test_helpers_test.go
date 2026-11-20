package plugins

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/repo"
)

type stubTool struct {
	name          string
	call          func(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
	cacheKey      string
	cacheTTL      time.Duration
	evidence      []EvidenceRecord
	provider      string
	querySummary  string
	resultPreview []EventPreview
	callCount     int
	mu            sync.Mutex
}

func (t *stubTool) Name() string {
	if t.name == "" {
		return "stub_tool"
	}
	return t.name
}

func (t *stubTool) Description() string {
	return "stub"
}

func (t *stubTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *stubTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	t.mu.Lock()
	t.callCount++
	t.mu.Unlock()
	if t.call == nil {
		return json.RawMessage(`{"ok":true}`), nil
	}
	return t.call(ctx, input)
}

func (t *stubTool) Calls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.callCount
}

func (t *stubTool) CacheKey(input json.RawMessage) (string, error) {
	return t.cacheKey, nil
}

func (t *stubTool) CacheTTL() time.Duration {
	return t.cacheTTL
}

func (t *stubTool) Evidence(input, output json.RawMessage) []EvidenceRecord {
	return t.evidence
}

func (t *stubTool) ResultCount(output json.RawMessage) int {
	var decoded struct {
		Items []any `json:"items"`
	}
	if json.Unmarshal(output, &decoded) != nil {
		return 0
	}
	return len(decoded.Items)
}

func (t *stubTool) Provider() string {
	return t.provider
}

func (t *stubTool) QuerySummary(input json.RawMessage) string {
	return t.querySummary
}

func (t *stubTool) ResultPreview(output json.RawMessage) []EventPreview {
	return t.resultPreview
}

type stubPlugin struct {
	name   string
	status Status
	tools  []agent.ResearchTool
}

func (p *stubPlugin) Name() string {
	return p.name
}

func (p *stubPlugin) Status() Status {
	return p.status
}

func (p *stubPlugin) Tools() []agent.ResearchTool {
	return p.tools
}

type fakeCallStore struct {
	mu    sync.Mutex
	calls []repo.InsertToolCallParams
	fail  error
}

func (s *fakeCallStore) InsertToolCall(ctx context.Context, arg repo.InsertToolCallParams) (repo.ToolCall, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, arg)
	return repo.ToolCall{}, s.fail
}

func (s *fakeCallStore) snapshot() []repo.InsertToolCallParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]repo.InsertToolCallParams(nil), s.calls...)
}

type fakeEvidenceStore struct {
	mu      sync.Mutex
	records []repo.InsertEvidenceParams
}

func (s *fakeEvidenceStore) InsertEvidence(ctx context.Context, arg repo.InsertEvidenceParams) (repo.Evidence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, arg)
	return repo.Evidence{}, nil
}

func (s *fakeEvidenceStore) snapshot() []repo.InsertEvidenceParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]repo.InsertEvidenceParams(nil), s.records...)
}

type fakePublisher struct {
	mu     sync.Mutex
	events []string
	items  []any
	fail   error
}

func (p *fakePublisher) Publish(ctx context.Context, runID, typ string, payload any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, typ)
	p.items = append(p.items, payload)
	return p.fail
}

func (p *fakePublisher) list() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.events...)
}

type fakeCache struct {
	mu      sync.Mutex
	values  map[string]json.RawMessage
	getErr  error
	setErr  error
	gets    int
	sets    int
	lastTTL time.Duration
}

func (c *fakeCache) Get(ctx context.Context, key string) (json.RawMessage, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets++
	if c.getErr != nil {
		return nil, false, c.getErr
	}
	value, ok := c.values[key]
	return value, ok, nil
}

func (c *fakeCache) Set(ctx context.Context, key, provider string, value json.RawMessage, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sets++
	c.lastTTL = ttl
	if c.setErr != nil {
		return c.setErr
	}
	if c.values == nil {
		c.values = make(map[string]json.RawMessage)
	}
	c.values[key] = append(json.RawMessage(nil), value...)
	return nil
}
