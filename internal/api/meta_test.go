package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func baseRouter(deps Deps) http.Handler {
	if deps.Version == "" {
		deps.Version = "test"
	}
	if deps.Logger == nil {
		deps.Logger = discardLogger()
	}
	if deps.RateLimitPerHour == 0 {
		deps.RateLimitPerHour = 100
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Unix(1700000000, 0) }
	}
	if deps.Store == nil {
		deps.Store = newFakeTripStore()
	}
	if deps.Runner == nil {
		deps.Runner = &fakeRunner{}
	}
	return NewRouter(deps)
}

func TestHealth(t *testing.T) {
	t.Run("liveness always reports ok", func(t *testing.T) {
		router := baseRouter(Deps{})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if body := rec.Body.String(); body != `{"status":"ok"}`+"\n" {
			t.Fatalf("body = %q", body)
		}
	})

	t.Run("readiness follows dependencies", func(t *testing.T) {
		var failure error
		router := baseRouter(Deps{Ready: func(context.Context) error { return failure }})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("ready status = %d", rec.Code)
		}

		failure = io.ErrUnexpectedEOF
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("not ready status = %d", rec.Code)
		}
		if body := decodeError(t, rec); body.Code != "not_ready" || body.RequestID == "" {
			t.Fatalf("error = %+v", body)
		}
	})
}

func TestMeta(t *testing.T) {
	router := baseRouter(Deps{Version: "1.2.3", RateLimitPerHour: 10})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Version  string   `json:"version"`
		Cities   []string `json:"cities"`
		Defaults struct {
			Days        int    `json:"days"`
			Pace        string `json:"pace"`
			Currency    string `json:"currency"`
			MaxTripDays int    `json:"max_trip_days"`
		} `json:"defaults"`
		RateLimit struct {
			PerHour int `json:"per_hour"`
		} `json:"rate_limit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Version != "1.2.3" || len(body.Cities) != 1 || body.Cities[0] != "beijing" {
		t.Fatalf("body = %+v", body)
	}
	if body.Defaults.Days != 2 || body.Defaults.Pace != "normal" || body.Defaults.Currency != "CNY" || body.Defaults.MaxTripDays != 4 {
		t.Fatalf("defaults = %+v", body.Defaults)
	}
	if body.RateLimit.PerHour != 10 {
		t.Fatalf("rate limit = %+v", body.RateLimit)
	}
}
