package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type llmCall struct {
	prompt string
	schema json.RawMessage
}

type fakeLLM struct {
	reply   json.RawMessage
	replies []json.RawMessage
	usage   Usage
	err     error
	calls   []llmCall
}

func (f *fakeLLM) GenerateJSON(ctx context.Context, prompt string, schema json.RawMessage) (json.RawMessage, Usage, error) {
	index := len(f.calls)
	f.calls = append(f.calls, llmCall{prompt: prompt, schema: schema})
	if f.err != nil {
		return nil, f.usage, f.err
	}
	if index < len(f.replies) {
		return f.replies[index], f.usage, nil
	}
	return f.reply, f.usage, nil
}

type recordedEvent struct {
	runID   string
	typ     string
	payload any
}

type eventRecorder struct {
	events []recordedEvent
	err    error
}

func (r *eventRecorder) Publish(ctx context.Context, runID string, typ string, payload any) error {
	if r.err != nil {
		return r.err
	}
	r.events = append(r.events, recordedEvent{runID: runID, typ: typ, payload: payload})
	return nil
}

func (r *eventRecorder) types() []string {
	out := make([]string, len(r.events))
	for i, event := range r.events {
		out[i] = event.typ
	}
	return out
}

func TestParseNode(t *testing.T) {
	fixed := time.Date(2026, 11, 19, 10, 0, 0, 0, time.Local)
	newNodes := func(reply string, events *eventRecorder) (*Nodes, *fakeLLM) {
		llm := &fakeLLM{
			reply: json.RawMessage(reply),
			usage: Usage{Model: "deepseek-chat", TotalTokens: 42, Attempts: 1},
		}
		return NewNodes(llm, events,
			WithNodeClock(func() time.Time { return fixed }),
			WithNodeLogger(discardLogger()),
		), llm
	}

	t.Run("valid input publishes parsed", func(t *testing.T) {
		events := &eventRecorder{}
		nodes, llm := newNodes(`{"city":"北京","days":2,"budget":{"total_yuan":800,"hard":true},"interests":["历史"," 历史 ","咖啡"]}`, events)

		out, err := nodes.Parse(context.Background(), ParseInput{RunID: "run-1", Input: "北京两天，预算800，喜欢历史和咖啡"})
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if out.Constraints.City != "beijing" || out.Constraints.Days != 2 {
			t.Fatalf("constraints = %+v", out.Constraints)
		}
		if out.Constraints.Pace != "normal" || out.Constraints.PartySize != 1 {
			t.Fatalf("defaults not applied: %+v", out.Constraints)
		}
		if out.Constraints.StartDate != "2026-11-20" {
			t.Fatalf("start date = %s", out.Constraints.StartDate)
		}
		if len(out.Constraints.Interests) != 2 {
			t.Fatalf("interests = %+v", out.Constraints.Interests)
		}
		if len(out.Missing) != 0 || out.Usage.TotalTokens != 42 {
			t.Fatalf("output = %+v", out)
		}
		if got := events.types(); len(got) != 1 || got[0] != "constraint.parsed" {
			t.Fatalf("events = %v", got)
		}
		if events.events[0].runID != "run-1" {
			t.Fatalf("run id = %s", events.events[0].runID)
		}
		if len(llm.calls) != 1 || !strings.Contains(llm.calls[0].prompt, "2026-11-19") {
			t.Fatalf("prompt missing today: %s", llm.calls[0].prompt)
		}
		if string(llm.calls[0].schema) != string(ConstraintSchema) {
			t.Fatal("wrong schema")
		}
	})

	t.Run("missing city and days asks user", func(t *testing.T) {
		events := &eventRecorder{}
		nodes, _ := newNodes(`{"city":"","days":0}`, events)

		out, err := nodes.Parse(context.Background(), ParseInput{RunID: "run-2", Input: "去玩两天"})
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if len(out.Missing) != 2 || out.Missing[0] != "city" || out.Missing[1] != "days" {
			t.Fatalf("missing = %v", out.Missing)
		}
		if got := events.types(); len(got) != 1 || got[0] != "constraint.needs_input" {
			t.Fatalf("events = %v", got)
		}
		payload, ok := events.events[0].payload.(map[string]any)
		if !ok {
			t.Fatalf("payload = %T", events.events[0].payload)
		}
		if missing, _ := payload["missing"].([]string); len(missing) != 2 {
			t.Fatalf("payload missing = %v", payload)
		}
	})

	t.Run("invalid days rejected", func(t *testing.T) {
		nodes, _ := newNodes(`{"city":"beijing","days":9}`, &eventRecorder{})
		_, err := nodes.Parse(context.Background(), ParseInput{Input: "北京九天"})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "invalid_constraints" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("llm failure surfaces", func(t *testing.T) {
		llm := &fakeLLM{err: errors.New("timeout")}
		nodes := NewNodes(llm, &eventRecorder{}, WithNodeLogger(discardLogger()))
		_, err := nodes.Parse(context.Background(), ParseInput{Input: "北京"})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "parse_llm_failed" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("empty input never calls llm", func(t *testing.T) {
		nodes, llm := newNodes(`{}`, &eventRecorder{})
		_, err := nodes.Parse(context.Background(), ParseInput{Input: "   "})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "empty_input" {
			t.Fatalf("err = %v", err)
		}
		if len(llm.calls) != 0 {
			t.Fatalf("calls = %d", len(llm.calls))
		}
	})

	t.Run("event failure fails the node", func(t *testing.T) {
		nodes, _ := newNodes(`{"city":"beijing","days":2}`, &eventRecorder{err: errors.New("db down")})
		_, err := nodes.Parse(context.Background(), ParseInput{Input: "北京两天"})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "event_failed" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("invalid json from llm rejected", func(t *testing.T) {
		nodes, _ := newNodes(`not json`, &eventRecorder{})
		_, err := nodes.Parse(context.Background(), ParseInput{Input: "北京两天"})
		var nodeErr *NodeError
		if !errors.As(err, &nodeErr) || nodeErr.Code != "parse_decode_failed" {
			t.Fatalf("err = %v", err)
		}
	})
}
