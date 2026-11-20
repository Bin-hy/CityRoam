package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"trippilot/internal/repo"
)

const (
	authRequestTTL = 10 * time.Minute
	maxReturnTo    = 2048
)

func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Enabled {
		writeAuthError(w, http.StatusNotFound, "auth_disabled", "认证未启用")
		return
	}
	returnTo, ok := sanitizeReturnTo(r.URL.Query().Get("return_to"))
	if !ok {
		writeAuthError(w, http.StatusBadRequest, "invalid_return_to", "return_to 必须是站内相对路径")
		return
	}
	state, err := randomURLToken()
	if err != nil {
		s.loggerFor(r.Context()).Error("auth login state", "error", err)
		writeAuthError(w, http.StatusInternalServerError, "oidc_error", "登录暂时不可用")
		return
	}
	nonce, err := randomURLToken()
	if err != nil {
		s.loggerFor(r.Context()).Error("auth login nonce", "error", err)
		writeAuthError(w, http.StatusInternalServerError, "oidc_error", "登录暂时不可用")
		return
	}
	verifier, err := randomURLToken()
	if err != nil {
		s.loggerFor(r.Context()).Error("auth login verifier", "error", err)
		writeAuthError(w, http.StatusInternalServerError, "oidc_error", "登录暂时不可用")
		return
	}
	if err := s.store.CreateAuthRequest(r.Context(), repo.CreateAuthRequestParams{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: verifier,
		ReturnTo:     returnTo,
		ExpiresAt:    s.now().Add(authRequestTTL),
	}); err != nil {
		s.loggerFor(r.Context()).Error("auth login store request", "error", err)
		writeAuthError(w, http.StatusInternalServerError, "oidc_error", "登录暂时不可用")
		return
	}

	challenge := pkceChallenge(verifier)
	authURL := s.oauth2Config.AuthCodeURL(state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	setOIDCCookie(w, state, cookieSecure(s.cfg.CookieSecure, r))
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Service) Callback(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Enabled {
		writeAuthError(w, http.StatusNotFound, "auth_disabled", "认证未启用")
		return
	}
	secure := cookieSecure(s.cfg.CookieSecure, r)
	defer clearOIDCCookie(w, secure)

	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		writeAuthError(w, http.StatusBadRequest, "login_state_invalid", "登录回调缺少必要参数")
		return
	}
	if !oidcCookieMatches(r, state) {
		s.redirectToLogin(w, r, "login_state_invalid")
		return
	}
	pending, err := s.store.ConsumeAuthRequest(r.Context(), state)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.redirectToLogin(w, r, "login_state_invalid")
			return
		}
		s.loggerFor(r.Context()).Error("auth callback consume request", "error", err)
		s.redirectToLogin(w, r, "oidc_error")
		return
	}

	tokens, err := s.oauth2Config.Exchange(r.Context(), code, oauth2.SetAuthURLParam("code_verifier", pending.CodeVerifier))
	if err != nil {
		s.loggerFor(r.Context()).Warn("auth callback exchange", "error", err)
		s.redirectToLogin(w, r, "auth_failed")
		return
	}
	rawIDToken, _ := tokens.Extra("id_token").(string)
	if rawIDToken == "" {
		s.loggerFor(r.Context()).Warn("auth callback missing id token")
		s.redirectToLogin(w, r, "auth_failed")
		return
	}
	idToken, err := s.idTokenVerifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		s.loggerFor(r.Context()).Warn("auth callback id token", "error", err)
		s.redirectToLogin(w, r, "auth_failed")
		return
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(pending.Nonce)) != 1 {
		s.loggerFor(r.Context()).Warn("auth callback nonce mismatch")
		s.redirectToLogin(w, r, "auth_failed")
		return
	}
	if s.accessTokenVerifier != nil {
		if _, err := s.accessTokenVerifier.Verify(r.Context(), tokens.AccessToken); err != nil {
			s.loggerFor(r.Context()).Warn("auth callback access token", "error", err)
			s.redirectToLogin(w, r, "auth_failed")
			return
		}
	}

	var claims struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		s.loggerFor(r.Context()).Warn("auth callback id claims", "error", err)
		s.redirectToLogin(w, r, "auth_failed")
		return
	}
	subject := claims.Subject
	email := claims.Email
	emailVerified := claims.EmailVerified
	name := claims.Name
	picture := claims.Picture

	userInfo, err := s.provider.UserInfo(r.Context(), oauth2.StaticTokenSource(tokens))
	if err != nil {
		s.loggerFor(r.Context()).Warn("auth callback userinfo fallback", "error", err)
	} else {
		if userInfo.Subject != "" {
			subject = userInfo.Subject
		}
		if userInfo.Email != "" {
			email = userInfo.Email
		}
		emailVerified = userInfo.EmailVerified
		var extra struct {
			Name    string `json:"name"`
			Picture string `json:"picture"`
		}
		if err := userInfo.Claims(&extra); err == nil {
			if extra.Name != "" {
				name = extra.Name
			}
			if extra.Picture != "" {
				picture = extra.Picture
			}
		}
	}
	if subject == "" {
		s.loggerFor(r.Context()).Warn("auth callback missing subject")
		s.redirectToLogin(w, r, "auth_failed")
		return
	}

	emailPtr := optionalString(email)
	role := resolveRole(emailPtr, emailVerified, subject, s.cfg.AdminEmails, s.cfg.AdminSubs)
	if previous, err := s.store.GetUserByIssuerSubject(r.Context(), s.cfg.Issuer, subject); err == nil && previous.Role != role {
		s.loggerFor(r.Context()).Info("auth role changed", "user_id", previous.ID, "from", previous.Role, "to", role)
	}
	user, err := s.store.UpsertUser(r.Context(), repo.UpsertUserParams{
		Issuer:        s.cfg.Issuer,
		Subject:       subject,
		Email:         emailPtr,
		EmailVerified: emailVerified,
		Name:          optionalString(name),
		AvatarUrl:     optionalString(picture),
		Role:          role,
	})
	if err != nil {
		s.loggerFor(r.Context()).Error("auth callback upsert user", "error", err)
		s.redirectToLogin(w, r, "auth_failed")
		return
	}

	token, hash, err := newSessionToken()
	if err != nil {
		s.loggerFor(r.Context()).Error("auth callback session token", "error", err)
		s.redirectToLogin(w, r, "auth_failed")
		return
	}
	if err := s.store.CreateSession(r.Context(), repo.CreateSessionParams{
		ID:        hash,
		UserID:    user.ID,
		ExpiresAt: s.now().Add(s.cfg.SessionTTL),
		UserAgent: truncate(r.UserAgent(), 512),
		Ip:        requestIP(r),
	}); err != nil {
		s.loggerFor(r.Context()).Error("auth callback create session", "error", err)
		s.redirectToLogin(w, r, "auth_failed")
		return
	}
	setSessionCookie(w, token, s.cfg.SessionTTL, secure)
	s.loggerFor(r.Context()).Info("auth login success", "user_id", user.ID, "role", user.Role)
	http.Redirect(w, r, s.returnURL(pending.ReturnTo), http.StatusFound)
}

func (s *Service) redirectToLogin(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, s.webURL("/login?error="+code), http.StatusFound)
}

func (s *Service) returnURL(returnTo string) string {
	if returnTo == "" {
		returnTo = "/"
	}
	return s.webURL(returnTo)
}

func (s *Service) webURL(path string) string {
	base := strings.TrimSuffix(s.cfg.WebBaseURL, "/")
	if base == "" {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

// sanitizeReturnTo only accepts same-site absolute paths so login cannot be
// turned into an open redirect.
func sanitizeReturnTo(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/", true
	}
	if len(raw) > maxReturnTo || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "", false
	}
	if strings.Contains(raw, "\\") {
		return "", false
	}
	for _, char := range raw {
		if unicode.IsControl(char) {
			return "", false
		}
	}
	return raw, true
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
