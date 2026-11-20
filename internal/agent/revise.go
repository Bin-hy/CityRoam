package agent

import (
	"context"
	"fmt"
	"strings"

	"trippilot/internal/domain"
)

const (
	proposalRejectedStage = "proposal_rejected"
	maxReviseInput        = 2000
)

// resumeRevise keeps the same run and switches it to adjustment mode so the
// user's feedback becomes the next adjustment input.
func (r *Runner) resumeRevise(ctx context.Context, runID string, state *TripState, input string) error {
	feedback := strings.TrimSpace(input)
	if feedback == "" {
		return fmt.Errorf("runner: revise input is empty")
	}
	if len([]rune(feedback)) > maxReviseInput {
		return fmt.Errorf("runner: revise input exceeds %d characters", maxReviseInput)
	}
	if len(state.Plan.Days) == 0 {
		return fmt.Errorf("%w: run has no plan to revise", ErrInvalidRunStatus)
	}

	baseSeq := state.PreviousSeq
	if state.Snapshot != nil {
		baseSeq = state.Snapshot.Seq
	}
	if baseSeq <= 0 {
		return fmt.Errorf("%w: run has no base version", ErrInvalidRunStatus)
	}
	if len(state.Draft.Days) < len(state.Plan.Days) {
		state.Draft = domain.DraftFromPlan(state.Plan)
	}

	previous := state.Plan.Clone()
	state.Previous = &previous
	state.PreviousSeq = baseSeq
	state.Mode = "adjustment"
	state.Input = feedback
	state.Round = 0
	state.Proposal = nil
	state.Snapshot = nil
	state.Best = nil
	state.BestViolations = nil
	state.History = nil
	state.Suggestions = nil
	state.StopReason = ""
	state.ResearchQuestion = nil
	state.Error = nil
	state.Status = StatusQueued
	state.Stage = "adjust"

	if err := r.store.UpdateRunMode(ctx, runID, "adjustment"); err != nil {
		return fmt.Errorf("runner: update run mode: %w", err)
	}
	if err := r.saveState(ctx, state); err != nil {
		return err
	}
	if err := r.store.UpdateRunStatus(ctx, runID, StatusQueued, "adjust"); err != nil {
		return fmt.Errorf("runner: requeue revise run: %w", err)
	}
	return r.Start(ctx, runID)
}

// rejectProposal restores the version the proposal was based on and keeps the
// run awaiting, so the user can revise again or cancel explicitly.
func (r *Runner) rejectProposal(ctx context.Context, runID string, state *TripState) error {
	if state.Proposal == nil {
		return r.cancelRejected(ctx, runID, state)
	}
	if state.PreviousSeq <= 0 {
		return fmt.Errorf("%w: proposal has no base version", ErrInvalidRunStatus)
	}
	base, err := r.store.GetPlanVersion(ctx, state.TripID, state.PreviousSeq)
	if err != nil {
		return fmt.Errorf("runner: load base version: %w", err)
	}

	previous := base.Plan.Clone()
	state.Plan = base.Plan
	state.Constraints = base.Constraints
	state.Violations = base.Violations
	state.Draft = domain.DraftFromPlan(base.Plan)
	state.Previous = &previous
	state.PreviousSeq = base.Seq
	state.Proposal = nil
	state.Snapshot = nil
	state.Suggestions = nil
	state.StopReason = ""
	state.Status = StatusAwaitingUser
	state.Stage = proposalRejectedStage
	state.Error = nil

	if err := r.saveState(ctx, state); err != nil {
		return err
	}
	if err := r.store.UpdateRunStatus(ctx, runID, StatusAwaitingUser, proposalRejectedStage); err != nil {
		return fmt.Errorf("runner: update rejected proposal status: %w", err)
	}
	if err := r.events.Publish(ctx, runID, "run.awaiting_user", map[string]any{
		"stage":    proposalRejectedStage,
		"proposal": nil,
	}); err != nil {
		return fmt.Errorf("runner: publish proposal rejection: %w", err)
	}
	return nil
}

// cancelRejected preserves the legacy reject semantics for runs without a
// pending proposal (old clients and pre-v1.2 states).
func (r *Runner) cancelRejected(ctx context.Context, runID string, state *TripState) error {
	state.Status = StatusCancelled
	state.Stage = "rejected"
	if err := r.saveState(ctx, state); err != nil {
		return err
	}
	if err := r.store.UpdateRunStatus(ctx, runID, StatusCancelled, "rejected"); err != nil {
		return fmt.Errorf("runner: reject run: %w", err)
	}
	if err := r.events.Publish(ctx, runID, "run.cancelled", map[string]any{
		"reason": "rejected",
	}); err != nil {
		return fmt.Errorf("runner: publish run.cancelled: %w", err)
	}
	return nil
}
