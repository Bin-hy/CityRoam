package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"trippilot/internal/auth"
)

// TripOwner hides trips that belong to another account behind the same 404 as
// a missing trip, including for admins.
func TripOwner(deps Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !deps.Auth.Enabled() {
				next.ServeHTTP(w, r)
				return
			}
			principal, ok := auth.FromContext(r.Context())
			if !ok {
				WriteError(w, r, &Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "请先登录"})
				return
			}
			trip, err := deps.Store.GetTrip(r.Context(), chi.URLParam(r, "tripID"))
			if err != nil {
				WriteError(w, r, mapStoreError(err))
				return
			}
			if trip.OwnerID == nil || *trip.OwnerID != principal.UserID {
				WriteError(w, r, &Error{Status: http.StatusNotFound, Code: "not_found", Message: "行程不存在"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RunOwner resolves a run to its trip before applying the same ownership rule.
func RunOwner(deps Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !deps.Auth.Enabled() {
				next.ServeHTTP(w, r)
				return
			}
			principal, ok := auth.FromContext(r.Context())
			if !ok {
				WriteError(w, r, &Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "请先登录"})
				return
			}
			run, err := deps.Store.GetRun(r.Context(), chi.URLParam(r, "runID"))
			if err != nil {
				WriteError(w, r, mapStoreError(err))
				return
			}
			trip, err := deps.Store.GetTrip(r.Context(), run.TripID)
			if err != nil {
				WriteError(w, r, mapStoreError(err))
				return
			}
			if trip.OwnerID == nil || *trip.OwnerID != principal.UserID {
				WriteError(w, r, &Error{Status: http.StatusNotFound, Code: "not_found", Message: "行程不存在"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// principalRateKey buckets authenticated traffic per user and falls back to the
// client address before login.
func principalRateKey(r *http.Request) string {
	if principal, ok := auth.FromContext(r.Context()); ok && principal.UserID != "" {
		return "user:" + principal.UserID
	}
	return "ip:" + clientIP(r)
}
