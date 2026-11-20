package agent

import (
	"context"

	"trippilot/internal/domain"
	"trippilot/internal/domain/scheduler"
	"trippilot/internal/domain/validator"
)

type Scheduler interface {
	Build(draft domain.DraftPlan, constraints domain.ConstraintSet, pois map[string]domain.POI, routes domain.RouteProvider, options domain.ScheduleOptions) (domain.Itinerary, error)
}

type Validator interface {
	Validate(plan domain.Itinerary, constraints domain.ConstraintSet, pois map[string]domain.POI, routes domain.RouteProvider) []domain.Violation
}

type validatorFunc func(domain.Itinerary, domain.ConstraintSet, map[string]domain.POI, domain.RouteProvider) []domain.Violation

func (f validatorFunc) Validate(plan domain.Itinerary, constraints domain.ConstraintSet, pois map[string]domain.POI, routes domain.RouteProvider) []domain.Violation {
	return f(plan, constraints, pois, routes)
}

func WithScheduler(schedule Scheduler) NodeOption {
	return func(n *Nodes) { n.scheduler = schedule }
}

func WithValidator(validate Validator) NodeOption {
	return func(n *Nodes) { n.validator = validate }
}

func WithRouteProvider(routes domain.RouteProvider) NodeOption {
	return func(n *Nodes) { n.routes = routes }
}

type ScheduleInput struct {
	RunID       string
	Constraints domain.ConstraintSet
	Draft       domain.DraftPlan
	POIs        map[string]domain.POI
	Research    *domain.ResearchBrief
}

// Schedule turns a draft into exact times, transit and costs, then publishes
// plan.drafted.
func (n *Nodes) Schedule(ctx context.Context, in ScheduleInput) (domain.Itinerary, error) {
	plan, err := n.scheduler.Build(in.Draft, in.Constraints, in.POIs, n.routes, scheduleOptions(in.Research))
	if err != nil {
		return domain.Itinerary{}, &NodeError{Code: "schedule_failed", Message: "排期失败", Err: err}
	}
	if err := n.events.Publish(ctx, in.RunID, "plan.drafted", map[string]any{"plan": plan}); err != nil {
		return domain.Itinerary{}, &NodeError{Code: "event_failed", Message: "发布排期事件失败", Err: err}
	}
	n.logger.Info("plan scheduled", "run_id", in.RunID, "days", len(plan.Days))
	return plan, nil
}

func scheduleOptions(research *domain.ResearchBrief) domain.ScheduleOptions {
	if research == nil || research.Accommodation == nil || research.Accommodation.Selected == nil {
		return domain.ScheduleOptions{}
	}
	accommodation := research.Accommodation
	return domain.ScheduleOptions{Lodging: &domain.LodgingAnchor{
		Hotel:           *accommodation.Selected,
		AnchorDays:      append([]int(nil), accommodation.AnchorDays...),
		IncludeInBudget: accommodation.IncludeInBudget,
	}}
}

type ValidateInput struct {
	RunID       string
	Constraints domain.ConstraintSet
	Plan        domain.Itinerary
	POIs        map[string]domain.POI
}

type ValidateOutput struct {
	Violations []domain.Violation
	Blocking   int
	Warnings   int
}

// Validate runs the deterministic C1-C12 checks and publishes the full list.
func (n *Nodes) Validate(ctx context.Context, in ValidateInput) (ValidateOutput, error) {
	violations := n.validator.Validate(in.Plan, in.Constraints, in.POIs, n.routes)
	counts := domain.CountByLevel(violations)

	out := ValidateOutput{
		Violations: violations,
		Blocking:   counts[domain.LevelBlocking],
		Warnings:   counts[domain.LevelWarning],
	}
	if err := n.events.Publish(ctx, in.RunID, "validation.result", map[string]any{
		"blocking":   out.Blocking,
		"warnings":   out.Warnings,
		"violations": violations,
	}); err != nil {
		return ValidateOutput{}, &NodeError{Code: "event_failed", Message: "发布校验事件失败", Err: err}
	}
	n.logger.Info("plan validated",
		"run_id", in.RunID, "blocking", out.Blocking, "warnings", out.Warnings)
	return out, nil
}

func defaultScheduler() Scheduler { return scheduler.New() }

func defaultValidator() Validator { return validatorFunc(validator.Validate) }
