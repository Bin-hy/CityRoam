package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"

	"trippilot/migrations"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := migrations.Up(db); err != nil {
		db.Close()
		t.Fatalf("apply migrations: %v", err)
	}
	db.Close()

	store, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

func TestStoreCore(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()

	requestID := "trip-" + strconv.FormatInt(suffix, 10)
	trip, err := store.CreateTrip(ctx, CreateTripParams{
		City:        "beijing",
		Days:        1,
		Constraints: json.RawMessage(`{}`),
		Status:      "parsing",
		RequestID:   &requestID,
	})
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}
	t.Cleanup(func() { _, _ = store.pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", trip.ID) })

	repeat, err := store.CreateTrip(ctx, CreateTripParams{
		City:        "beijing",
		Days:        1,
		Constraints: json.RawMessage(`{}`),
		Status:      "parsing",
		RequestID:   &requestID,
	})
	if err != nil {
		t.Fatalf("repeat create trip: %v", err)
	}
	if repeat.ID != trip.ID {
		t.Fatalf("repeat trip id = %s, want %s", repeat.ID, trip.ID)
	}

	runRequestID := "run-" + strconv.FormatInt(suffix, 10)
	run, err := store.CreateRun(ctx, CreateRunParams{
		TripID:      trip.ID,
		Mode:        "initial",
		Status:      "queued",
		Stage:       "admit",
		Input:       "北京一日游",
		Constraints: json.RawMessage(`{}`),
		RequestID:   &runRequestID,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	started := time.Now().UTC().Truncate(time.Millisecond)
	if err := store.SaveRunState(ctx, UpdateRunStateParams{
		ID:        run.ID,
		State:     json.RawMessage(`{"stage":"validate"}`),
		Stage:     "validate",
		Round:     1,
		Status:    "running",
		StartedAt: &started,
	}); err != nil {
		t.Fatalf("save run state: %v", err)
	}

	loadedRun, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if loadedRun.Stage != "validate" || loadedRun.Round != 1 || loadedRun.Status != "running" {
		t.Fatalf("run after save = %+v", loadedRun)
	}
	var state map[string]string
	if err := json.Unmarshal(loadedRun.State, &state); err != nil {
		t.Fatalf("decode run state: %v", err)
	}
	if state["stage"] != "validate" {
		t.Fatalf("run state = %v", state)
	}
	if startedAt := loadedRun.StartedAt; startedAt == nil || !startedAt.Equal(started) {
		t.Fatalf("started_at = %v, want %v", startedAt, started)
	}

	reason := "initial draft"
	first, err := store.SaveVersion(ctx, CreatePlanVersionParams{
		TripID:      trip.ID,
		RunID:       run.ID,
		Plan:        json.RawMessage(`{"city":"beijing"}`),
		Constraints: json.RawMessage(`{}`),
		Violations:  json.RawMessage(`[]`),
		Reason:      &reason,
		Status:      "draft",
	})
	if err != nil {
		t.Fatalf("save first version: %v", err)
	}
	second, err := store.SaveVersion(ctx, CreatePlanVersionParams{
		TripID:      trip.ID,
		RunID:       run.ID,
		Plan:        json.RawMessage(`{"city":"beijing","days":1}`),
		Constraints: json.RawMessage(`{}`),
		Violations:  json.RawMessage(`[]`),
		Status:      "draft",
	})
	if err != nil {
		t.Fatalf("save second version: %v", err)
	}
	if first.Seq != 1 || second.Seq != 2 {
		t.Fatalf("version seqs = %d, %d, want 1, 2", first.Seq, second.Seq)
	}

	for step := 1; step <= 2; step++ {
		event, err := store.AppendEvent(ctx, AppendEventParams{
			RunID:   run.ID,
			Type:    "stage",
			Payload: json.RawMessage(`{"step":` + strconv.FormatInt(int64(step), 10) + `}`),
		})
		if err != nil {
			t.Fatalf("append event %d: %v", step, err)
		}
		if event.Seq != int32(step) {
			t.Fatalf("event seq = %d, want %d", event.Seq, step)
		}
	}

	all, err := store.ListEventsAfter(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("list all events: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("events = %d, want 2", len(all))
	}
	tail, err := store.ListEventsAfter(ctx, run.ID, 1)
	if err != nil {
		t.Fatalf("list tail events: %v", err)
	}
	if len(tail) != 1 || tail[0].Seq != 2 {
		t.Fatalf("tail events = %+v, want seq 2", tail)
	}

	if err := store.SoftDeleteTrip(ctx, trip.ID); err != nil {
		t.Fatalf("soft delete trip: %v", err)
	}
	if _, err := store.GetTrip(ctx, trip.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("get deleted trip err = %v, want ErrNoRows", err)
	}
}

func TestStoreShareEval(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()

	requestID := "share-trip-" + strconv.FormatInt(suffix, 10)
	trip, err := store.CreateTrip(ctx, CreateTripParams{
		City:        "beijing",
		Days:        1,
		Constraints: json.RawMessage(`{}`),
		Status:      "completed",
		RequestID:   &requestID,
	})
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}
	t.Cleanup(func() { _, _ = store.pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", trip.ID) })

	run, err := store.CreateRun(ctx, CreateRunParams{
		TripID:      trip.ID,
		Mode:        "initial",
		Status:      "completed",
		Stage:       "present",
		Input:       "北京一日游",
		Constraints: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	version, err := store.SaveVersion(ctx, CreatePlanVersionParams{
		TripID:      trip.ID,
		RunID:       run.ID,
		Plan:        json.RawMessage(`{"city":"beijing"}`),
		Constraints: json.RawMessage(`{}`),
		Violations:  json.RawMessage(`[]`),
		Status:      "accepted",
	})
	if err != nil {
		t.Fatalf("save version: %v", err)
	}

	slug := "dup-" + strconv.FormatInt(suffix, 10)
	if _, err := store.InsertShareLink(ctx, slug, trip.ID, version.ID); err != nil {
		t.Fatalf("insert share link: %v", err)
	}
	if _, err := store.InsertShareLink(ctx, slug, trip.ID, version.ID); !isUniqueViolation(err) {
		t.Fatalf("duplicate slug err = %v, want unique violation", err)
	}

	link, err := store.CreateShareLink(ctx, trip.ID, version.ID)
	if err != nil {
		t.Fatalf("create share link: %v", err)
	}
	if len(link.Slug) != 12 {
		t.Fatalf("slug length = %d, want 12", len(link.Slug))
	}
	if _, err := store.GetShareLink(ctx, link.Slug); err != nil {
		t.Fatalf("get share link: %v", err)
	}
	if err := store.RevokeShareLink(ctx, link.Slug); err != nil {
		t.Fatalf("revoke share link: %v", err)
	}
	if _, err := store.GetShareLink(ctx, link.Slug); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("revoked link err = %v, want ErrNoRows", err)
	}

	caseID := "case-" + strconv.FormatInt(suffix, 10)
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), "DELETE FROM eval_cases WHERE id = $1", caseID)
	})
	if err := store.UpsertEvalCase(ctx, UpsertEvalCaseParams{
		ID:       caseID,
		Input:    "故宫一日游",
		Expected: json.RawMessage(`{"days":1}`),
		Tags:     []string{"smoke"},
		Enabled:  true,
	}); err != nil {
		t.Fatalf("upsert eval case: %v", err)
	}
	cases, err := store.ListEvalCases(ctx)
	if err != nil {
		t.Fatalf("list eval cases: %v", err)
	}
	found := false
	for _, evalCase := range cases {
		if evalCase.ID == caseID {
			found = true
		}
	}
	if !found {
		t.Fatalf("eval case %s not listed", caseID)
	}

	duration := int32(1200)
	if _, err := store.InsertEvalRun(ctx, InsertEvalRunParams{
		CaseID:     caseID,
		Metrics:    json.RawMessage(`{"hard_pass":0.9}`),
		Passed:     true,
		DurationMs: &duration,
	}); err != nil {
		t.Fatalf("insert eval run: %v", err)
	}
	runs, err := store.ListEvalRunsByCase(ctx, caseID)
	if err != nil {
		t.Fatalf("list eval runs: %v", err)
	}
	if len(runs) != 1 || !runs[0].Passed {
		t.Fatalf("eval runs = %+v, want one passing run", runs)
	}
}
