package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"trippilot/internal/domain"
)

type draftScheduler struct {
	mutate func(*domain.Itinerary)
}

func (s *draftScheduler) Build(draft domain.DraftPlan, constraints domain.ConstraintSet, pois map[string]domain.POI, _ domain.RouteProvider) (domain.Itinerary, error) {
	plan := domain.Itinerary{City: constraints.City}
	for dayIndex, day := range draft.Days {
		dayPlan := domain.DayPlan{Date: day.Date}
		for itemIndex, item := range day.Items {
			poi := pois[item.POIID]
			block := domain.Block{
				ID:          fmt.Sprintf("b-%d-%d", dayIndex, itemIndex),
				Type:        item.Type,
				POIID:       item.POIID,
				Title:       poi.Name,
				DurationMin: item.DurationMin,
			}
			if poi.TicketYuan != nil {
				block.CostYuan = *poi.TicketYuan
			}
			dayPlan.Blocks = append(dayPlan.Blocks, block)
			dayPlan.DayCostYuan += block.CostYuan
			if item.Type == domain.BlockTypePOI {
				plan.Totals.POICount++
			}
			plan.Totals.CostYuan += block.CostYuan
		}
		plan.Days = append(plan.Days, dayPlan)
	}
	if s.mutate != nil {
		s.mutate(&plan)
	}
	return plan, nil
}

func adjustmentFixture(t *testing.T) (AdjustInput, *draftScheduler) {
	t.Helper()
	constraints := domain.ConstraintSet{City: "beijing", Days: 2, StartDate: "2026-11-20"}
	pois := map[string]domain.POI{
		"p0": {ID: "p0", City: "beijing", Name: "故宫", Category: "历史"},
		"p1": {ID: "p1", City: "beijing", Name: "天坛", Category: "历史"},
		"p2": {ID: "p2", City: "beijing", Name: "国博", Category: "历史"},
	}
	draft := domain.DraftPlan{Days: []domain.DraftDay{
		{Date: "2026-11-20", Items: []domain.DraftItem{{POIID: "p0", Type: domain.ItemTypePOI, DurationMin: 90}}},
		{Date: "2026-11-21", Items: []domain.DraftItem{{POIID: "p1", Type: domain.ItemTypePOI, DurationMin: 90}}},
	}}
	scheduler := &draftScheduler{}
	basePlan, err := scheduler.Build(draft, constraints, pois, nil)
	if err != nil {
		t.Fatalf("build base: %v", err)
	}
	return AdjustInput{
		RunID:       "run-1",
		Input:       "把第一天换成别的博物馆",
		Constraints: constraints,
		BasePlan:    basePlan,
		BaseDraft:   draft,
		POIs:        pois,
		BaseVersion: 2,
	}, scheduler
}

func adjustmentSource() *fakeSource {
	return &fakeSource{fn: func(context.Context, domain.POIQuery) ([]domain.POI, error) {
		return []domain.POI{{ID: "p2", City: "beijing", Name: "国博", Category: "历史"}}, nil
	}}
}

func TestAdjustment(t *testing.T) {
	intentReply := json.RawMessage(`{"scope":"day","affected_days":[0],"summary":"换掉第一天"}`)
	patchReply := json.RawMessage(`{"day_index":0,"items":[{"poi_id":"p2","type":"poi","duration_min":90}],"reason":"第一天换成国博"}`)

	t.Run("proposes a scoped patch", func(t *testing.T) {
		in, scheduler := adjustmentFixture(t)
		events := &eventRecorder{}
		llm := &fakeLLM{replies: []json.RawMessage{intentReply, patchReply}}
		nodes := NewNodes(llm, events,
			WithSnapshotSource(adjustmentSource()),
			WithScheduler(scheduler),
			WithValidator(&fakeValidator{}),
			WithNodeLogger(discardLogger()),
		)

		out, err := nodes.Adjust(context.Background(), in)
		if err != nil {
			t.Fatalf("adjust: %v", err)
		}
		if out.Intent.Scope != "day" || len(out.Intent.AffectedDays) != 1 || out.Intent.AffectedDays[0] != 0 {
			t.Fatalf("intent = %+v", out.Intent)
		}
		if len(out.Plan.Days) != 2 {
			t.Fatalf("plan = %+v", out.Plan)
		}
		if out.Plan.Days[0].Blocks[0].POIID != "p2" {
			t.Fatalf("day 0 not patched: %+v", out.Plan.Days[0])
		}
		if out.Plan.Days[1].Blocks[0].POIID != "p1" {
			t.Fatalf("day 1 changed: %+v", out.Plan.Days[1])
		}
		if out.Proposal.BaseVersion != 2 || out.Proposal.Scope != "day" || out.Proposal.DayIndex != 0 {
			t.Fatalf("proposal = %+v", out.Proposal)
		}
		if out.Proposal.Summary != "换掉第一天" || !strings.Contains(out.Proposal.Impact, "费用") {
			t.Fatalf("proposal text = %+v", out.Proposal)
		}
		if len(out.Alternatives) == 0 || out.Alternatives[0].ID != "p2" {
			t.Fatalf("alternatives = %+v", out.Alternatives)
		}
		if len(llm.calls) != 2 {
			t.Fatalf("llm calls = %d", len(llm.calls))
		}
		if string(llm.calls[0].schema) != string(AdjustmentIntentSchema) {
			t.Fatal("intent call used the wrong schema")
		}
		if string(llm.calls[1].schema) != string(DayPatchSchema) || !strings.Contains(llm.calls[1].prompt, "国博") {
			t.Fatalf("patch prompt missing alternatives: %s", llm.calls[1].prompt)
		}
	})

	t.Run("unknown intent asks the user", func(t *testing.T) {
		in, scheduler := adjustmentFixture(t)
		llm := &fakeLLM{reply: json.RawMessage(`{"scope":"unknown","affected_days":[]}`)}
		nodes := NewNodes(llm, &eventRecorder{},
			WithSnapshotSource(adjustmentSource()),
			WithScheduler(scheduler),
			WithValidator(&fakeValidator{}),
			WithNodeLogger(discardLogger()),
		)

		_, err := nodes.Adjust(context.Background(), in)
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "adjustment_needs_input" {
			t.Fatalf("err = %v", err)
		}
		if len(llm.calls) != 1 {
			t.Fatalf("llm calls = %d", len(llm.calls))
		}
	})

	t.Run("out of range day asks the user", func(t *testing.T) {
		in, scheduler := adjustmentFixture(t)
		llm := &fakeLLM{reply: json.RawMessage(`{"scope":"day","affected_days":[3]}`)}
		nodes := NewNodes(llm, &eventRecorder{},
			WithSnapshotSource(adjustmentSource()),
			WithScheduler(scheduler),
			WithValidator(&fakeValidator{}),
			WithNodeLogger(discardLogger()),
		)

		_, err := nodes.Adjust(context.Background(), in)
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "adjustment_needs_input" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("patch outside the affected days is rejected", func(t *testing.T) {
		in, scheduler := adjustmentFixture(t)
		wrongDay := json.RawMessage(`{"day_index":1,"items":[{"poi_id":"p2","type":"poi","duration_min":90}],"reason":"换第二天"}`)
		llm := &fakeLLM{replies: []json.RawMessage{intentReply, wrongDay, wrongDay}}
		nodes := NewNodes(llm, &eventRecorder{},
			WithSnapshotSource(adjustmentSource()),
			WithScheduler(scheduler),
			WithValidator(&fakeValidator{}),
			WithNodeLogger(discardLogger()),
		)

		_, err := nodes.Adjust(context.Background(), in)
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "adjust_invalid" {
			t.Fatalf("err = %v", err)
		}
		if len(llm.calls) != 3 {
			t.Fatalf("llm calls = %d", len(llm.calls))
		}
		if !strings.Contains(llm.calls[2].prompt, "受影响的天") {
			t.Fatalf("retry prompt missing scope feedback: %s", llm.calls[2].prompt)
		}
	})

	t.Run("scheduler changes to untouched days are rejected", func(t *testing.T) {
		in, _ := adjustmentFixture(t)
		baseScheduler := &draftScheduler{}
		basePlan, err := baseScheduler.Build(in.BaseDraft, in.Constraints, in.POIs, nil)
		if err != nil {
			t.Fatalf("base: %v", err)
		}
		in.BasePlan = basePlan
		mutating := &draftScheduler{mutate: func(plan *domain.Itinerary) {
			plan.Days[1].Blocks[0].Title = "被改动"
		}}
		llm := &fakeLLM{replies: []json.RawMessage{intentReply, patchReply}}
		nodes := NewNodes(llm, &eventRecorder{},
			WithSnapshotSource(adjustmentSource()),
			WithScheduler(mutating),
			WithValidator(&fakeValidator{}),
			WithNodeLogger(discardLogger()),
		)

		_, err = nodes.Adjust(context.Background(), in)
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "adjustment_scope_violation" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("missing base plan fails", func(t *testing.T) {
		nodes := NewNodes(&fakeLLM{}, &eventRecorder{}, WithNodeLogger(discardLogger()))
		_, err := nodes.Adjust(context.Background(), AdjustInput{})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "adjustment_base_missing" {
			t.Fatalf("err = %v", err)
		}
	})
}

type countingBlockingLLM struct {
	started chan struct{}
}

func (b *countingBlockingLLM) GenerateJSON(ctx context.Context, _ string, _ json.RawMessage) (json.RawMessage, Usage, error) {
	b.started <- struct{}{}
	<-ctx.Done()
	return nil, Usage{}, ctx.Err()
}

func TestConcurrency(t *testing.T) {
	runs := []RunRecord{
		runnerRun("run-1", StatusQueued, "北京一天"),
		runnerRun("run-2", StatusQueued, "北京一天"),
		runnerRun("run-3", StatusQueued, "北京一天"),
		runnerRun("run-4", StatusQueued, "北京一天"),
	}
	store := newFakeRunStore(runs...)
	events := &eventRecorder{}
	llm := &countingBlockingLLM{started: make(chan struct{}, 4)}
	runner := newTestRunner(t, store, events, llm, WithRunnerConcurrency(3))

	for _, run := range runs {
		if err := runner.Start(context.Background(), run.ID); err != nil {
			t.Fatalf("start %s: %v", run.ID, err)
		}
	}
	for i := 0; i < 3; i++ {
		select {
		case <-llm.started:
		case <-time.After(5 * time.Second):
			t.Fatal("three runs did not start")
		}
	}

	if running, queued := countStatuses(store, runs); running != 3 || queued != 1 {
		t.Fatalf("running=%d queued=%d", running, queued)
	}

	for _, run := range runs {
		if err := runner.Cancel(context.Background(), run.ID); err != nil {
			t.Fatalf("cancel %s: %v", run.ID, err)
		}
	}
	runner.Wait()

	running, queued := countStatuses(store, runs)
	if running != 0 || queued != 0 {
		t.Fatalf("running=%d queued=%d", running, queued)
	}
	for _, run := range runs {
		if got := store.statusOf(run.ID); got != StatusCancelled {
			t.Fatalf("%s status = %s", run.ID, got)
		}
	}
}

func countStatuses(store *fakeRunStore, runs []RunRecord) (running, queued int) {
	for _, run := range runs {
		switch store.statusOf(run.ID) {
		case StatusRunning:
			running++
		case StatusQueued:
			queued++
		}
	}
	return running, queued
}

func TestIdempotency(t *testing.T) {
	store := newFakeRunStore(runnerRun("run-1", StatusQueued, "北京一天"))
	events := &eventRecorder{}
	llm := &fakeLLM{replies: []json.RawMessage{
		json.RawMessage(`{"city":"北京","days":1}`),
		graphDraftReply(),
	}}
	runner := newTestRunner(t, store, events, llm)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs[index] = runner.StartRequest(context.Background(), "run-1", "req-1")
		}(i)
	}
	wg.Wait()
	runner.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("start request: %v", err)
		}
	}
	if countString(events.types(), "run.started") != 1 {
		t.Fatalf("events = %v", events.types())
	}
	if got := store.statusOf("run-1"); got != StatusAwaitingUser {
		t.Fatalf("status = %s", got)
	}

	if err := runner.StartRequest(context.Background(), "run-1", "req-1"); err != nil {
		t.Fatalf("duplicate request: %v", err)
	}
	runner.Wait()
	if countString(events.types(), "run.started") != 1 {
		t.Fatalf("duplicate request restarted the run: %v", events.types())
	}
}
