package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"trippilot/internal/domain"
)

type fakeSource struct {
	mu    sync.Mutex
	calls []domain.POIQuery
	fn    func(ctx context.Context, query domain.POIQuery) ([]domain.POI, error)
}

func (f *fakeSource) SearchCandidates(ctx context.Context, query domain.POIQuery) ([]domain.POI, error) {
	f.mu.Lock()
	f.calls = append(f.calls, query)
	f.mu.Unlock()
	return f.fn(ctx, query)
}

func (f *fakeSource) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeSource) queryAt(i int) domain.POIQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[i]
}

func makePOIs(prefix string, count int) []domain.POI {
	pois := make([]domain.POI, 0, count)
	for i := 0; i < count; i++ {
		pois = append(pois, domain.POI{
			ID:          fmt.Sprintf("%s-%d", prefix, i),
			City:        "beijing",
			Name:        fmt.Sprintf("%s景点%d", prefix, i),
			Category:    "历史",
			Lat:         39.80 + float64(i)/1000,
			Lng:         116.30 + float64(i)/1000,
			CoordSystem: domain.CoordSystemGCJ02,
		})
	}
	return pois
}

func TestRetrieveNode(t *testing.T) {
	base := domain.ConstraintSet{City: "beijing", Days: 2}

	t.Run("merges dedupes and publishes", func(t *testing.T) {
		snapshotPOIs := makePOIs("snap", 50)
		externalPOIs := append([]domain.POI{{
			ID:     "amap:dup",
			City:   "beijing",
			Name:   "snap景点0",
			Lat:    39.80,
			Lng:    116.30,
			Source: "amap",
		}}, makePOIs("ext", 20)...)

		snapshot := &fakeSource{fn: func(ctx context.Context, q domain.POIQuery) ([]domain.POI, error) {
			return snapshotPOIs, nil
		}}
		external := &fakeSource{fn: func(ctx context.Context, q domain.POIQuery) ([]domain.POI, error) {
			return externalPOIs, nil
		}}
		events := &eventRecorder{}
		nodes := NewNodes(nil, events,
			WithSnapshotSource(snapshot),
			WithExternalSource(external),
			WithNodeLogger(discardLogger()),
		)

		out, err := nodes.Retrieve(context.Background(), RetrieveInput{RunID: "run-1", Constraints: base})
		if err != nil {
			t.Fatalf("retrieve: %v", err)
		}
		if len(out.Candidates) != 60 {
			t.Fatalf("candidates = %d, want 60", len(out.Candidates))
		}
		if out.Candidates[0].ID != "snap-0" {
			t.Fatalf("first candidate = %s", out.Candidates[0].ID)
		}
		seen := map[string]bool{}
		for _, poi := range out.Candidates {
			if seen[poi.ID] {
				t.Fatalf("duplicate id %s", poi.ID)
			}
			seen[poi.ID] = true
			if poi.Name == "snap景点0" && poi.ID != "snap-0" {
				t.Fatalf("duplicate place kept: %+v", poi)
			}
		}
		got := events.types()
		if len(got) != 1 || got[0] != "candidate.retrieved" {
			t.Fatalf("events = %v", got)
		}
		payload := events.events[0].payload.(map[string]any)
		if payload["count"] != 60 || payload["attempts"] != 1 {
			t.Fatalf("payload = %v", payload)
		}
		if snapshot.callCount() != 1 || external.callCount() != 1 {
			t.Fatalf("calls: snap=%d ext=%d", snapshot.callCount(), external.callCount())
		}
	})

	t.Run("must visit triggers one widened retry", func(t *testing.T) {
		mustVisitPOI := domain.POI{
			ID: "amap:universal", City: "beijing", Name: "环球影城",
			Lat: 39.855, Lng: 116.674, Source: "amap",
		}
		snapshot := &fakeSource{fn: func(ctx context.Context, q domain.POIQuery) ([]domain.POI, error) {
			return makePOIs("snap", 45), nil
		}}
		external := &fakeSource{fn: func(ctx context.Context, q domain.POIQuery) ([]domain.POI, error) {
			if len(q.Keywords) == 0 {
				return nil, nil
			}
			return []domain.POI{mustVisitPOI}, nil
		}}
		events := &eventRecorder{}
		nodes := NewNodes(nil, events,
			WithSnapshotSource(snapshot),
			WithExternalSource(external),
			WithNodeLogger(discardLogger()),
		)

		constraints := base
		constraints.MustVisit = []string{"环球影城"}
		out, err := nodes.Retrieve(context.Background(), RetrieveInput{RunID: "run-2", Constraints: constraints})
		if err != nil {
			t.Fatalf("retrieve: %v", err)
		}
		if out.Attempts != 2 || len(out.Candidates) != 46 {
			t.Fatalf("output = %+v (candidates=%d)", out, len(out.Candidates))
		}
		if external.callCount() != 2 {
			t.Fatalf("external calls = %d", external.callCount())
		}
		widened := external.queryAt(1)
		if len(widened.Keywords) != 1 || widened.Keywords[0] != "环球影城" {
			t.Fatalf("widened query = %+v", widened)
		}
		found := false
		for _, poi := range out.Candidates {
			if poi.ID == mustVisitPOI.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("must-visit POI missing from candidates")
		}
		if got := events.types(); len(got) != 1 || got[0] != "candidate.retrieved" {
			t.Fatalf("events = %v", got)
		}
	})

	t.Run("unfindable must visit fails after retry", func(t *testing.T) {
		snapshot := &fakeSource{fn: func(ctx context.Context, q domain.POIQuery) ([]domain.POI, error) {
			return makePOIs("snap", 45), nil
		}}
		external := &fakeSource{fn: func(ctx context.Context, q domain.POIQuery) ([]domain.POI, error) {
			return nil, nil
		}}
		nodes := NewNodes(nil, &eventRecorder{},
			WithSnapshotSource(snapshot),
			WithExternalSource(external),
			WithNodeLogger(discardLogger()),
		)

		constraints := base
		constraints.MustVisit = []string{"不存在的地点"}
		_, err := nodes.Retrieve(context.Background(), RetrieveInput{Constraints: constraints})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "must_visit_missing" {
			t.Fatalf("err = %v", err)
		}
		if snapshot.callCount() != 2 || external.callCount() != 2 {
			t.Fatalf("calls: snap=%d ext=%d", snapshot.callCount(), external.callCount())
		}
	})

	t.Run("external failure is soft", func(t *testing.T) {
		snapshot := &fakeSource{fn: func(ctx context.Context, q domain.POIQuery) ([]domain.POI, error) {
			return makePOIs("snap", 57), nil
		}}
		external := &fakeSource{fn: func(ctx context.Context, q domain.POIQuery) ([]domain.POI, error) {
			return nil, errors.New("quota exceeded")
		}}
		events := &eventRecorder{}
		nodes := NewNodes(nil, events,
			WithSnapshotSource(snapshot),
			WithExternalSource(external),
			WithNodeLogger(discardLogger()),
		)

		out, err := nodes.Retrieve(context.Background(), RetrieveInput{Constraints: base})
		if err != nil {
			t.Fatalf("retrieve: %v", err)
		}
		if len(out.Candidates) != 57 {
			t.Fatalf("candidates = %d", len(out.Candidates))
		}
		if len(events.types()) != 1 {
			t.Fatalf("events = %v", events.types())
		}
	})

	t.Run("snapshot failure aborts", func(t *testing.T) {
		snapshot := &fakeSource{fn: func(ctx context.Context, q domain.POIQuery) ([]domain.POI, error) {
			return nil, errors.New("db down")
		}}
		nodes := NewNodes(nil, &eventRecorder{},
			WithSnapshotSource(snapshot),
			WithNodeLogger(discardLogger()),
		)
		_, err := nodes.Retrieve(context.Background(), RetrieveInput{Constraints: base})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "retrieve_failed" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("missing source", func(t *testing.T) {
		nodes := NewNodes(nil, &eventRecorder{}, WithNodeLogger(discardLogger()))
		_, err := nodes.Retrieve(context.Background(), RetrieveInput{Constraints: base})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "source_unavailable" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("event failure aborts", func(t *testing.T) {
		snapshot := &fakeSource{fn: func(ctx context.Context, q domain.POIQuery) ([]domain.POI, error) {
			return makePOIs("snap", 50), nil
		}}
		nodes := NewNodes(nil, &eventRecorder{err: errors.New("db down")},
			WithSnapshotSource(snapshot),
			WithNodeLogger(discardLogger()),
		)
		_, err := nodes.Retrieve(context.Background(), RetrieveInput{Constraints: base})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "event_failed" {
			t.Fatalf("err = %v", err)
		}
	})
}
