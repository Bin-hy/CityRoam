package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unicode"
)

type contextKey int

const requestIDKey contextKey = iota

const maxRequestIDLength = 128

// RequestID accepts a caller-provided id or generates one, then exposes it via
// context and the response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func newRequestID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

// Recover converts panics into the unified 500 envelope.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("http panic",
						"method", r.Method,
						"path", r.URL.Path,
						"request_id", RequestIDFrom(r.Context()),
						"panic", recovered,
						"stack", string(debug.Stack()),
					)
					WriteError(w, r, &Error{
						Status:  http.StatusInternalServerError,
						Code:    "internal",
						Message: "服务暂时不可用",
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// BodyLimit rejects clearly oversized requests and caps reads for the rest.
func BodyLimit(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limit > 0 {
				if r.ContentLength > limit {
					WriteError(w, r, &Error{
						Status:  http.StatusRequestEntityTooLarge,
						Code:    "payload_too_large",
						Message: "请求体超过大小限制",
					})
					return
				}
				if r.Body != nil {
					r.Body = http.MaxBytesReader(w, r.Body, limit)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

type rateWindow struct {
	start time.Time
	count int
}

// RateLimit allows limit requests per IP per fixed one-hour window.
func RateLimit(limit int, now func() time.Time) func(http.Handler) http.Handler {
	if now == nil {
		now = time.Now
	}
	limiter := &rateLimiter{
		limit:   limit,
		now:     now,
		windows: make(map[string]*rateWindow),
	}
	return limiter.middleware
}

type rateLimiter struct {
	limit   int
	now     func() time.Time
	mu      sync.Mutex
	windows map[string]*rateWindow
}

const (
	rateWindowDuration = time.Hour
	rateSweepThreshold = 1024
)

func (l *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l.limit <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		now := l.now()

		l.mu.Lock()
		if len(l.windows) > rateSweepThreshold {
			l.sweep(now)
		}
		window, ok := l.windows[ip]
		if !ok {
			window = &rateWindow{start: now}
			l.windows[ip] = window
		}
		if now.Sub(window.start) >= rateWindowDuration {
			window.start = now
			window.count = 0
		}
		if window.count >= l.limit {
			retry := int(rateWindowDuration - now.Sub(window.start))
			l.mu.Unlock()
			if retry < 1 {
				retry = 1
			}
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retry))
			WriteError(w, r, &Error{
				Status:  http.StatusTooManyRequests,
				Code:    "rate_limited",
				Message: "请求过于频繁，请稍后再试",
			})
			return
		}
		window.count++
		l.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}

func (l *rateLimiter) sweep(now time.Time) {
	for ip, window := range l.windows {
		if now.Sub(window.start) >= rateWindowDuration {
			delete(l.windows, ip)
		}
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	if r.RemoteAddr == "" {
		return "unknown"
	}
	return r.RemoteAddr
}

// AdminToken protects operator routes with a constant-time token check.
func AdminToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				provided = strings.TrimSpace(auth[len("bearer "):])
			}
			if token == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				WriteError(w, r, &Error{
					Status:  http.StatusUnauthorized,
					Code:    "unauthorized",
					Message: "缺少或无效的管理员令牌",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
