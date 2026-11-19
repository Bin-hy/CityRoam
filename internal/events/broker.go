package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const defaultBuffer = 64

// Event is one persisted run event. Seq doubles as the SSE event id so clients
// can resume with Last-Event-ID.
type Event struct {
	ID        int64           `json:"id"`
	RunID     string          `json:"run_id"`
	Seq       int32           `json:"seq"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// Store persists events before they are broadcast, so replay always has a
// complete history even when no subscriber was connected.
type Store interface {
	AppendEvent(ctx context.Context, runID, typ string, payload json.RawMessage) (Event, error)
	ListEventsAfter(ctx context.Context, runID string, afterSeq int32) ([]Event, error)
}

type options struct {
	buffer int
	logger *slog.Logger
}

type Option func(*options)

func WithSubscriberBuffer(size int) Option {
	return func(o *options) {
		if size > 0 {
			o.buffer = size
		}
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(o *options) {
		if logger != nil {
			o.logger = logger
		}
	}
}

type subscriber struct {
	ch chan Event
}

// Broker fans persisted events out to per-run subscribers. Slow subscribers
// lose events instead of blocking publishing; they recover via replay.
type Broker struct {
	store  Store
	logger *slog.Logger
	buffer int

	mu     sync.Mutex
	subs   map[string]map[*subscriber]struct{}
	closed bool
}

func NewBroker(store Store, opts ...Option) (*Broker, error) {
	if store == nil {
		return nil, fmt.Errorf("events: broker requires a store")
	}
	o := options{buffer: defaultBuffer, logger: slog.Default()}
	for _, opt := range opts {
		opt(&o)
	}
	return &Broker{
		store:  store,
		logger: o.logger,
		buffer: o.buffer,
		subs:   make(map[string]map[*subscriber]struct{}),
	}, nil
}

// Publish persists the event first and broadcasts it afterwards. A failed
// write means nobody sees the event, which keeps DB and stream consistent.
func (b *Broker) Publish(ctx context.Context, runID, typ string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("events: encode payload: %w", err)
	}
	event, err := b.store.AppendEvent(ctx, runID, typ, raw)
	if err != nil {
		return fmt.Errorf("events: append event: %w", err)
	}

	dropped := 0
	b.mu.Lock()
	for sub := range b.subs[runID] {
		select {
		case sub.ch <- event:
		default:
			dropped++
		}
	}
	b.mu.Unlock()

	if dropped > 0 {
		b.logger.Warn("events: dropped events for slow subscribers",
			"run_id", runID, "count", dropped)
	}
	return nil
}

// Subscribe returns a channel of live events for one run and an unsubscribe
// function. The channel is closed after unsubscribe.
func (b *Broker) Subscribe(runID string) (<-chan Event, func()) {
	sub := &subscriber{ch: make(chan Event, b.buffer)}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		closed := make(chan Event)
		close(closed)
		return closed, func() {}
	}
	if b.subs[runID] == nil {
		b.subs[runID] = make(map[*subscriber]struct{})
	}
	b.subs[runID][sub] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			if subscribers, ok := b.subs[runID]; ok {
				if _, exists := subscribers[sub]; exists {
					delete(subscribers, sub)
					close(sub.ch)
				}
				if len(subscribers) == 0 {
					delete(b.subs, runID)
				}
			}
			b.mu.Unlock()
		})
	}
	return sub.ch, cancel
}

// Replay returns persisted events after the given sequence for reconnect.
func (b *Broker) Replay(ctx context.Context, runID string, afterSeq int32) ([]Event, error) {
	if afterSeq < 0 {
		afterSeq = 0
	}
	events, err := b.store.ListEventsAfter(ctx, runID, afterSeq)
	if err != nil {
		return nil, fmt.Errorf("events: replay: %w", err)
	}
	return events, nil
}

// Close unsubscribes every listener. Events already persisted stay readable.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for runID, subscribers := range b.subs {
		for sub := range subscribers {
			close(sub.ch)
		}
		delete(b.subs, runID)
	}
}

// SubscriberCount reports live subscribers for a run (test and metrics aid).
func (b *Broker) SubscriberCount(runID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs[runID])
}
