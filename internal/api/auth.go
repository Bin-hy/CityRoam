package api

import (
	"net/http"
)

// Authenticator is the HTTP-facing slice of the auth service. Keeping it narrow
// lets tests inject a fake without running OIDC discovery.
type Authenticator interface {
	Enabled() bool
	Middleware(next http.Handler) http.Handler
	AdminOrToken(token string) func(http.Handler) http.Handler
	Me(w http.ResponseWriter, r *http.Request)
	Login(w http.ResponseWriter, r *http.Request)
	Callback(w http.ResponseWriter, r *http.Request)
	Logout(w http.ResponseWriter, r *http.Request)
}

// disabledAuth keeps existing deployments and tests working with
// AUTH_ENABLED=false: resource routes pass through and eval still uses the
// static operator token.
type disabledAuth struct {
	adminToken string
}

func (a disabledAuth) Enabled() bool { return false }

func (a disabledAuth) Middleware(next http.Handler) http.Handler { return next }

func (a disabledAuth) AdminOrToken(token string) func(http.Handler) http.Handler {
	return AdminToken(token)
}

func (a disabledAuth) Me(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       false,
		"authenticated": false,
		"provider_name": "",
	})
}

func (a disabledAuth) Login(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, &Error{Status: http.StatusNotFound, Code: "auth_disabled", Message: "认证未启用"})
}

func (a disabledAuth) Callback(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, &Error{Status: http.StatusNotFound, Code: "auth_disabled", Message: "认证未启用"})
}

func (a disabledAuth) Logout(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
