package api

import (
	"net/http"
	"net/url"
	"strings"
)

var corsMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
var corsHeaders = "Content-Type, Idempotency-Key, X-Admin-Token, X-Request-ID"

// CORS only activates for explicitly configured origins; credentials are
// allowed so the session cookie travels from a separately hosted frontend.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := originSet(allowedOrigins)
	if len(allowed) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Origin")
			origin := r.Header.Get("Origin")
			_, ok := allowed[origin]
			if ok && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if r.Method == http.MethodOptions {
				if ok {
					w.Header().Set("Access-Control-Allow-Methods", corsMethods)
					w.Header().Set("Access-Control-Allow-Headers", corsHeaders)
					w.Header().Set("Access-Control-Max-Age", "600")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SameOrigin blocks cross-site state-changing requests when auth is enabled.
// Browser requests always carry Origin on cross-site POSTs; clients without
// Origin (curl, scripts) keep working.
func SameOrigin(allowedOrigins []string, enabled bool) func(http.Handler) http.Handler {
	allowed := originSet(allowedOrigins)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled || !isMutating(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				if !sameSite(origin, r.Host) && !allowed[origin] {
					WriteError(w, r, &Error{Status: http.StatusForbidden, Code: "forbidden", Message: "请求来源不被允许"})
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if referer := r.Header.Get("Referer"); referer != "" {
				parsed, err := url.Parse(referer)
				if err != nil || (!sameSite("https://"+parsed.Host, r.Host) && !allowed[originOf(parsed)]) {
					WriteError(w, r, &Error{Status: http.StatusForbidden, Code: "forbidden", Message: "请求来源不被允许"})
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originSet(origins []string) map[string]bool {
	set := make(map[string]bool, len(origins))
	for _, origin := range origins {
		trimmed := strings.TrimSpace(origin)
		if trimmed != "" {
			set[strings.TrimSuffix(trimmed, "/")] = true
		}
	}
	return set
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func sameSite(origin, host string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, host)
}

func originOf(parsed *url.URL) string {
	return parsed.Scheme + "://" + parsed.Host
}
