// Package skillseval checks whether a real model loads the skills its
// description promises. It is an offline tool: default tests never call a
// model, and fixtures live under eval/ instead of the production skills/ tree.
package skillseval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/domain"
	"trippilot/internal/skills"
)

const (
	DefaultTimeout      = 45 * time.Second
	DefaultMaxToolCalls = 4
	guideToolName       = "search_travel_notes"
)

type Case struct {
	ID           string       `json:"id"`
	Input        string       `json:"input"`
	City         string       `json:"city"`
	Days         int          `json:"days"`
	Candidates   []domain.POI `json:"candidates"`
	ExpectLoaded []string     `json:"expect_loaded"`
}

type Config struct {
	SkillsDir string
	Cases     []Case
	LLM       agent.ToolLLM
	Timeout   time.Duration
}

type Result struct {
	Case     Case
	Loaded   []string
	Duration time.Duration
	Err      error
}

type Report struct {
	Results []Result
	Passed  int
	Failed  int
}

func LoadCases(path string) ([]Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("skillseval: read %s: %w", path, err)
	}
	var cases []Case
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("skillseval: decode %s: %w", path, err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("skillseval: %s has no cases", path)
	}
	seen := make(map[string]struct{}, len(cases))
	for index, item := range cases {
		if item.ID == "" || item.Input == "" {
			return nil, fmt.Errorf("skillseval: case %d needs id and input", index)
		}
		if _, dup := seen[item.ID]; dup {
			return nil, fmt.Errorf("skillseval: duplicate case id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return cases, nil
}

// Run executes every case against a fresh provider so catalog changes between
// cases cannot leak. A tool or model failure fails only that case.
func Run(ctx context.Context, cfg Config) (Report, error) {
	if cfg.LLM == nil {
		return Report{}, fmt.Errorf("skillseval: a tool-capable model is required")
	}
	if cfg.SkillsDir == "" {
		return Report{}, fmt.Errorf("skillseval: skills dir is required")
	}
	if len(cfg.Cases) == 0 {
		return Report{}, fmt.Errorf("skillseval: no cases")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	report := Report{Results: make([]Result, 0, len(cfg.Cases))}
	for _, item := range cfg.Cases {
		result := runCase(ctx, cfg, item, timeout)
		if result.Err == nil {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, result)
	}
	return report, nil
}

func runCase(ctx context.Context, cfg Config, item Case, timeout time.Duration) Result {
	startedAt := time.Now()
	result := Result{Case: item, Loaded: []string{}}

	provider, err := skills.Open(cfg.SkillsDir)
	if err != nil {
		result.Err = fmt.Errorf("open skills: %w", err)
		result.Duration = time.Since(startedAt)
		return result
	}
	caseCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	pins, err := provider.List(caseCtx)
	if err != nil {
		result.Err = fmt.Errorf("list skills: %w", err)
		result.Duration = time.Since(startedAt)
		return result
	}

	events := &eventRecorder{}
	nodes := agent.NewNodes(nil, events,
		agent.WithToolLLM(cfg.LLM),
		agent.WithSkillProvider(provider),
		agent.WithResearchTools(noopGuideTool{}),
		agent.WithResearchLimits(timeout, DefaultMaxToolCalls),
	)
	if _, err := nodes.Research(caseCtx, agent.ResearchInput{
		RunID:       "skillseval:" + item.ID,
		Input:       item.Input,
		Constraints: domain.ConstraintSet{City: item.City, Days: item.Days},
		Candidates:  item.Candidates,
		SkillPins:   pins,
	}); err != nil {
		result.Err = fmt.Errorf("research: %w", err)
	}
	if caseCtx.Err() != nil {
		result.Err = fmt.Errorf("timeout after %s", timeout)
	}

	result.Loaded = events.loadedSkills()
	result.Duration = time.Since(startedAt)
	if result.Err == nil {
		if diff := setDifference(item.ExpectLoaded, result.Loaded); diff != "" {
			result.Err = fmt.Errorf("loaded set mismatch: %s", diff)
		}
	}
	return result
}

func setDifference(want, got []string) string {
	wanted := normalizeSet(want)
	actual := normalizeSet(got)
	var missing, extra []string
	for name := range wanted {
		if _, ok := actual[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name := range actual {
		if _, ok := wanted[name]; !ok {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	switch {
	case len(missing) == 0 && len(extra) == 0:
		return ""
	case len(missing) == 0:
		return "unexpected " + joinNames(extra)
	case len(extra) == 0:
		return "missing " + joinNames(missing)
	default:
		return "missing " + joinNames(missing) + ", unexpected " + joinNames(extra)
	}
}

func normalizeSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func joinNames(names []string) string {
	return strings.Join(names, ", ")
}

type eventRecorder struct {
	mu     sync.Mutex
	loaded []string
}

func (r *eventRecorder) Publish(_ context.Context, _ string, typ string, payload any) error {
	if typ != "research.skill_loaded" {
		return nil
	}
	values, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	name, _ := values["name"].(string)
	if name == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.loaded {
		if existing == name {
			return nil
		}
	}
	r.loaded = append(r.loaded, name)
	return nil
}

func (r *eventRecorder) loadedSkills() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.loaded...)
}

type noopGuideTool struct{}

func (noopGuideTool) Name() string { return guideToolName }

func (noopGuideTool) Description() string {
	return "查询旅行攻略摘要；评测中固定返回空结果。"
}

func (noopGuideTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`)
}

func (noopGuideTool) Call(context.Context, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"results":[]}`), nil
}
