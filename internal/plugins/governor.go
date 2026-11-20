package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/repo"
)

const (
	CodeAuthFailed      = "auth_failed"
	CodeTimeout         = "timeout"
	CodeRateLimited     = "rate_limited"
	CodeBadResponse     = "bad_response"
	CodeEmptyResult     = "empty_result"
	CodeInvalidInput    = "invalid_input"
	CodeToolCallLimit   = "tool_call_limit"
	CodeMissingRun      = "missing_run_context"
	CodeUpstreamFailure = "upstream_unavailable"

	defaultGovernorTimeout      = 10 * time.Second
	defaultGovernorMaxCalls     = 6
	defaultGovernorPersistLimit = 16 << 10
	defaultGovernorPersistTime  = 3 * time.Second
	maxToolAttempts             = 2
)

var defaultToolLimits = map[string]int{
	"search_hotels":       2,
	"search_travel_notes": 3,
}

// ToolError is the only error shape tool clients should return to the
// Governor. Its Message is shown to the research LLM, so it must never contain
// credentials or raw upstream bodies.
type ToolError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *ToolError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func IsRetryable(err error) bool {
	var toolErr *ToolError
	if errors.As(err, &toolErr) {
		return toolErr.Retryable
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// ToolCallStore and EvidenceStore are narrowed persistence seams so tests do
// not need a database and the Governor does not depend on *repo.Store.
type ToolCallStore interface {
	InsertToolCall(ctx context.Context, arg repo.InsertToolCallParams) (repo.ToolCall, error)
}

type EvidenceStore interface {
	InsertEvidence(ctx context.Context, arg repo.InsertEvidenceParams) (repo.Evidence, error)
}

type EventPublisher interface {
	Publish(ctx context.Context, runID, typ string, payload any) error
}

type EvidenceRecord struct {
	POIID      string
	Kind       string
	Claim      string
	Source     string
	SourceURL  string
	Confidence *float64
}

// EvidenceProvider lets a tool describe which parts of its own result should
// become evidence without the Governor parsing business payloads.
type EvidenceProvider interface {
	Evidence(input, output json.RawMessage) []EvidenceRecord
}

type ResultCounter interface {
	ResultCount(output json.RawMessage) int
}

type ProviderNamer interface {
	Provider() string
}

type Governor struct {
	cache          Cache
	calls          ToolCallStore
	evidence       EvidenceStore
	events         EventPublisher
	timeout        time.Duration
	toolTimeouts   map[string]time.Duration
	maxCalls       int
	toolLimits     map[string]int
	persistBytes   int
	persistTimeout time.Duration
	retryDelay     time.Duration
	now            func() time.Time
	logger         *slog.Logger

	mu   sync.Mutex
	runs map[string]*runCounters
}

type runCounters struct {
	total  int
	byTool map[string]int
}

type GovernorOption func(*Governor)

func WithGovernorTimeout(timeout time.Duration) GovernorOption {
	return func(g *Governor) {
		if timeout > 0 {
			g.timeout = timeout
		}
	}
}

func WithGovernorMaxCalls(max int) GovernorOption {
	return func(g *Governor) {
		if max > 0 {
			g.maxCalls = max
		}
	}
}

func WithGovernorToolLimit(tool string, max int) GovernorOption {
	return func(g *Governor) {
		if tool != "" && max > 0 {
			g.toolLimits[tool] = max
		}
	}
}

func WithGovernorToolTimeout(tool string, timeout time.Duration) GovernorOption {
	return func(g *Governor) {
		if tool != "" && timeout > 0 {
			g.toolTimeouts[tool] = timeout
		}
	}
}

func WithGovernorClock(now func() time.Time) GovernorOption {
	return func(g *Governor) {
		if now != nil {
			g.now = now
		}
	}
}

func WithGovernorLogger(logger *slog.Logger) GovernorOption {
	return func(g *Governor) {
		if logger != nil {
			g.logger = logger
		}
	}
}

func WithGovernorRetryDelay(delay time.Duration) GovernorOption {
	return func(g *Governor) {
		if delay >= 0 {
			g.retryDelay = delay
		}
	}
}

func NewGovernor(cache Cache, calls ToolCallStore, evidence EvidenceStore, events EventPublisher, opts ...GovernorOption) *Governor {
	g := &Governor{
		cache:          cache,
		calls:          calls,
		evidence:       evidence,
		events:         events,
		timeout:        defaultGovernorTimeout,
		toolTimeouts:   make(map[string]time.Duration),
		maxCalls:       defaultGovernorMaxCalls,
		toolLimits:     make(map[string]int, len(defaultToolLimits)),
		persistBytes:   defaultGovernorPersistLimit,
		persistTimeout: defaultGovernorPersistTime,
		now:            time.Now,
		logger:         slog.Default(),
		runs:           make(map[string]*runCounters),
	}
	for name, limit := range defaultToolLimits {
		g.toolLimits[name] = limit
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Wrap returns a governed view of a tool. Identity methods are forwarded
// unchanged so the research prompt sees the same schema.
func (g *Governor) Wrap(tool agent.ResearchTool) agent.ResearchTool {
	if tool == nil {
		return nil
	}
	return &governedTool{governor: g, tool: tool}
}

type governedTool struct {
	governor *Governor
	tool     agent.ResearchTool
}

func (t *governedTool) Name() string {
	return t.tool.Name()
}

func (t *governedTool) Description() string {
	return t.tool.Description()
}

func (t *governedTool) InputSchema() json.RawMessage {
	return t.tool.InputSchema()
}

func (t *governedTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	return t.governor.call(ctx, t.tool, input)
}

func (g *Governor) call(ctx context.Context, tool agent.ResearchTool, input json.RawMessage) (json.RawMessage, error) {
	start := g.now()
	run, ok := RunFromContext(ctx)
	if !ok {
		return errorEnvelope(CodeMissingRun, "missing run context"), nil
	}
	name := tool.Name()
	callIndex, limitCode, limitMessage := g.reserve(run.RunID, name)
	provider := name
	if named, ok := tool.(ProviderNamer); ok && named.Provider() != "" {
		provider = named.Provider()
	}
	if limitCode != "" {
		g.record(ctx, run, name, callIndex, input, nil, limitCode, limitMessage, 0, false, 0)
		g.publish(ctx, run.RunID, "plugin.tool_called", toolEvent{
			Name: name, Provider: provider, CallIndex: callIndex, Status: "started",
		})
		g.publish(ctx, run.RunID, "plugin.tool_completed", toolEvent{
			Name: name, Provider: provider, CallIndex: callIndex, Status: "error",
		})
		return errorEnvelope(limitCode, limitMessage), nil
	}
	cacheKey := ""
	cacheTTL := time.Duration(0)
	if keyer, ok := tool.(CacheKeyer); ok && g.cache != nil {
		key, err := keyer.CacheKey(input)
		if err == nil {
			cacheKey = key
			cacheTTL = keyer.CacheTTL()
		}
	}

	g.publish(ctx, run.RunID, "plugin.tool_called", toolEvent{
		Name: name, Provider: provider, CallIndex: callIndex, Status: "started",
	})

	if cacheKey != "" {
		if cached, ok := g.cached(ctx, cacheKey); ok {
			latency := g.now().Sub(start)
			g.record(ctx, run, name, callIndex, input, cached, "", "", latency, true, 0)
			g.writeEvidence(ctx, run, tool, input, cached)
			g.publish(ctx, run.RunID, "plugin.tool_completed", toolEvent{
				Name: name, Provider: provider, CallIndex: callIndex, Status: "ok",
				LatencyMS: latency.Milliseconds(), ResultCount: resultCount(tool, cached), Cached: true,
			})
			return cached, nil
		}
	}

	callCtx, cancel := g.callerContext(ctx, name)
	defer cancel()

	var (
		output   json.RawMessage
		callErr  error
		attempts int
	)
	for attempts = 1; attempts <= maxToolAttempts; attempts++ {
		output, callErr = tool.Call(callCtx, input)
		if callErr == nil {
			break
		}
		if attempts == maxToolAttempts || !IsRetryable(callErr) || ctx.Err() != nil || callCtx.Err() != nil {
			break
		}
		if g.retryDelay > 0 {
			timer := time.NewTimer(g.retryDelay)
			select {
			case <-callCtx.Done():
				timer.Stop()
			case <-timer.C:
			}
			if callCtx.Err() != nil {
				callErr = callCtx.Err()
				break
			}
		}
	}

	latency := g.now().Sub(start)
	if callErr != nil {
		code, message := classify(callCtx, callErr)
		g.record(ctx, run, name, callIndex, input, nil, code, message, latency, false, attempts)
		g.publish(ctx, run.RunID, "plugin.tool_completed", toolEvent{
			Name: name, Provider: provider, CallIndex: callIndex, Status: "error",
			LatencyMS: latency.Milliseconds(), Attempts: attempts,
		})
		return errorEnvelope(code, message), nil
	}

	if cacheKey != "" && cacheTTL > 0 && g.cache != nil {
		if err := g.cache.Set(ctx, cacheKey, provider, output, cacheTTL); err != nil {
			g.logger.Warn("plugins: cache set failed", "tool", name, "error", err)
		}
	}
	g.record(ctx, run, name, callIndex, input, output, "", "", latency, false, attempts)
	g.writeEvidence(ctx, run, tool, input, output)
	g.publish(ctx, run.RunID, "plugin.tool_completed", toolEvent{
		Name: name, Provider: provider, CallIndex: callIndex, Status: "ok",
		LatencyMS: latency.Milliseconds(), ResultCount: resultCount(tool, output), Attempts: attempts,
	})
	return output, nil
}

func (g *Governor) cached(ctx context.Context, key string) (json.RawMessage, bool) {
	value, hit, err := g.cache.Get(ctx, key)
	if err != nil {
		g.logger.Warn("plugins: cache get failed", "error", err)
		return nil, false
	}
	if !hit || !json.Valid(value) {
		return nil, false
	}
	return value, true
}

func (g *Governor) reserve(runID, tool string) (int, string, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	counter := g.runs[runID]
	if counter == nil {
		counter = &runCounters{byTool: make(map[string]int)}
		g.runs[runID] = counter
	}
	next := counter.total + 1
	if counter.total >= g.maxCalls {
		return next, CodeToolCallLimit, fmt.Sprintf("run_limit: this run may call at most %d research tools", g.maxCalls)
	}
	if limit, ok := g.toolLimits[tool]; ok && counter.byTool[tool] >= limit {
		return next, CodeToolCallLimit, fmt.Sprintf("tool_limit: %s may be called at most %d times per run", tool, limit)
	}
	counter.total++
	counter.byTool[tool]++
	return next, "", ""
}

func (g *Governor) callerContext(ctx context.Context, tool string) (context.Context, context.CancelFunc) {
	timeout := g.timeout
	if configured, ok := g.toolTimeouts[tool]; ok {
		timeout = configured
	}
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	return context.WithTimeout(ctx, timeout)
}

func classify(callCtx context.Context, err error) (string, string) {
	var toolErr *ToolError
	if errors.As(err, &toolErr) {
		message := strings.TrimSpace(toolErr.Message)
		if message == "" {
			message = "tool call failed"
		}
		return toolErr.Code, message
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		return CodeTimeout, "tool call timed out"
	}
	if errors.Is(err, context.Canceled) || errors.Is(callCtx.Err(), context.Canceled) {
		return CodeTimeout, "tool call was cancelled"
	}
	return CodeUpstreamFailure, "tool call failed; upstream is unavailable"
}

func (g *Governor) record(ctx context.Context, run RunContext, tool string, callIndex int, input, output json.RawMessage, code, message string, latency time.Duration, cached bool, attempts int) {
	if g.calls == nil {
		return
	}
	status := "ok"
	var errText *string
	if code != "" {
		status = "error"
		text := code + ": " + message
		errText = &text
	}
	storedOutput := output
	if cached && len(output) > 0 {
		marked, err := json.Marshal(map[string]json.RawMessage{"cached": json.RawMessage("true"), "result": output})
		if err == nil {
			storedOutput = marked
		}
	}
	latencyMS := int32(latency.Milliseconds())
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), g.persistTimeout)
	defer cancel()
	_, err := g.calls.InsertToolCall(persistCtx, repo.InsertToolCallParams{
		RunID:     run.RunID,
		TripID:    run.TripID,
		Tool:      tool,
		Provider:  "plugin",
		Input:     limitedJSON(input, g.persistBytes),
		Output:    limitedJSON(storedOutput, g.persistBytes),
		Status:    status,
		LatencyMs: &latencyMS,
		Error:     errText,
	})
	if err != nil {
		g.logger.Warn("plugins: persist tool call failed", "tool", tool, "error", err)
	}
}

func (g *Governor) writeEvidence(ctx context.Context, run RunContext, tool agent.ResearchTool, input, output json.RawMessage) {
	provider, ok := tool.(EvidenceProvider)
	if !ok || g.evidence == nil {
		return
	}
	records := provider.Evidence(input, output)
	if len(records) == 0 {
		return
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), g.persistTimeout)
	defer cancel()
	for _, record := range records {
		var poiID, sourceURL *string
		if strings.TrimSpace(record.POIID) != "" {
			value := record.POIID
			poiID = &value
		}
		if httpURL(record.SourceURL) {
			value := record.SourceURL
			sourceURL = &value
		}
		_, err := g.evidence.InsertEvidence(persistCtx, repo.InsertEvidenceParams{
			TripID:     run.TripID,
			RunID:      &run.RunID,
			PoiID:      poiID,
			Kind:       record.Kind,
			Claim:      record.Claim,
			Source:     record.Source,
			SourceUrl:  sourceURL,
			Confidence: record.Confidence,
		})
		if err != nil {
			g.logger.Warn("plugins: persist evidence failed", "tool", tool.Name(), "error", err)
			return
		}
	}
}

func (g *Governor) publish(ctx context.Context, runID, typ string, payload toolEvent) {
	if g.events == nil {
		return
	}
	publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), g.persistTimeout)
	defer cancel()
	if err := g.events.Publish(publishCtx, runID, typ, payload); err != nil {
		g.logger.Warn("plugins: publish tool event failed", "event", typ, "error", err)
	}
}

type toolEvent struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	CallIndex   int    `json:"call_index"`
	Status      string `json:"status"`
	LatencyMS   int64  `json:"latency_ms,omitempty"`
	ResultCount int    `json:"result_count,omitempty"`
	Cached      bool   `json:"cached,omitempty"`
	Attempts    int    `json:"attempts,omitempty"`
}

func resultCount(tool agent.ResearchTool, output json.RawMessage) int {
	if counter, ok := tool.(ResultCounter); ok {
		return counter.ResultCount(output)
	}
	return 0
}

func errorEnvelope(code, message string) json.RawMessage {
	payload, err := json.Marshal(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
	if err != nil {
		return json.RawMessage(`{"error":{"code":"bad_response","message":"tool call failed"}}`)
	}
	return payload
}

func limitedJSON(value json.RawMessage, limit int) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	if len(value) <= limit {
		return value
	}
	wrapped, err := json.Marshal(map[string]any{
		"truncated": true,
		"preview":   string(value[:limit]),
	})
	if err != nil {
		return json.RawMessage(`{"truncated":true}`)
	}
	return wrapped
}
