package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"trippilot/internal/auth"
)

type fakeAuth struct {
	enabled   bool
	principal *auth.Principal
}

func (f fakeAuth) Enabled() bool { return f.enabled }

func (f fakeAuth) Middleware(next http.Handler) http.Handler {
	if !f.enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.principal == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "unauthorized", "message": "请先登录"}})
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), *f.principal)))
	})
}

func (f fakeAuth) AdminOrToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token != "" && r.Header.Get("X-Admin-Token") == token {
				next.ServeHTTP(w, r)
				return
			}
			if !f.enabled {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "unauthorized"}})
				return
			}
			if f.principal == nil {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "unauthorized"}})
				return
			}
			if f.principal.Role != auth.RoleAdmin {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": map[string]string{"code": "forbidden"}})
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), *f.principal)))
		})
	}
}

func (f fakeAuth) Me(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       f.enabled,
		"authenticated": f.principal != nil,
		"provider_name": "test",
	})
}

func (f fakeAuth) Login(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func (f fakeAuth) Callback(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func (f fakeAuth) Logout(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func principalFor(id, role string) *auth.Principal {
	return &auth.Principal{UserID: id, Role: role}
}

func authRouter(store TripStore, runner RunStarter, authenticator Authenticator) http.Handler {
	return NewRouter(Deps{
		Version:          "test",
		Logger:           discardLogger(),
		AdminToken:       "secret",
		RateLimitPerHour: 100,
		Now:              func() time.Time { return time.Unix(1700000000, 0) },
		Store:            store,
		Runner:           runner,
		Auth:             authenticator,
	})
}

func createTripAs(t *testing.T, router http.Handler, requestID string) createTripResponse {
	t.Helper()
	body := `{"input":"北京三天","client_request_id":"` + requestID + `"}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/trips", strings.NewReader(body)))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var response createTripResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestAuthRequiredOnProtectedRoutes(t *testing.T) {
	store := newFakeTripStore()
	router := authRouter(store, &fakeRunner{}, fakeAuth{enabled: true})

	for _, route := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/trips", ""},
		{http.MethodPost, "/api/trips", `{"input":"北京三天"}`},
		{http.MethodGet, "/api/trips/trip-1", ""},
		{http.MethodPost, "/api/runs/run-1/resume", `{}`},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(route.method, route.path, strings.NewReader(route.body)))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d", route.method, route.path, recorder.Code)
		}
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"enabled":true`) {
		t.Fatalf("me = %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/share/missing", nil))
	if recorder.Code == http.StatusUnauthorized {
		t.Fatal("share pages must stay public")
	}
}

func TestTripOwnershipIsolation(t *testing.T) {
	store := newFakeTripStore()
	routerA := authRouter(store, &fakeRunner{}, fakeAuth{enabled: true, principal: principalFor("user-a", auth.RoleUser)})
	routerB := authRouter(store, &fakeRunner{}, fakeAuth{enabled: true, principal: principalFor("user-b", auth.RoleUser)})

	created := createTripAs(t, routerA, "trip-a")
	tripPath := "/api/trips/" + created.TripID

	recorder := httptest.NewRecorder()
	routerA.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tripPath, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("owner get status = %d", recorder.Code)
	}

	for _, route := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, tripPath, ""},
		{http.MethodGet, tripPath + "/versions", ""},
		{http.MethodDelete, tripPath, ""},
		{http.MethodPost, tripPath + "/share", ""},
		{http.MethodGet, "/api/runs/" + created.RunID + "/events", ""},
	} {
		recorder := httptest.NewRecorder()
		routerB.ServeHTTP(recorder, httptest.NewRequest(route.method, route.path, strings.NewReader(route.body)))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("non-owner %s %s status = %d body = %s", route.method, route.path, recorder.Code, recorder.Body.String())
		}
	}

	recorder = httptest.NewRecorder()
	routerB.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/trips", strings.NewReader(`{"input":"北京三天","client_request_id":"trip-a"}`)))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("cross-user idempotency status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), created.TripID) {
		t.Fatalf("conflict leaked trip id: %s", recorder.Body.String())
	}

	listA := httptest.NewRecorder()
	routerA.ServeHTTP(listA, httptest.NewRequest(http.MethodGet, "/api/trips?limit=20", nil))
	var tripsA tripListResponse
	if err := json.Unmarshal(listA.Body.Bytes(), &tripsA); err != nil {
		t.Fatal(err)
	}
	if len(tripsA.Trips) != 1 || tripsA.Trips[0].ID != created.TripID {
		t.Fatalf("owner list = %+v", tripsA)
	}
	if tripsA.Trips[0].LatestRun == nil || tripsA.Trips[0].LatestRun.ID != created.RunID {
		t.Fatalf("latest run summary = %+v", tripsA.Trips[0].LatestRun)
	}

	listB := httptest.NewRecorder()
	routerB.ServeHTTP(listB, httptest.NewRequest(http.MethodGet, "/api/trips", nil))
	var tripsB tripListResponse
	if err := json.Unmarshal(listB.Body.Bytes(), &tripsB); err != nil {
		t.Fatal(err)
	}
	if len(tripsB.Trips) != 0 {
		t.Fatalf("other user list = %+v", tripsB)
	}
}

func TestTripListAnonymousWhenAuthDisabled(t *testing.T) {
	store := newFakeTripStore()
	router := authRouter(store, &fakeRunner{}, fakeAuth{enabled: false})
	created := createTripAs(t, router, "anon")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/trips?limit=20", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d", recorder.Code)
	}
	var response tripListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Trips) != 1 || response.Trips[0].ID != created.TripID {
		t.Fatalf("anonymous list = %+v", response)
	}
}

func TestTripListPagingValidation(t *testing.T) {
	router := authRouter(newFakeTripStore(), &fakeRunner{}, fakeAuth{enabled: false})
	for _, query := range []string{"limit=0", "limit=abc", "offset=-1"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/trips?"+query, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d", query, recorder.Code)
		}
	}
}

func TestEvalAuthorization(t *testing.T) {
	eval := &fakeEval{}
	tests := []struct {
		name     string
		auth     Authenticator
		token    string
		wantCode int
	}{
		{name: "user forbidden", auth: fakeAuth{enabled: true, principal: principalFor("u", auth.RoleUser)}, wantCode: http.StatusForbidden},
		{name: "admin allowed", auth: fakeAuth{enabled: true, principal: principalFor("a", auth.RoleAdmin)}, wantCode: http.StatusOK},
		{name: "no credentials", auth: fakeAuth{enabled: true}, wantCode: http.StatusUnauthorized},
		{name: "operator token", auth: fakeAuth{enabled: true}, token: "secret", wantCode: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/eval/cases", nil)
			if tt.token != "" {
				request.Header.Set("X-Admin-Token", tt.token)
			}
			recorder := httptest.NewRecorder()
			NewRouter(Deps{
				Version: "test", Logger: discardLogger(), AdminToken: "secret", RateLimitPerHour: 100,
				Now: func() time.Time { return time.Unix(1700000000, 0) }, Store: newFakeTripStore(),
				Runner: &fakeRunner{}, Eval: eval, Auth: tt.auth,
			}).ServeHTTP(recorder, request)
			if recorder.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d body = %s", recorder.Code, tt.wantCode, recorder.Body.String())
			}
		})
	}
}

func TestSameOriginGuard(t *testing.T) {
	handler := SameOrigin([]string{"https://app.example.com"}, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	cases := []struct {
		name   string
		origin string
		want   int
	}{
		{name: "cross-site", origin: "https://evil.example.com", want: http.StatusForbidden},
		{name: "same host", origin: "https://example.com", want: http.StatusOK},
		{name: "allowlisted", origin: "https://app.example.com", want: http.StatusOK},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://example.com/api/trips", nil)
			request.Header.Set("Origin", tt.origin)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
		})
	}

	request := httptest.NewRequest(http.MethodPost, "https://example.com/api/trips", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("no origin status = %d", recorder.Code)
	}
}

func TestCORS(t *testing.T) {
	handler := CORS([]string{"https://app.example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
	request.Header.Set("Origin", "https://app.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" ||
		recorder.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("cors headers = %v", recorder.Header())
	}
	if recorder.Header().Get("Vary") != "Origin" {
		t.Fatalf("Vary = %q", recorder.Header().Get("Vary"))
	}

	request = httptest.NewRequest(http.MethodOptions, "/api/trips", nil)
	request.Header.Set("Origin", "https://app.example.com")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || recorder.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatalf("preflight = %d %v", recorder.Code, recorder.Header())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/meta", nil)
	request.Header.Set("Origin", "https://evil.example.com")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("disallowed origin echoed: %v", recorder.Header())
	}
}

func TestRateLimitByPrincipal(t *testing.T) {
	now := func() time.Time { return time.Unix(1700000000, 0) }
	limiter := RateLimitBy(1, now, principalRateKey)
	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	call := func(userID string) int {
		request := httptest.NewRequest(http.MethodPost, "/api/trips", nil)
		request.RemoteAddr = "203.0.113.7:1234"
		if userID != "" {
			request = request.WithContext(auth.WithPrincipal(request.Context(), auth.Principal{UserID: userID}))
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Code
	}
	if code := call("a"); code != http.StatusOK {
		t.Fatalf("first = %d", code)
	}
	if code := call("b"); code != http.StatusOK {
		t.Fatalf("second user = %d", code)
	}
	if code := call("a"); code != http.StatusTooManyRequests {
		t.Fatalf("repeat a = %d", code)
	}
	if code := call(""); code != http.StatusOK {
		t.Fatalf("anonymous first = %d", code)
	}
	if code := call(""); code != http.StatusTooManyRequests {
		t.Fatalf("anonymous repeat = %d", code)
	}
}
