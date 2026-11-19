package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"trippilot/internal/agent"
	"trippilot/internal/events"
)

type fakeEventStore struct {
	mu     sync.Mutex
	events []events.Event
	seq    int32
}

func (s *fakeEventStore) AppendEvent(_ context.Context, runID, typ string, payload json.RawMessage) (events.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	event := events.Event{
		ID:        int64(s.seq),
		RunID:     runID,
		Seq:       s.seq,
		Type:      typ,
		Payload:   payload,
		CreatedAt: time.Unix(1700000000, 0).UTC(),
	}
	s.events = append(s.events, event)
	return event, nil
}

func (s *fakeEventStore) ListEventsAfter(_ context.Context, runID string, afterSeq int32) ([]events.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]events.Event, 0, len(s.events))
	for _, event := range s.events {
		if event.RunID == runID && event.Seq > afterSeq {
			out = append(out, event)
		}
	}
	return out, nil
}

func sseRouter(store TripStore, broker *events.Broker) http.Handler {
	return NewRouter(Deps{
		Version:          "test",
		Logger:           discardLogger(),
		AdminToken:       "secret",
		RateLimitPerHour: 100,
		Now:              func() time.Time { return time.Unix(1700000000, 0) },
		Store:            store,
		Runner:           &fakeRunner{},
		Broker:           broker,
	})
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) (string, string) {
	t.Helper()
	var id, typ string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read sse: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "id: "):
			id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			typ = strings.TrimPrefix(line, "event: ")
		case line == "" && id != "":
			return id, typ
		}
	}
}

func openStream(t *testing.T, server *httptest.Server, path string, headers map[string]string) (*http.Response, *bufio.Reader) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("stream status = %d", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		resp.Body.Close()
		t.Fatalf("content type = %q", contentType)
	}
	return resp, bufio.NewReader(resp.Body)
}

func waitSubscribers(t *testing.T, broker *events.Broker, runID string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if broker.SubscriberCount(runID) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("subscribers = %d, want %d", broker.SubscriberCount(runID), want)
}

func TestSSE(t *testing.T) {
	newFixture := func(t *testing.T) (*fakeTripStore, *events.Broker, string) {
		t.Helper()
		store, run := runFixture(t, agent.StatusRunning)
		broker, err := events.NewBroker(&fakeEventStore{})
		if err != nil {
			t.Fatalf("new broker: %v", err)
		}
		t.Cleanup(broker.Close)
		return store, broker, run.ID
	}

	t.Run("replays from Last-Event-ID and streams live events", func(t *testing.T) {
		store, broker, runID := newFixture(t)
		for _, typ := range []string{"run.started", "stage.completed"} {
			if err := broker.Publish(context.Background(), runID, typ, map[string]int{"n": 1}); err != nil {
				t.Fatalf("publish: %v", err)
			}
		}
		server := httptest.NewServer(sseRouter(store, broker))
		t.Cleanup(server.Close)

		resp, reader := openStream(t, server, "/api/runs/"+runID+"/events", map[string]string{"Last-Event-ID": "1"})
		if id, typ := readSSEEvent(t, reader); id != "2" || typ != "stage.completed" {
			t.Fatalf("replay = %s/%s", id, typ)
		}
		waitSubscribers(t, broker, runID, 1)
		if err := broker.Publish(context.Background(), runID, "plan.updated", map[string]int{"n": 2}); err != nil {
			t.Fatalf("publish live: %v", err)
		}
		if id, typ := readSSEEvent(t, reader); id != "3" || typ != "plan.updated" {
			t.Fatalf("live = %s/%s", id, typ)
		}

		resp.Body.Close()
		waitSubscribers(t, broker, runID, 0)
	})

	t.Run("supports the after query parameter", func(t *testing.T) {
		store, broker, runID := newFixture(t)
		for i := 0; i < 3; i++ {
			if err := broker.Publish(context.Background(), runID, "plan.updated", map[string]int{"n": i}); err != nil {
				t.Fatalf("publish: %v", err)
			}
		}
		server := httptest.NewServer(sseRouter(store, broker))
		t.Cleanup(server.Close)

		resp, reader := openStream(t, server, "/api/runs/"+runID+"/events?after=2", nil)
		defer resp.Body.Close()
		if id, typ := readSSEEvent(t, reader); id != "3" || typ != "plan.updated" {
			t.Fatalf("replay = %s/%s", id, typ)
		}
	})

	t.Run("sends heartbeats while idle", func(t *testing.T) {
		store, broker, runID := newFixture(t)
		previous := sseHeartbeat
		sseHeartbeat = 20 * time.Millisecond
		t.Cleanup(func() { sseHeartbeat = previous })

		server := httptest.NewServer(sseRouter(store, broker))
		t.Cleanup(server.Close)
		resp, reader := openStream(t, server, "/api/runs/"+runID+"/events", nil)
		defer resp.Body.Close()

		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read heartbeat: %v", err)
		}
		if line != ": ping\n" {
			t.Fatalf("heartbeat = %q", line)
		}
	})

	t.Run("validates cursor and run", func(t *testing.T) {
		store, broker, runID := newFixture(t)
		router := sseRouter(store, broker)

		for _, tc := range []struct {
			name   string
			path   string
			status int
		}{
			{"bad cursor", "/api/runs/" + runID + "/events?after=abc", http.StatusBadRequest},
			{"missing run", "/api/runs/nope/events", http.StatusNotFound},
		} {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.status {
				t.Fatalf("%s: status = %d body=%s", tc.name, rec.Code, rec.Body.String())
			}
		}
	})
}
