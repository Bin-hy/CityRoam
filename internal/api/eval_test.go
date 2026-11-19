package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"trippilot/internal/repo"
)

type fakeEval struct {
	cases   []repo.EvalCase
	runs    []repo.EvalRun
	started bool
	err     error
}

func (e *fakeEval) ListCases(context.Context) ([]repo.EvalCase, error) {
	return e.cases, e.err
}

func (e *fakeEval) Start(context.Context) error {
	if e.err != nil {
		return e.err
	}
	e.started = true
	return nil
}

func (e *fakeEval) ListRuns(context.Context) ([]repo.EvalRun, error) {
	return e.runs, e.err
}

func TestEval(t *testing.T) {
	service := &fakeEval{
		cases: []repo.EvalCase{{
			ID: "case-01", Input: "北京一天", Expected: json.RawMessage(`{"days":1}`), Tags: []string{"1d"}, Enabled: true,
		}},
		runs: []repo.EvalRun{{
			ID: "eval-1", CaseID: "case-01", Metrics: json.RawMessage(`{"passed":true}`), Passed: true,
		}},
	}
	router := baseRouter(Deps{AdminToken: "secret", Eval: service})

	request := func(method, path string, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("X-Admin-Token", token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	if rec := request(http.MethodGet, "/api/eval/cases", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", rec.Code)
	}
	if rec := request(http.MethodGet, "/api/eval/cases", "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status = %d", rec.Code)
	}
	if rec := request(http.MethodGet, "/api/eval/cases", "secret"); rec.Code != http.StatusOK {
		t.Fatalf("cases status = %d body=%s", rec.Code, rec.Body.String())
	} else {
		var body struct {
			Cases []repo.EvalCase `json:"cases"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode cases: %v", err)
		}
		if len(body.Cases) != 1 || body.Cases[0].ID != "case-01" {
			t.Fatalf("cases = %+v", body.Cases)
		}
	}

	if rec := request(http.MethodPost, "/api/eval/runs", "secret"); rec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !service.started {
		t.Fatal("eval batch was not started")
	}

	if rec := request(http.MethodGet, "/api/eval/runs", "secret"); rec.Code != http.StatusOK {
		t.Fatalf("runs status = %d body=%s", rec.Code, rec.Body.String())
	}

	bearer := httptest.NewRequest(http.MethodGet, "/api/eval/cases", nil)
	bearer.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, bearer)
	if rec.Code != http.StatusOK {
		t.Fatalf("bearer status = %d", rec.Code)
	}
}

func TestEvalUnavailable(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/eval/cases", nil)
	req.Header.Set("X-Admin-Token", "secret")
	baseRouter(Deps{AdminToken: "secret"}).ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if body := decodeError(t, rec); body.Code != "eval_unavailable" {
		t.Fatalf("error = %+v", body)
	}
}

func TestEvalFailure(t *testing.T) {
	service := &fakeEval{err: errors.New("db down")}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/eval/runs", nil)
	req.Header.Set("X-Admin-Token", "secret")
	baseRouter(Deps{AdminToken: "secret", Eval: service}).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRateLimit(t *testing.T) {
	router := baseRouter(Deps{RateLimitPerHour: 10})
	do := func(ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/api/trips/nope", nil)
		req.RemoteAddr = ip + ":4567"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	for i := 0; i < 10; i++ {
		if rec := do("192.0.2.10"); rec.Code != http.StatusNoContent {
			t.Fatalf("request %d status = %d body=%s", i+1, rec.Code, rec.Body.String())
		}
	}
	rec := do("192.0.2.10")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("limited status = %d body=%s", rec.Code, rec.Body.String())
	}
	if retry, err := strconv.Atoi(rec.Header().Get("Retry-After")); err != nil || retry <= 0 {
		t.Fatalf("retry-after = %q", rec.Header().Get("Retry-After"))
	}
	if body := decodeError(t, rec); body.Code != "rate_limited" || body.RequestID == "" {
		t.Fatalf("error = %+v", body)
	}
	if rec := do("192.0.2.11"); rec.Code != http.StatusNoContent {
		t.Fatalf("other ip status = %d", rec.Code)
	}

	meta := httptest.NewRecorder()
	router.ServeHTTP(meta, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
	if meta.Code != http.StatusOK {
		t.Fatalf("meta status = %d", meta.Code)
	}
}
