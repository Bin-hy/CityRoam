package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

type fakeIDP struct {
	server            *httptest.Server
	key               *rsa.PrivateKey
	kid               string
	issuer            string
	nonce             string
	expectedChallenge string
	accessAudience    string
	subject           string
	email             string
	name              string
	picture           string
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIDP{
		key:     key,
		kid:     "test-key",
		subject: "user-sub",
		email:   "user@example.com",
		name:    "Trip User",
		picture: "https://example.com/avatar.png",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeTestJSON(w, map[string]any{
			"issuer":                                idp.issuer,
			"authorization_endpoint":                idp.issuer + "/authorize",
			"token_endpoint":                        idp.issuer + "/token",
			"jwks_uri":                              idp.issuer + "/jwks",
			"userinfo_endpoint":                     idp.issuer + "/userinfo",
			"end_session_endpoint":                  idp.issuer + "/logout",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key:       key.Public(),
			KeyID:     idp.kid,
			Algorithm: "RS256",
			Use:       "sig",
		}}}
		writeTestJSON(w, set)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.Form.Get("code") != "good-code" {
			http.Error(w, "bad code", http.StatusBadRequest)
			return
		}
		if idp.expectedChallenge != "" && pkceChallenge(r.Form.Get("code_verifier")) != idp.expectedChallenge {
			http.Error(w, "bad verifier", http.StatusBadRequest)
			return
		}
		idToken, err := idp.signIDToken()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		accessToken := "access-token"
		if idp.accessAudience != "" {
			accessToken, err = idp.signAccessToken()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		writeTestJSON(w, map[string]any{
			"access_token": accessToken,
			"token_type":   "Bearer",
			"id_token":     idToken,
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		writeTestJSON(w, map[string]any{
			"sub":            idp.subject,
			"email":          idp.email,
			"email_verified": true,
			"name":           idp.name,
			"picture":        idp.picture,
		})
	})
	idp.server = httptest.NewServer(mux)
	t.Cleanup(idp.server.Close)
	idp.issuer = idp.server.URL
	return idp
}

func writeTestJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (idp *fakeIDP) signIDToken() (string, error) {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: idp.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", idp.kid))
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := jwt.Claims{
		Issuer:   idp.issuer,
		Subject:  idp.subject,
		Audience: jwt.Audience{"trippilot"},
		Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
		IssuedAt: jwt.NewNumericDate(now),
		ID:       "id-token-1",
	}
	extra := map[string]any{
		"nonce":          idp.nonce,
		"email":          idp.email,
		"email_verified": true,
		"name":           idp.name,
		"picture":        idp.picture,
	}
	return jwt.Signed(signer).Claims(claims).Claims(extra).Serialize()
}

func (idp *fakeIDP) signAccessToken() (string, error) {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: idp.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", idp.kid))
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := jwt.Claims{
		Issuer:   idp.issuer,
		Subject:  idp.subject,
		Audience: jwt.Audience{idp.accessAudience},
		Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
		IssuedAt: jwt.NewNumericDate(now),
	}
	return jwt.Signed(signer).Claims(claims).Serialize()
}

type authHarness struct {
	idp     *fakeIDP
	store   *fakeStore
	service *Service
	logs    *bytes.Buffer
}

func newAuthHarness(t *testing.T) *authHarness {
	t.Helper()
	idp := newFakeIDP(t)
	store := newFakeStore()
	logs := &bytes.Buffer{}
	service, err := New(context.Background(), Config{
		Enabled:      true,
		CookieSecure: "false",
		Issuer:       idp.issuer,
		ClientID:     "trippilot",
		ClientSecret: "client-secret-value",
		RedirectURI:  "http://app.local/api/auth/callback",
		Scopes:       []string{"openid", "profile", "email"},
		ProviderName: "yufu",
		SessionTTL:   time.Hour,
		AdminEmails:  []string{"admin@example.com"},
	}, store, slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return &authHarness{idp: idp, store: store, service: service, logs: logs}
}

// startLogin runs Login and returns the browser-bound state plus the OIDC
// binding cookie.
func (h *authHarness) startLogin(t *testing.T, returnTo string) (state string, cookie *http.Cookie) {
	t.Helper()
	request := httptest.NewRequest("GET", "/api/auth/login?return_to="+url.QueryEscape(returnTo), nil)
	recorder := httptest.NewRecorder()
	h.service.Login(recorder, request)
	if recorder.Code != http.StatusFound {
		t.Fatalf("Login status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	h.idp.nonce = location.Query().Get("nonce")
	h.idp.expectedChallenge = location.Query().Get("code_challenge")
	return location.Query().Get("state"), cookies[0]
}

func (h *authHarness) callback(t *testing.T, state string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest("GET", "/api/auth/callback?code=good-code&state="+url.QueryEscape(state), nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	h.service.Callback(recorder, request)
	return recorder
}

func TestCallbackCreatesSessionAndReturnsToTrip(t *testing.T) {
	harness := newAuthHarness(t)
	state, cookie := harness.startLogin(t, "/trips/abc")

	recorder := harness.callback(t, state, cookie)
	if recorder.Code != http.StatusFound {
		t.Fatalf("Callback status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if location := recorder.Header().Get("Location"); location != "/trips/abc" {
		t.Fatalf("redirect = %q, want /trips/abc", location)
	}
	sessionCookie := findCookie(recorder.Result().Cookies(), sessionCookieName)
	if sessionCookie == nil || sessionCookie.Value == "" || !sessionCookie.HttpOnly {
		t.Fatalf("session cookie = %+v", sessionCookie)
	}
	if harness.store.sessionCount() != 1 {
		t.Fatalf("sessions = %d, want 1", harness.store.sessionCount())
	}
	user, err := harness.store.GetUserByIssuerSubject(context.Background(), harness.idp.issuer, "user-sub")
	if err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if user.Role != RoleUser || user.Email == nil || *user.Email != "user@example.com" || user.Name == nil {
		t.Fatalf("user = %+v", user)
	}

	meRequest := httptest.NewRequest("GET", "/api/auth/me", nil)
	meRequest.AddCookie(sessionCookie)
	meRecorder := httptest.NewRecorder()
	harness.service.Me(meRecorder, meRequest)
	var me authMeResponse
	if err := json.NewDecoder(meRecorder.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if !me.Authenticated || me.User == nil || me.User.ID != user.ID || me.User.Role != RoleUser {
		t.Fatalf("Me = %+v", me)
	}
	for _, secret := range []string{"client-secret-value", "access-token", "id-token", "eyJ"} {
		if strings.Contains(harness.logs.String(), secret) {
			t.Fatalf("logs leaked %q: %s", secret, harness.logs.String())
		}
	}
}

func TestCallbackPromotesAllowlistedAdmin(t *testing.T) {
	harness := newAuthHarness(t)
	harness.idp.email = "Admin@Example.com"
	state, cookie := harness.startLogin(t, "/")
	if recorder := harness.callback(t, state, cookie); recorder.Code != http.StatusFound {
		t.Fatalf("Callback status = %d", recorder.Code)
	}
	user, err := harness.store.GetUserByIssuerSubject(context.Background(), harness.idp.issuer, "user-sub")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != RoleAdmin {
		t.Fatalf("role = %q, want admin", user.Role)
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	harness := newAuthHarness(t)
	state, _ := harness.startLogin(t, "/")

	recorder := harness.callback(t, state, &http.Cookie{Name: oidcCookieName, Value: "other-state"})
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "/login?error=login_state_invalid" {
		t.Fatalf("Callback = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
	if harness.store.sessionCount() != 0 {
		t.Fatal("no session should be created")
	}
}

func TestCallbackRejectsReplay(t *testing.T) {
	harness := newAuthHarness(t)
	state, cookie := harness.startLogin(t, "/")
	if recorder := harness.callback(t, state, cookie); recorder.Code != http.StatusFound {
		t.Fatalf("first callback status = %d", recorder.Code)
	}
	recorder := harness.callback(t, state, cookie)
	if recorder.Header().Get("Location") != "/login?error=login_state_invalid" {
		t.Fatalf("replay redirect = %q", recorder.Header().Get("Location"))
	}
	if harness.store.sessionCount() != 1 {
		t.Fatal("replay should not create another session")
	}
}

func TestCallbackRejectsNonceMismatch(t *testing.T) {
	harness := newAuthHarness(t)
	state, cookie := harness.startLogin(t, "/")
	harness.idp.nonce = "wrong-nonce"
	recorder := harness.callback(t, state, cookie)
	if recorder.Header().Get("Location") != "/login?error=auth_failed" {
		t.Fatalf("redirect = %q", recorder.Header().Get("Location"))
	}
	if harness.store.sessionCount() != 0 {
		t.Fatal("no session should be created")
	}
}

func TestCallbackRejectsWrongAudience(t *testing.T) {
	harness := newAuthHarness(t)
	harness.idp.accessAudience = "urn:other-app"

	store := newFakeStore()
	service, err := New(context.Background(), Config{
		Enabled:      true,
		CookieSecure: "false",
		Issuer:       harness.idp.issuer,
		ClientID:     "trippilot",
		ClientSecret: "client-secret-value",
		RedirectURI:  "http://app.local/api/auth/callback",
		Scopes:       []string{"openid"},
		Audience:     "urn:trippilot",
		SessionTTL:   time.Hour,
	}, store, testLogger())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	harness.service = service
	harness.store = store

	state, cookie := harness.startLogin(t, "/")
	recorder := harness.callback(t, state, cookie)
	if recorder.Header().Get("Location") != "/login?error=auth_failed" {
		t.Fatalf("redirect = %q", recorder.Header().Get("Location"))
	}
	if harness.store.sessionCount() != 0 {
		t.Fatal("no session should be created")
	}
}

func TestCallbackExpiredAuthRequest(t *testing.T) {
	harness := newAuthHarness(t)
	state, cookie := harness.startLogin(t, "/")
	pending := harness.store.requests[state]
	pending.ExpiresAt = time.Now().Add(-time.Minute)
	harness.store.requests[state] = pending

	recorder := harness.callback(t, state, cookie)
	if recorder.Header().Get("Location") != "/login?error=login_state_invalid" {
		t.Fatalf("redirect = %q", recorder.Header().Get("Location"))
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func TestLoginRedirectUsesWebBaseURL(t *testing.T) {
	harness := newAuthHarness(t)
	harness.service.cfg.WebBaseURL = "https://app.example.com"
	state, cookie := harness.startLogin(t, "/trips/1")
	if recorder := harness.callback(t, state, cookie); recorder.Header().Get("Location") != "https://app.example.com/trips/1" {
		t.Fatalf("redirect = %q", recorder.Header().Get("Location"))
	}
}
