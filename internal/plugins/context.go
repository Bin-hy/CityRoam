package plugins

import (
	"context"
	"strings"
)

type runContextKey struct{}

// RunContext identifies the run a tool call belongs to. It deliberately holds
// no secrets, user input or tool results.
type RunContext struct {
	RunID  string
	TripID string
}

func WithRun(ctx context.Context, runID, tripID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	run := RunContext{RunID: strings.TrimSpace(runID), TripID: strings.TrimSpace(tripID)}
	if run.RunID == "" {
		return ctx
	}
	return context.WithValue(ctx, runContextKey{}, run)
}

func RunFromContext(ctx context.Context) (RunContext, bool) {
	if ctx == nil {
		return RunContext{}, false
	}
	run, ok := ctx.Value(runContextKey{}).(RunContext)
	if !ok || strings.TrimSpace(run.RunID) == "" {
		return RunContext{}, false
	}
	return run, true
}
