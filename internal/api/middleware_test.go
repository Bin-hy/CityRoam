package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func decodeError(t *testing.T, recorder *httptest.ResponseRecorder) errorDetail {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", recorder.Body.String(), err)
	}
	return body.Error
}

func TestMiddleware(t *testing.T) {
	t.Run("request id is reused and generated", func(t *testing.T) {
		var seen string
		handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = RequestIDFrom(r.Context())
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-ID", "req-123")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if seen != "req-123" || rec.Header().Get("X-Request-ID") != "req-123" {
			t.Fatalf("seen=%q header=%q", seen, rec.Header().Get("X-Request-ID"))
		}

		req = httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-ID", "bad\nvalue")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		generated := rec.Header().Get("X-Request-ID")
		if generated == "" || generated == "bad\nvalue" || seen != generated {
			t.Fatalf("generated=%q seen=%q", generated, seen)
		}
	})

	t.Run("recover maps panics", func(t *testing.T) {
		handler := RequestID(Recover(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("boom")
		})))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d", rec.Code)
		}
		body := decodeError(t, rec)
		if body.Code != "internal" || body.RequestID == "" {
			t.Fatalf("error = %+v", body)
		}
	})

	t.Run("body limit rejects oversized requests", func(t *testing.T) {
		handler := BodyLimit(16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 32)))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d", rec.Code)
		}
		if body := decodeError(t, rec); body.Code != "payload_too_large" {
			t.Fatalf("error = %+v", body)
		}
	})

	t.Run("rate limit per ip and window reset", func(t *testing.T) {
		now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
		handler := RequestID(RateLimit(2, func() time.Time { return now })(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})))

		call := func(ip string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.RemoteAddr = ip + ":4000"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			return rec
		}

		if rec := call("1.1.1.1"); rec.Code != http.StatusOK {
			t.Fatalf("first status = %d", rec.Code)
		}
		if rec := call("1.1.1.1"); rec.Code != http.StatusOK {
			t.Fatalf("second status = %d", rec.Code)
		}
		rec := call("1.1.1.1")
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("third status = %d", rec.Code)
		}
		if rec.Header().Get("Retry-After") == "" {
			t.Fatal("missing Retry-After")
		}
		if body := decodeError(t, rec); body.Code != "rate_limited" {
			t.Fatalf("error = %+v", body)
		}
		if rec := call("2.2.2.2"); rec.Code != http.StatusOK {
			t.Fatalf("other ip status = %d", rec.Code)
		}

		now = now.Add(time.Hour)
		if rec := call("1.1.1.1"); rec.Code != http.StatusOK {
			t.Fatalf("after reset status = %d", rec.Code)
		}
	})

	t.Run("admin token", func(t *testing.T) {
		handler := AdminToken("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("missing token status = %d", rec.Code)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer wrong")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong token status = %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer secret")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("bearer status = %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Admin-Token", "secret")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("header status = %d", rec.Code)
		}
	})

	t.Run("empty admin token denies all", func(t *testing.T) {
		handler := AdminToken("")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Admin-Token", "")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}

func TestRouter(t *testing.T) {
	var readyErr error
	router := NewRouter(Deps{
		Version:          "test",
		Logger:           discardLogger(),
		AdminToken:       "secret",
		RateLimitPerHour: 10,
		Now:              func() time.Time { return time.Unix(1700000000, 0) },
		Ready:            func(context.Context) error { return readyErr },
	})

	t.Run("health", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("ready", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		readyErr = io.ErrUnexpectedEOF
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d", rec.Code)
		}
		readyErr = nil
	})

	t.Run("meta", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var body struct {
			Version string   `json:"version"`
			Cities  []string `json:"cities"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Version != "test" || len(body.Cities) != 1 || body.Cities[0] != "beijing" {
			t.Fatalf("body = %+v", body)
		}
	})

	t.Run("unimplemented endpoints share the envelope", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/runs/x", nil))
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d", rec.Code)
		}
		if body := decodeError(t, rec); body.Code != "not_implemented" || body.RequestID == "" {
			t.Fatalf("error = %+v", body)
		}
	})

	t.Run("eval requires admin token", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/eval/cases", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}
