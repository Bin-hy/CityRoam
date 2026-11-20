package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewSessionTokenIsRandomAndHashed(t *testing.T) {
	first, firstHash, err := newSessionToken()
	if err != nil {
		t.Fatalf("newSessionToken() error = %v", err)
	}
	second, secondHash, err := newSessionToken()
	if err != nil {
		t.Fatalf("newSessionToken() error = %v", err)
	}
	if first == second || firstHash == secondHash {
		t.Fatal("session tokens should not repeat")
	}
	if firstHash != hashToken(first) {
		t.Fatal("hashToken() is not stable")
	}
	if strings.Contains(firstHash, first) || len(firstHash) != 64 {
		t.Fatalf("unexpected hash %q for token %q", firstHash, first)
	}
}

func TestSessionCookieFlags(t *testing.T) {
	recorder := httptest.NewRecorder()
	setSessionCookie(recorder, "token", 2*time.Hour, true)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != sessionCookieName || cookie.Value != "token" {
		t.Fatalf("cookie = %+v", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != 2 || cookie.Path != "/" {
		t.Fatalf("cookie flags = %+v", cookie)
	}
	if cookie.MaxAge != 7200 {
		t.Fatalf("MaxAge = %d, want 7200", cookie.MaxAge)
	}
}

func TestCookieSecureModes(t *testing.T) {
	plain := httptest.NewRequest("GET", "/", nil)
	proxied := httptest.NewRequest("GET", "/", nil)
	proxied.Header.Set("X-Forwarded-Proto", "HTTPS")
	proxied.Header.Set("X-Forwarded-Proto", "https")
	tlsRequest := httptest.NewRequest("GET", "https://example.com/", nil)

	if cookieSecure("true", plain) != true {
		t.Fatal("secure true should force Secure")
	}
	if cookieSecure("false", tlsRequest) != false {
		t.Fatal("secure false should force insecure")
	}
	if cookieSecure("auto", plain) != false {
		t.Fatal("auto without TLS should be insecure")
	}
	if cookieSecure("auto", tlsRequest) != true {
		t.Fatal("auto with TLS should be Secure")
	}
	if cookieSecure("auto", proxied) != true {
		t.Fatal("auto with X-Forwarded-Proto should be Secure")
	}
}

func TestOIDCCookieBinding(t *testing.T) {
	recorder := httptest.NewRecorder()
	setOIDCCookie(recorder, "state-1", false)
	cookie := recorder.Result().Cookies()[0]
	if cookie.Path != "/api/auth" || !cookie.HttpOnly || cookie.MaxAge != 600 {
		t.Fatalf("binding cookie = %+v", cookie)
	}
	request := httptest.NewRequest("GET", "/api/auth/callback", nil)
	request.AddCookie(cookie)
	if !oidcCookieMatches(request, "state-1") {
		t.Fatal("matching state should pass")
	}
	if oidcCookieMatches(request, "state-2") {
		t.Fatal("different state should fail")
	}
	if oidcCookieMatches(httptest.NewRequest("GET", "/", nil), "state-1") {
		t.Fatal("missing cookie should fail")
	}
}
