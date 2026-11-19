package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Heartbeat is an SSE comment that keeps proxies and browsers from timing out
// during long agent stages.
const Heartbeat = ": ping\n\n"

// SetHeaders applies the headers required for a non-buffered event stream.
func SetHeaders(header http.Header) {
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
}

// WriteEvent encodes one event as `id`, `event` and `data` fields. Multi-line
// JSON payloads are split across data lines as the SSE format requires.
func WriteEvent(w io.Writer, event Event) error {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "id: %d\n", event.Seq)
	fmt.Fprintf(&buf, "event: %s\n", event.Type)
	payload := event.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`null`)
	}
	for _, line := range strings.Split(string(payload), "\n") {
		fmt.Fprintf(&buf, "data: %s\n", line)
	}
	buf.WriteString("\n")
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("events: write sse: %w", err)
	}
	return nil
}

// WriteHeartbeat emits an SSE comment and is safe to call between events.
func WriteHeartbeat(w io.Writer) error {
	if _, err := io.WriteString(w, Heartbeat); err != nil {
		return fmt.Errorf("events: write heartbeat: %w", err)
	}
	return nil
}
