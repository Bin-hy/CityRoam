package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"trippilot/internal/domain"
)

// EventPublisher is implemented by the events broker. Nodes publish domain
// milestones; persistence and fan-out stay outside the agent.
type EventPublisher interface {
	Publish(ctx context.Context, runID string, typ string, payload any) error
}

// NodeError carries a stable machine-readable code through the run.
type NodeError struct {
	Code    string
	Message string
	Err     error
}

func (e *NodeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *NodeError) Unwrap() error { return e.Err }

// Nodes holds the dependencies shared by every agent stage.
type Nodes struct {
	llm            LLMClient
	events         EventPublisher
	logger         *slog.Logger
	now            func() time.Time
	snapshot       CandidateSource
	external       CandidateSource
	lookup         POILookup
	candidateLimit int
	scheduler      Scheduler
	validator      Validator
	routes         domain.RouteProvider

	toolLLM              ToolLLM
	researchTools        []ResearchTool
	researchDisabled     []DisabledPlugin
	researchTimeout      time.Duration
	researchMaxToolCalls int
	researchContext      func(context.Context, string, string) context.Context
}

type NodeOption func(*Nodes)

func WithNodeLogger(logger *slog.Logger) NodeOption {
	return func(n *Nodes) { n.logger = logger }
}

func WithNodeClock(now func() time.Time) NodeOption {
	return func(n *Nodes) { n.now = now }
}

func WithToolLLM(client ToolLLM) NodeOption {
	return func(n *Nodes) { n.toolLLM = client }
}

func WithResearchTools(tools ...ResearchTool) NodeOption {
	return func(n *Nodes) {
		n.researchTools = make([]ResearchTool, 0, len(tools))
		for _, tool := range tools {
			if tool != nil {
				n.researchTools = append(n.researchTools, tool)
			}
		}
	}
}

func WithResearchLimits(timeout time.Duration, maxToolCalls int) NodeOption {
	return func(n *Nodes) {
		if timeout > 0 {
			n.researchTimeout = timeout
		}
		if maxToolCalls > 0 {
			n.researchMaxToolCalls = maxToolCalls
		}
	}
}

func WithResearchDisabled(disabled ...DisabledPlugin) NodeOption {
	return func(n *Nodes) {
		n.researchDisabled = append([]DisabledPlugin(nil), disabled...)
	}
}

// WithResearchContext lets wiring attach run identity (for the plugin
// Governor) without making the agent package depend on plugins.
func WithResearchContext(decorate func(context.Context, string, string) context.Context) NodeOption {
	return func(n *Nodes) { n.researchContext = decorate }
}

func NewNodes(llm LLMClient, events EventPublisher, opts ...NodeOption) *Nodes {
	n := &Nodes{
		llm:                  llm,
		events:               events,
		logger:               slog.Default(),
		now:                  time.Now,
		candidateLimit:       maxCandidates,
		scheduler:            defaultScheduler(),
		validator:            defaultValidator(),
		researchTimeout:      12 * time.Second,
		researchMaxToolCalls: 6,
	}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

type ParseInput struct {
	RunID string
	Input string
}

type ParseOutput struct {
	Constraints domain.ConstraintSet
	Usage       Usage
	Missing     []string
}

// Parse turns one free-form request into a ConstraintSet with defaults. When
// city or days is absent it publishes constraint.needs_input and returns
// normally so the run can wait for the user.
func (n *Nodes) Parse(ctx context.Context, in ParseInput) (ParseOutput, error) {
	input := strings.TrimSpace(in.Input)
	if input == "" {
		return ParseOutput{}, &NodeError{Code: "empty_input", Message: "行程需求不能为空"}
	}

	prompt, err := RenderPrompt("parse", ParsePromptData{
		Input: input,
		Today: n.now().Format("2006-01-02"),
	})
	if err != nil {
		return ParseOutput{}, &NodeError{Code: "prompt_failed", Message: "渲染解析提示词失败", Err: err}
	}

	raw, usage, err := n.llm.GenerateJSON(ctx, prompt, ConstraintSchema)
	if err != nil {
		return ParseOutput{}, &NodeError{Code: "parse_llm_failed", Message: "解析需求失败", Err: err}
	}

	var constraints domain.ConstraintSet
	if err := json.Unmarshal(raw, &constraints); err != nil {
		return ParseOutput{}, &NodeError{Code: "parse_decode_failed", Message: "解析结果不是合法约束", Err: err}
	}
	constraints = normalizeConstraints(constraints).ApplyDefaults()
	if constraints.StartDate == "" {
		constraints.StartDate = n.now().AddDate(0, 0, 1).Format("2006-01-02")
	}

	missing := constraints.MissingCritical()
	constraints.Missing = missing
	if len(missing) > 0 {
		payload := map[string]any{"constraints": constraints, "missing": missing}
		if err := n.events.Publish(ctx, in.RunID, "constraint.needs_input", payload); err != nil {
			return ParseOutput{}, &NodeError{Code: "event_failed", Message: "发布追问事件失败", Err: err}
		}
		return ParseOutput{Constraints: constraints, Usage: usage, Missing: missing}, nil
	}

	if err := constraints.Validate(); err != nil {
		return ParseOutput{}, &NodeError{Code: "invalid_constraints", Message: "约束不合法", Err: err}
	}

	if err := n.events.Publish(ctx, in.RunID, "constraint.parsed", map[string]any{"constraints": constraints}); err != nil {
		return ParseOutput{}, &NodeError{Code: "event_failed", Message: "发布约束事件失败", Err: err}
	}

	n.logger.Info("constraints parsed",
		"run_id", in.RunID,
		"city", constraints.City,
		"days", constraints.Days,
		"tokens", usage.TotalTokens,
	)
	return ParseOutput{Constraints: constraints, Usage: usage}, nil
}

func normalizeConstraints(c domain.ConstraintSet) domain.ConstraintSet {
	c.City = normalizeCity(c.City)
	c.Interests = cleanNames(c.Interests)
	c.MustVisit = cleanNames(c.MustVisit)
	c.Avoid = cleanNames(c.Avoid)
	c.Lodging.Area = strings.TrimSpace(c.Lodging.Area)
	c.Lodging.Hotels = cleanNames(c.Lodging.Hotels)
	if c.Lodging.MaxPricePerNightYuan < 0 {
		c.Lodging.MaxPricePerNightYuan = 0
	}
	if !c.Lodging.Mentioned {
		c.Lodging.Hotels = nil
		c.Lodging.Area = ""
		c.Lodging.MaxPricePerNightYuan = 0
	}
	return c
}

func normalizeCity(city string) string {
	trimmed := strings.TrimSpace(city)
	switch strings.ToLower(trimmed) {
	case "beijing", "北京", "北京市":
		return "beijing"
	}
	return trimmed
}

func cleanNames(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, dup := seen[trimmed]; dup {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
