package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/compose"

	"trippilot/internal/domain"
)

const (
	nodeAdmit    = "admit"
	nodeParse    = "parse"
	nodeRetrieve = "retrieve"
	nodeDraft    = "draft"
	nodeSchedule = "schedule"
	nodeValidate = "validate"
	nodeRepair   = "repair"
	nodePresent  = "present"
)

// Graph is the compiled Eino pipeline. It only orchestrates: every stage
// delegates to a deterministic Nodes method.
type Graph struct {
	nodes    *Nodes
	runnable compose.Runnable[*TripState, *TripState]
}

// GraphOption customizes graph wiring.
type GraphOption func(*graphOptions)

type graphOptions struct {
	stageHook StageHook
}

// StageHook runs after every stage with the updated state, which lets the
// Runner persist runs.state between stages.
type StageHook func(ctx context.Context, stage string, state *TripState) error

func WithStageHook(hook StageHook) GraphOption {
	return func(options *graphOptions) { options.stageHook = hook }
}

func NewGraph(ctx context.Context, nodes *Nodes, opts ...GraphOption) (*Graph, error) {
	if nodes == nil {
		return nil, fmt.Errorf("agent: graph requires nodes")
	}
	options := graphOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	g := compose.NewGraph[*TripState, *TripState]()
	graph := &Graph{nodes: nodes}
	stages := []struct {
		key string
		fn  func(context.Context, *TripState) (*TripState, error)
	}{
		{nodeAdmit, graph.admit},
		{nodeParse, graph.parse},
		{nodeRetrieve, graph.retrieve},
		{nodeDraft, graph.draft},
		{nodeSchedule, graph.schedule},
		{nodeValidate, graph.validate},
		{nodeRepair, graph.repair},
		{nodePresent, graph.present},
	}
	for _, stage := range stages {
		key := stage.key
		fn := stage.fn
		if options.stageHook != nil {
			hook := options.stageHook
			fn = func(ctx context.Context, state *TripState) (*TripState, error) {
				out, err := stage.fn(ctx, state)
				if err != nil {
					return nil, err
				}
				out.Stage = key
				if err := hook(ctx, key, out); err != nil {
					return nil, err
				}
				return out, nil
			}
		}
		if err := g.AddLambdaNode(key, compose.InvokableLambda(fn)); err != nil {
			return nil, fmt.Errorf("agent: add %s node: %w", key, err)
		}
	}

	edges := [][2]string{
		{compose.START, nodeAdmit},
		{nodeAdmit, nodeParse},
		{nodeRetrieve, nodeDraft},
		{nodeDraft, nodeSchedule},
		{nodeSchedule, nodeValidate},
		{nodeRepair, nodeValidate},
		{nodePresent, compose.END},
	}
	for _, edge := range edges {
		if err := g.AddEdge(edge[0], edge[1]); err != nil {
			return nil, fmt.Errorf("agent: add edge %s->%s: %w", edge[0], edge[1], err)
		}
	}

	afterParse := compose.NewGraphBranch(func(_ context.Context, state *TripState) (string, error) {
		if len(state.Missing) > 0 {
			return compose.END, nil
		}
		return nodeRetrieve, nil
	}, map[string]bool{nodeRetrieve: true, compose.END: true})
	if err := g.AddBranch(nodeParse, afterParse); err != nil {
		return nil, fmt.Errorf("agent: add parse branch: %w", err)
	}

	afterValidate := compose.NewGraphBranch(graph.afterValidate, map[string]bool{
		nodeRepair:  true,
		nodePresent: true,
	})
	if err := g.AddBranch(nodeValidate, afterValidate); err != nil {
		return nil, fmt.Errorf("agent: add validate branch: %w", err)
	}

	runnable, err := g.Compile(ctx,
		compose.WithGraphName("trippilot"),
		compose.WithMaxRunSteps(32),
	)
	if err != nil {
		return nil, fmt.Errorf("agent: compile graph: %w", err)
	}
	graph.runnable = runnable
	return graph, nil
}

// Run executes the full pipeline and mutates the supplied state in place.
func (g *Graph) Run(ctx context.Context, state *TripState) (*TripState, error) {
	if state == nil {
		return nil, fmt.Errorf("agent: graph requires state")
	}
	out, err := g.runnable.Invoke(ctx, state)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (g *Graph) admit(_ context.Context, state *TripState) (*TripState, error) {
	if state == nil {
		return nil, &NodeError{Code: "invalid_state", Message: "状态为空"}
	}
	if strings.TrimSpace(state.RunID) == "" {
		return nil, &NodeError{Code: "missing_run_id", Message: "缺少 run_id"}
	}
	if strings.TrimSpace(state.Input) == "" {
		return nil, &NodeError{Code: "empty_input", Message: "行程需求不能为空"}
	}
	state.Status = StatusRunning
	return state, nil
}

func (g *Graph) parse(ctx context.Context, state *TripState) (*TripState, error) {
	out, err := g.nodes.Parse(ctx, ParseInput{RunID: state.RunID, Input: state.Input})
	if err != nil {
		return nil, err
	}
	state.Constraints = out.Constraints
	state.Missing = out.Missing
	state.LLM = addUsage(state.LLM, out.Usage)
	if len(out.Missing) > 0 {
		state.Status = StatusAwaitingUser
	}
	return state, nil
}

func (g *Graph) retrieve(ctx context.Context, state *TripState) (*TripState, error) {
	out, err := g.nodes.Retrieve(ctx, RetrieveInput{RunID: state.RunID, Constraints: state.Constraints})
	if err != nil {
		return nil, err
	}
	state.Candidates = out.Candidates
	return state, nil
}

func (g *Graph) draft(ctx context.Context, state *TripState) (*TripState, error) {
	out, err := g.nodes.Draft(ctx, DraftInput{
		RunID:       state.RunID,
		Constraints: state.Constraints,
		Candidates:  state.Candidates,
	})
	if err != nil {
		return nil, err
	}
	state.Draft = out.Draft
	state.LLM = addUsage(state.LLM, out.Usage)
	return state, nil
}

func (g *Graph) schedule(ctx context.Context, state *TripState) (*TripState, error) {
	plan, err := g.nodes.Schedule(ctx, ScheduleInput{
		RunID:       state.RunID,
		Constraints: state.Constraints,
		Draft:       state.Draft,
		POIs:        poiMap(state.Candidates),
	})
	if err != nil {
		return nil, err
	}
	state.Plan = plan
	return state, nil
}

func (g *Graph) validate(ctx context.Context, state *TripState) (*TripState, error) {
	out, err := g.nodes.Validate(ctx, ValidateInput{
		RunID:       state.RunID,
		Constraints: state.Constraints,
		Plan:        state.Plan,
		POIs:        poiMap(state.Candidates),
	})
	if err != nil {
		return nil, err
	}
	state.Violations = out.Violations

	metrics := MetricsFor(len(state.History), state.Plan, state.Violations)
	state.History = append(state.History, metrics)
	if state.Best == nil || better(metrics, MetricsFor(0, *state.Best, state.BestViolations)) {
		best := state.Plan.Clone()
		state.Best = &best
		state.BestViolations = append([]domain.Violation(nil), state.Violations...)
	}
	state.Suggestions = RelaxationSuggestions(state.Violations, state.Constraints)
	return state, nil
}

func (g *Graph) afterValidate(_ context.Context, state *TripState) (string, error) {
	if !domain.HasBlocking(state.Violations) {
		if state.StopReason == "" {
			state.StopReason = string(StopCleared)
		}
		return nodePresent, nil
	}

	stop, reason := stopFromHistory(state.History, defaultMaxRepairRounds, false)
	if !stop {
		return nodeRepair, nil
	}
	if state.Best != nil {
		state.Plan = state.Best.Clone()
		state.Violations = append([]domain.Violation(nil), state.BestViolations...)
	}
	state.StopReason = string(reason)
	state.Suggestions = RelaxationSuggestions(state.Violations, state.Constraints)
	return nodePresent, nil
}

func (g *Graph) repair(ctx context.Context, state *TripState) (*TripState, error) {
	round := len(state.History)
	out, err := g.nodes.Repair(ctx, RepairInput{
		RunID:       state.RunID,
		Constraints: state.Constraints,
		Draft:       state.Draft,
		Plan:        state.Plan,
		POIs:        poiMap(state.Candidates),
		Candidates:  state.Candidates,
		Violations:  state.Violations,
		Round:       round,
	})
	if err != nil {
		return nil, err
	}
	state.Draft = out.Draft
	state.Plan = out.Plan
	state.Violations = out.Violations
	state.LLM = addUsage(state.LLM, out.Usage)
	state.Round = round
	return state, nil
}

func (g *Graph) present(ctx context.Context, state *TripState) (*TripState, error) {
	out, err := g.nodes.Present(ctx, PresentInput{
		RunID:        state.RunID,
		Constraints:  state.Constraints,
		Plan:         state.Plan,
		Violations:   state.Violations,
		Reason:       presentReason(state),
		Explanations: state.Draft.Explanations,
		Suggestions:  state.Suggestions,
		Previous:     state.Previous,
		PreviousSeq:  state.PreviousSeq,
	})
	if err != nil {
		return nil, err
	}
	state.Snapshot = &out.Snapshot
	state.Status = out.Status
	return state, nil
}

func presentReason(state *TripState) string {
	if state.Mode == "adjustment" {
		return "adjustment"
	}
	if len(state.History) > 1 {
		return "repair"
	}
	return "initial"
}

func poiMap(candidates []domain.POI) map[string]domain.POI {
	pois := make(map[string]domain.POI, len(candidates))
	for _, poi := range candidates {
		if _, dup := pois[poi.ID]; dup {
			continue
		}
		pois[poi.ID] = poi
	}
	return pois
}
