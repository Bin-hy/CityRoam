package plugins

import (
	"context"
	"testing"
)

func TestRunContext(t *testing.T) {
	if _, ok := RunFromContext(context.Background()); ok {
		t.Fatal("plain context must not carry a run")
	}
	ctx := WithRun(context.Background(), "run-1", "trip-1")
	run, ok := RunFromContext(ctx)
	if !ok || run.RunID != "run-1" || run.TripID != "trip-1" {
		t.Fatalf("run = %+v ok=%v", run, ok)
	}
	if _, ok := RunFromContext(WithRun(context.Background(), "   ", "trip-1")); ok {
		t.Fatal("empty run id must be ignored")
	}
}
