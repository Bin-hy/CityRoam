package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	DefaultTimeout          = 15 * time.Second
	DefaultMaxRetries       = 2
	DefaultRunCallLimit     = 5
	DefaultBreakerThreshold = 3
	DefaultBreakerCooldown  = 30 * time.Second
	defaultBackoff          = 200 * time.Millisecond
)

var (
	ErrNotAllowed    = errors.New("mcp: tool not in allowlist")
	ErrRunLimit      = errors.New("mcp: per-run call limit reached")
	ErrCircuitOpen   = errors.New("mcp: circuit open")
	ErrUnknownServer = errors.New("mcp: unknown server")
	ErrRunIDRequired = errors.New("mcp: run id missing from context")
)

// Config governs every MCP call. Zero values fall back to the defaults above.
type Config struct {
	Timeout          time.Duration
	MaxRetries       int
	RunCallLimit     int
	BreakerThreshold int
	BreakerCooldown  time.Duration
	Backoff          time.Duration
	Logger           *slog.Logger
	Now              func() time.Time
}

func (c Config) withDefaults() Config {
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.RunCallLimit <= 0 {
		c.RunCallLimit = DefaultRunCallLimit
	}
	if c.BreakerThreshold <= 0 {
		c.BreakerThreshold = DefaultBreakerThreshold
	}
	if c.BreakerCooldown <= 0 {
		c.BreakerCooldown = DefaultBreakerCooldown
	}
	if c.Backoff <= 0 {
		c.Backoff = defaultBackoff
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

// Result is a transport-neutral view of a tool call.
type Result struct {
	Content    []sdk.Content
	Structured json.RawMessage
	IsError    bool
}

// CallRecord captures one governed call for traceability and tests.
type CallRecord struct {
	RunID    string
	Server   string
	Tool     string
	Attempts int
	Duration time.Duration
	Err      string
	IsError  bool
	At       time.Time
}

type serverState struct {
	session *sdk.ClientSession
	allowed map[string]bool
}

type breakerKey struct {
	server string
	tool   string
}

type breakerState struct {
	failures int
	openedAt time.Time
	probing  bool
}

// Hub is the only entry point to external MCP servers. It enforces the
// allowlist, per-run budget, timeout, retries and per-tool circuit breaking.
type Hub struct {
	cfg Config

	mu       sync.Mutex
	servers  map[string]*serverState
	breakers map[breakerKey]*breakerState
	runCalls map[string]int
	calls    []CallRecord
}

func NewHub(cfg Config) *Hub {
	return &Hub{
		cfg:      cfg.withDefaults(),
		servers:  make(map[string]*serverState),
		breakers: make(map[breakerKey]*breakerState),
		runCalls: make(map[string]int),
	}
}

type runIDKey struct{}

func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDKey{}, runID)
}

func RunIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(runIDKey{}).(string)
	return id, ok && id != ""
}

// AddServer connects the hub to a transport, discovers its tools and keeps
// only the allowlisted ones callable. An empty allowlist denies everything.
func (h *Hub) AddServer(ctx context.Context, name string, transport sdk.Transport, allowedTools []string) error {
	if name == "" {
		return errors.New("mcp: server name is required")
	}

	allowed := make(map[string]bool, len(allowedTools))
	for _, tool := range allowedTools {
		if tool != "" {
			allowed[tool] = true
		}
	}
	if len(allowed) == 0 {
		return fmt.Errorf("mcp: server %q has an empty allowlist", name)
	}

	client := sdk.NewClient(&sdk.Implementation{Name: "trippilot", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("mcp: connect %s: %w", name, err)
	}

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		session.Close()
		return fmt.Errorf("mcp: list tools for %s: %w", name, err)
	}
	existing := make(map[string]bool, len(listed.Tools))
	for _, tool := range listed.Tools {
		existing[tool.Name] = true
	}
	for tool := range allowed {
		if !existing[tool] {
			session.Close()
			return fmt.Errorf("mcp: server %s does not expose allowlisted tool %q", name, tool)
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if _, dup := h.servers[name]; dup {
		session.Close()
		return fmt.Errorf("mcp: server %q already registered", name)
	}
	h.servers[name] = &serverState{session: session, allowed: allowed}
	return nil
}

func (h *Hub) Close() error {
	h.mu.Lock()
	servers := h.servers
	h.servers = make(map[string]*serverState)
	h.mu.Unlock()

	var firstErr error
	for _, state := range servers {
		if err := state.session.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Call invokes an allowlisted tool within the run identified by ctx. Retries
// reuse the same deadline, so the 15s budget covers all attempts.
func (h *Hub) Call(ctx context.Context, server, tool string, args map[string]any) (*Result, error) {
	runID, ok := RunIDFrom(ctx)
	if !ok {
		return nil, ErrRunIDRequired
	}

	state, err := h.server(server, tool)
	if err != nil {
		return nil, err
	}
	if err := h.breakerAllow(breakerKey{server, tool}); err != nil {
		return nil, err
	}
	if err := h.beginCall(runID); err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, h.cfg.Timeout)
	defer cancel()

	start := h.cfg.Now()
	attempts := 0
	var lastResult *Result
	var lastErr error

retry:
	for attempt := 0; attempt <= h.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(h.cfg.Backoff)
			select {
			case <-callCtx.Done():
				timer.Stop()
				lastResult = nil
				lastErr = callCtx.Err()
				break retry
			case <-timer.C:
			}
		}

		attempts++
		lastResult, lastErr = invoke(callCtx, state.session, tool, args)
		if lastErr == nil && lastResult != nil && !lastResult.IsError {
			break
		}
	}

	record := CallRecord{
		RunID:    runID,
		Server:   server,
		Tool:     tool,
		Attempts: attempts,
		Duration: h.cfg.Now().Sub(start),
		At:       start,
	}
	if lastErr != nil {
		record.Err = lastErr.Error()
	} else if lastResult != nil && lastResult.IsError {
		record.IsError = true
		record.Err = "tool returned an error result"
	}
	h.finishCall(record)

	if lastErr != nil {
		return nil, fmt.Errorf("mcp: %s/%s: %w", server, tool, lastErr)
	}
	if lastResult != nil && lastResult.IsError {
		return lastResult, fmt.Errorf("mcp: %s/%s returned an error result", server, tool)
	}
	return lastResult, nil
}

// Calls returns a snapshot of every governed call in invocation order.
func (h *Hub) Calls() []CallRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]CallRecord, len(h.calls))
	copy(out, h.calls)
	return out
}

// ToolSchema returns the JSON schema advertised by an allowlisted tool.
func (h *Hub) ToolSchema(ctx context.Context, server, tool string) (json.RawMessage, error) {
	state, err := h.server(server, tool)
	if err != nil {
		return nil, err
	}
	listed, err := state.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: list tools for %s: %w", server, err)
	}
	for _, candidate := range listed.Tools {
		if candidate.Name != tool {
			continue
		}
		schema, err := json.Marshal(candidate.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("mcp: encode schema for %s/%s: %w", server, tool, err)
		}
		return schema, nil
	}
	return nil, fmt.Errorf("mcp: tool %s/%s disappeared", server, tool)
}

func (h *Hub) server(name, tool string) (*serverState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state, ok := h.servers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownServer, name)
	}
	if !state.allowed[tool] {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotAllowed, name, tool)
	}
	return state, nil
}

func (h *Hub) beginCall(runID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.runCalls[runID] >= h.cfg.RunCallLimit {
		return fmt.Errorf("%w: run %s already used %d calls", ErrRunLimit, runID, h.runCalls[runID])
	}
	h.runCalls[runID]++
	return nil
}

func (h *Hub) breakerAllow(key breakerKey) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.breakers[key]
	if state == nil || state.failures < h.cfg.BreakerThreshold {
		return nil
	}
	if state.probing {
		return fmt.Errorf("%w: %s/%s", ErrCircuitOpen, key.server, key.tool)
	}
	if h.cfg.Now().Sub(state.openedAt) < h.cfg.BreakerCooldown {
		return fmt.Errorf("%w: %s/%s", ErrCircuitOpen, key.server, key.tool)
	}
	state.probing = true
	return nil
}

func (h *Hub) finishCall(record CallRecord) {
	key := breakerKey{record.Server, record.Tool}
	h.mu.Lock()
	if record.Err == "" {
		delete(h.breakers, key)
	} else if state := h.breakers[key]; state != nil {
		state.failures++
		state.probing = false
		state.openedAt = h.cfg.Now()
	} else {
		h.breakers[key] = &breakerState{failures: 1, openedAt: h.cfg.Now()}
	}
	h.calls = append(h.calls, record)
	h.mu.Unlock()

	level := slog.LevelInfo
	if record.Err != "" {
		level = slog.LevelWarn
	}
	h.cfg.Logger.Log(context.Background(), level, "mcp tool call",
		"run_id", record.RunID,
		"server", record.Server,
		"tool", record.Tool,
		"attempts", record.Attempts,
		"duration_ms", record.Duration.Milliseconds(),
		"error", record.Err,
	)
}

func invoke(ctx context.Context, session *sdk.ClientSession, tool string, args map[string]any) (*Result, error) {
	raw, err := session.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return nil, err
	}
	result := &Result{Content: raw.Content, IsError: raw.IsError}
	if raw.StructuredContent != nil {
		encoded, err := json.Marshal(raw.StructuredContent)
		if err != nil {
			return nil, fmt.Errorf("mcp: encode structured content: %w", err)
		}
		result.Structured = encoded
	}
	return result, nil
}
