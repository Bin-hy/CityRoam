package auth

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"trippilot/internal/repo"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAuthDisabledBehaviour(t *testing.T) {
	service := &Service{cfg: Config{Enabled: false, ProviderName: "统一登录"}, logger: testLogger(), now: time.Now}

	recorder := httptest.NewRecorder()
	service.Me(recorder, httptest.NewRequest("GET", "/api/auth/me", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("Me status = %d", recorder.Code)
	}
	var body authMeResponse
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Enabled || body.Authenticated || body.ProviderName != "统一登录" {
		t.Fatalf("Me body = %+v", body)
	}

	recorder = httptest.NewRecorder()
	service.Login(recorder, httptest.NewRequest("GET", "/api/auth/login", nil))
	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "auth_disabled") {
		t.Fatalf("Login = %d %s", recorder.Code, recorder.Body.String())
	}

	called := false
	handler := service.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Fatal("middleware should pass through when disabled")
	}
}

func TestLoginStoresRequestAndRedirects(t *testing.T) {
	store := newFakeStore()
	service := &Service{
		cfg:    Config{Enabled: true, CookieSecure: "false"},
		store:  store,
		logger: testLogger(),
		now:    time.Now,
		oauth2Config: oauth2.Config{
			ClientID:    "trippilot",
			Endpoint:    oauth2.Endpoint{AuthURL: "https://idp.example.com/authorize"},
			RedirectURL: "http://app.local/api/auth/callback",
			Scopes:      []string{"openid", "email"},
		},
	}

	recorder := httptest.NewRecorder()
	service.Login(recorder, httptest.NewRequest("GET", "/api/auth/login?return_to=%2Ftrips%2Fabc", nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("Login status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := location.Query().Get("state")
	if state == "" || location.Query().Get("nonce") == "" {
		t.Fatalf("missing state/nonce in %s", location)
	}
	if location.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("challenge method = %q", location.Query().Get("code_challenge_method"))
	}
	pending, ok := store.pendingRequest(state)
	if !ok {
		t.Fatal("auth request not stored")
	}
	if location.Query().Get("code_challenge") != pkceChallenge(pending.CodeVerifier) {
		t.Fatal("PKCE challenge does not match stored verifier")
	}
	if pending.ReturnTo != "/trips/abc" {
		t.Fatalf("ReturnTo = %q", pending.ReturnTo)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != oidcCookieName || cookies[0].Value != state {
		t.Fatalf("binding cookie = %+v", cookies)
	}
}

func TestLoginRejectsUnsafeReturnTo(t *testing.T) {
	service := &Service{cfg: Config{Enabled: true}, store: newFakeStore(), logger: testLogger(), now: time.Now}
	for _, target := range []string{"https://evil.example.com", "//evil.example.com", `/\evil`, "/ok\x00"} {
		t.Run(target, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/api/auth/login", nil)
			request.URL.RawQuery = url.Values{"return_to": {target}}.Encode()
			recorder := httptest.NewRecorder()
			service.Login(recorder, request)
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_return_to") {
				t.Fatalf("Login(%q) = %d %s", target, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestSanitizeReturnTo(t *testing.T) {
	cases := map[string]bool{
		"":                 true,
		"/":                true,
		"/trips/1?tab=map": true,
		"https://evil.com": false,
		"//evil.com":       false,
		`/a\b`:             false,
		"relative":         false,
	}
	for input, want := range cases {
		if _, ok := sanitizeReturnTo(input); ok != want {
			t.Fatalf("sanitizeReturnTo(%q) ok = %v, want %v", input, ok, want)
		}
	}
}

func TestMeReturnsSessionUser(t *testing.T) {
	store := newFakeStore()
	user, err := store.UpsertUser(context.Background(), repo.UpsertUserParams{
		Issuer: "issuer", Subject: "sub", Email: optionalString("a@example.com"), EmailVerified: true, Role: RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	token, hash, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(context.Background(), repo.CreateSessionParams{ID: hash, UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	service := &Service{cfg: Config{Enabled: true, ProviderName: "yufu"}, store: store, logger: testLogger(), now: time.Now}

	request := httptest.NewRequest("GET", "/api/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	service.Me(recorder, request)

	var body authMeResponse
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Enabled || !body.Authenticated || body.User == nil || body.User.ID != user.ID {
		t.Fatalf("Me body = %+v", body)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", recorder.Header().Get("Cache-Control"))
	}
}

func TestLogoutDeletesSession(t *testing.T) {
	store := newFakeStore()
	user, _ := store.UpsertUser(context.Background(), repo.UpsertUserParams{Issuer: "i", Subject: "s", Role: RoleUser})
	token, hash, _ := newSessionToken()
	_ = store.CreateSession(context.Background(), repo.CreateSessionParams{ID: hash, UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)})
	service := &Service{cfg: Config{Enabled: true, CookieSecure: "false"}, store: store, logger: testLogger(), now: time.Now}

	request := httptest.NewRequest("POST", "/api/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	service.Logout(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("Logout status = %d", recorder.Code)
	}
	if store.sessionCount() != 0 {
		t.Fatal("session should be deleted")
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 || cookies[0].MaxAge >= 0 {
		t.Fatalf("session cookie should be cleared: %+v", cookies)
	}
}

func TestLogoutReturnsEndSessionURL(t *testing.T) {
	store := newFakeStore()
	service := &Service{
		cfg: Config{
			Enabled:               true,
			CookieSecure:          "false",
			ClientID:              "trippilot",
			PostLogoutRedirectURI: "http://app.local/",
		},
		store:              store,
		logger:             testLogger(),
		now:                time.Now,
		endSessionEndpoint: "https://idp.example.com/logout",
	}
	recorder := httptest.NewRecorder()
	service.Logout(recorder, httptest.NewRequest("POST", "/api/auth/logout", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("Logout status = %d", recorder.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	redirect := body["redirect_url"]
	if !strings.HasPrefix(redirect, "https://idp.example.com/logout?") ||
		!strings.Contains(redirect, "client_id=trippilot") {
		t.Fatalf("redirect_url = %q", redirect)
	}
}

func TestMiddlewareAuthStates(t *testing.T) {
	store := newFakeStore()
	user, _ := store.UpsertUser(context.Background(), repo.UpsertUserParams{Issuer: "i", Subject: "s", Role: RoleUser})
	admin, _ := store.UpsertUser(context.Background(), repo.UpsertUserParams{Issuer: "i", Subject: "a", Role: RoleAdmin})
	userToken, userHash, _ := newSessionToken()
	adminToken, adminHash, _ := newSessionToken()
	_ = store.CreateSession(context.Background(), repo.CreateSessionParams{ID: userHash, UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)})
	_ = store.CreateSession(context.Background(), repo.CreateSessionParams{ID: adminHash, UserID: admin.ID, ExpiresAt: time.Now().Add(time.Hour)})

	service := &Service{cfg: Config{Enabled: true, CookieSecure: "false"}, store: store, logger: testLogger(), now: time.Now}
	var seen Principal
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := service.Middleware(finalHandler)

	request := httptest.NewRequest("GET", "/trips", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("without cookie status = %d", recorder.Code)
	}

	request = httptest.NewRequest("GET", "/trips", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "forged"})
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("forged cookie status = %d", recorder.Code)
	}

	request = httptest.NewRequest("GET", "/trips", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userToken})
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || seen.UserID != user.ID {
		t.Fatalf("user status = %d principal = %+v", recorder.Code, seen)
	}

	adminHandler := service.AdminOrToken("operator-token")
	request = httptest.NewRequest("GET", "/eval", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userToken})
	recorder = httptest.NewRecorder()
	adminHandler(finalHandler).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("user admin status = %d", recorder.Code)
	}

	request = httptest.NewRequest("GET", "/eval", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: adminToken})
	recorder = httptest.NewRecorder()
	adminHandler(finalHandler).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("admin status = %d", recorder.Code)
	}

	request = httptest.NewRequest("GET", "/eval", nil)
	request.Header.Set("X-Admin-Token", "operator-token")
	recorder = httptest.NewRecorder()
	adminHandler(finalHandler).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("token status = %d", recorder.Code)
	}

	request = httptest.NewRequest("GET", "/eval", nil)
	recorder = httptest.NewRecorder()
	adminHandler(finalHandler).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("no credentials status = %d", recorder.Code)
	}
}

func TestCleanupRemovesExpiredRows(t *testing.T) {
	store := newFakeStore()
	past := time.Now().Add(-time.Hour)
	store.sessions["expired"] = repo.Session{ID: "expired", UserID: "u", ExpiresAt: past}
	store.sessions["active"] = repo.Session{ID: "active", UserID: "u", ExpiresAt: time.Now().Add(time.Hour)}
	store.requests["old"] = repo.AuthRequest{State: "old", ExpiresAt: past}
	store.requests["new"] = repo.AuthRequest{State: "new", ExpiresAt: time.Now().Add(time.Minute)}
	service := &Service{cfg: Config{Enabled: true}, store: store, logger: testLogger(), now: time.Now}

	if err := service.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, ok := store.sessions["expired"]; ok {
		t.Fatal("expired session should be removed")
	}
	if _, ok := store.sessions["active"]; !ok {
		t.Fatal("active session should remain")
	}
	if _, ok := store.requests["old"]; ok {
		t.Fatal("expired auth request should be removed")
	}
	if _, ok := store.requests["new"]; !ok {
		t.Fatal("active auth request should remain")
	}
}
