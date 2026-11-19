package events

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu     sync.Mutex
	seq    map[string]int32
	events map[string][]Event
	err    error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		seq:    make(map[string]int32),
		events: make(map[string][]Event),
	}
}

func (s *fakeStore) AppendEvent(_ context.Context, runID, typ string, payload json.RawMessage) (Event, error) {
	if s.err != nil {
		return Event{}, s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq[runID]++
	event := Event{
		ID:        int64(s.seq[runID]),
		RunID:     runID,
		Seq:       s.seq[runID],
		Type:      typ,
		Payload:   payload,
		CreatedAt: time.Unix(1700000000, 0).UTC(),
	}
	s.events[runID] = append(s.events[runID], event)
	return event, nil
}

func (s *fakeStore) ListEventsAfter(_ context.Context, runID string, afterSeq int32) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, 0)
	for _, event := range s.events[runID] {
		if event.Seq > afterSeq {
			out = append(out, event)
		}
	}
	return out, nil
}

func newTestBroker(t *testing.T, store Store, opts ...Option) *Broker {
	t.Helper()
	opts = append([]Option{WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))}, opts...)
	broker, err := NewBroker(store, opts...)
	if err != nil {
		t.Fatalf("new broker: %v", err)
	}
	return broker
}

func TestBrokerPublishAndSubscribe(t *testing.T) {
	store := newFakeStore()
	broker := newTestBroker(t, store)

	first, cancelFirst := broker.Subscribe("run-1")
	defer cancelFirst()
	second, cancelSecond := broker.Subscribe("run-1")
	defer cancelSecond()
	other, cancelOther := broker.Subscribe("run-2")
	defer cancelOther()

	if err := broker.Publish(context.Background(), "run-1", "run.started", map[string]any{"mode": "initial"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	for name, ch := range map[string]<-chan Event{"first": first, "second": second} {
		select {
		case event := <-ch:
			if event.Seq != 1 || event.Type != "run.started" || event.RunID != "run-1" {
				t.Fatalf("%s event = %+v", name, event)
			}
			if string(event.Payload) != `{"mode":"initial"}` {
				t.Fatalf("%s payload = %s", name, event.Payload)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s subscriber got no event", name)
		}
	}
	select {
	case event := <-other:
		t.Fatalf("other run got %+v", event)
	default:
	}

	persisted, err := store.ListEventsAfter(context.Background(), "run-1", 0)
	if err != nil || len(persisted) != 1 {
		t.Fatalf("persisted = %v, %v", persisted, err)
	}
}

func TestBrokerWritesBeforeBroadcast(t *testing.T) {
	store := newFakeStore()
	store.err = errors.New("db down")
	broker := newTestBroker(t, store)
	ch, cancel := broker.Subscribe("run-1")
	defer cancel()

	if err := broker.Publish(context.Background(), "run-1", "run.started", map[string]any{}); err == nil {
		t.Fatal("want publish error")
	}
	select {
	case event := <-ch:
		t.Fatalf("subscriber got %+v despite store failure", event)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestBrokerDropsSlowSubscriber(t *testing.T) {
	store := newFakeStore()
	broker := newTestBroker(t, store, WithSubscriberBuffer(1))
	slow, cancelSlow := broker.Subscribe("run-1")
	defer cancelSlow()
	fast, cancelFast := broker.Subscribe("run-1")
	defer cancelFast()

	for i := 0; i < 3; i++ {
		if err := broker.Publish(context.Background(), "run-1", "stage.completed", map[string]any{"i": i}); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
		select {
		case <-fast:
		case <-time.After(time.Second):
			t.Fatalf("fast subscriber missed event %d", i)
		}
	}

	select {
	case event := <-slow:
		if event.Seq != 1 {
			t.Fatalf("slow kept seq %d", event.Seq)
		}
	case <-time.After(time.Second):
		t.Fatal("slow subscriber got no event")
	}
}

func TestBrokerUnsubscribe(t *testing.T) {
	broker := newTestBroker(t, newFakeStore())
	ch, cancel := broker.Subscribe("run-1")
	if broker.SubscriberCount("run-1") != 1 {
		t.Fatalf("subscribers = %d", broker.SubscriberCount("run-1"))
	}

	cancel()
	cancel()
	if broker.SubscriberCount("run-1") != 0 {
		t.Fatalf("subscribers = %d", broker.SubscriberCount("run-1"))
	}
	if _, open := <-ch; open {
		t.Fatal("channel still open")
	}
}

func TestBrokerClose(t *testing.T) {
	broker := newTestBroker(t, newFakeStore())
	ch, _ := broker.Subscribe("run-1")
	broker.Close()

	if _, open := <-ch; open {
		t.Fatal("channel still open after close")
	}
	if broker.SubscriberCount("run-1") != 0 {
		t.Fatal("subscribers left after close")
	}
	closed, _ := broker.Subscribe("run-1")
	if _, open := <-closed; open {
		t.Fatal("subscribe after close returned an open channel")
	}
	if err := broker.Publish(context.Background(), "run-1", "run.started", map[string]any{}); err != nil {
		t.Fatalf("publish after close: %v", err)
	}
}

func TestBrokerReplay(t *testing.T) {
	store := newFakeStore()
	broker := newTestBroker(t, store)
	for i := 0; i < 3; i++ {
		if err := broker.Publish(context.Background(), "run-1", "stage.completed", map[string]any{"i": i}); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	events, err := broker.Replay(context.Background(), "run-1", 1)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(events) != 2 || events[0].Seq != 2 || events[1].Seq != 3 {
		t.Fatalf("replay = %+v", events)
	}

	events, err = broker.Replay(context.Background(), "run-1", -5)
	if err != nil || len(events) != 3 {
		t.Fatalf("replay negative = %d, %v", len(events), err)
	}
}

func TestNewBrokerRequiresStore(t *testing.T) {
	if _, err := NewBroker(nil); err == nil {
		t.Fatal("want error")
	}
}
