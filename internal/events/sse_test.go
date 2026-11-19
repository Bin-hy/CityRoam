package events

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestWriteEvent(t *testing.T) {
	var buf bytes.Buffer
	event := Event{
		Seq:     7,
		Type:    "plan.updated",
		Payload: json.RawMessage("{\n  \"seq\": 2,\n  \"reason\": \"initial\"\n}"),
	}
	if err := WriteEvent(&buf, event); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := buf.String()
	want := "id: 7\nevent: plan.updated\ndata: {\ndata:   \"seq\": 2,\ndata:   \"reason\": \"initial\"\ndata: }\n\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestWriteEventEmptyPayload(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteEvent(&buf, Event{Seq: 1, Type: "run.cancelled"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(buf.String(), "data: null\n") {
		t.Fatalf("got %q", buf.String())
	}
}

func TestWriteHeartbeat(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHeartbeat(&buf); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if buf.String() != Heartbeat {
		t.Fatalf("got %q", buf.String())
	}
}

func TestSetHeaders(t *testing.T) {
	header := http.Header{}
	SetHeaders(header)
	if got := header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q", got)
	}
	if header.Get("Cache-Control") != "no-cache" || header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("headers = %v", header)
	}
}
