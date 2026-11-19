package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"trippilot/internal/events"
)

var sseHeartbeat = 2 * time.Second

func handleRunEvents(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "runID")
		if _, err := deps.Store.GetRun(r.Context(), runID); err != nil {
			WriteError(w, r, mapStoreError(err))
			return
		}
		if deps.Broker == nil {
			WriteError(w, r, &Error{Status: http.StatusServiceUnavailable, Code: "events_unavailable", Message: "事件流未启用"})
			return
		}
		after, err := parseAfterSeq(r)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			WriteError(w, r, &Error{Status: http.StatusInternalServerError, Code: "stream_unsupported", Message: "不支持事件流"})
			return
		}

		events.SetHeaders(w.Header())
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		live, unsubscribe := deps.Broker.Subscribe(runID)
		defer unsubscribe()

		last := after
		replay, err := deps.Broker.Replay(r.Context(), runID, after)
		if err != nil {
			logError(deps.Logger, "replay run events", r, err)
		}
		for _, event := range replay {
			if event.Seq <= last {
				continue
			}
			if err := events.WriteEvent(w, event); err != nil {
				return
			}
			last = event.Seq
		}
		flusher.Flush()

		ticker := time.NewTicker(sseHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if err := events.WriteHeartbeat(w); err != nil {
					return
				}
				flusher.Flush()
			case event, open := <-live:
				if !open {
					return
				}
				if event.Seq <= last {
					continue
				}
				if err := events.WriteEvent(w, event); err != nil {
					return
				}
				last = event.Seq
				flusher.Flush()
			}
		}
	}
}

func parseAfterSeq(r *http.Request) (int32, error) {
	raw := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("after"))
	}
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, &Error{Status: http.StatusBadRequest, Code: "invalid_cursor", Message: "事件游标不合法"}
	}
	if value < 0 {
		value = 0
	}
	return int32(value), nil
}
