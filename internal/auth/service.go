package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"trippilot/internal/repo"
)

type Config struct {
	Enabled               bool
	CookieSecure          string
	Issuer                string
	ClientID              string
	ClientSecret          string
	RedirectURI           string
	Scopes                []string
	Audience              string
	ProviderName          string
	PostLogoutRedirectURI string
	WebBaseURL            string
	SessionTTL            time.Duration
	AdminEmails           []string
	AdminSubs             []string
}

type Store interface {
	UpsertUser(ctx context.Context, arg repo.UpsertUserParams) (repo.User, error)
	GetUserByIssuerSubject(ctx context.Context, issuer, subject string) (repo.User, error)
	CreateSession(ctx context.Context, arg repo.CreateSessionParams) error
	GetSessionUser(ctx context.Context, id string) (repo.Session, repo.User, error)
	DeleteSession(ctx context.Context, id string) error
	DeleteExpiredSessions(ctx context.Context) (int64, error)
	CreateAuthRequest(ctx context.Context, arg repo.CreateAuthRequestParams) error
	ConsumeAuthRequest(ctx context.Context, state string) (repo.AuthRequest, error)
	DeleteExpiredAuthRequests(ctx context.Context) (int64, error)
}

type Service struct {
	cfg    Config
	store  Store
	logger *slog.Logger
	now    func() time.Time

	provider            *oidc.Provider
	oauth2Config        oauth2.Config
	idTokenVerifier     *oidc.IDTokenVerifier
	accessTokenVerifier *oidc.IDTokenVerifier
	endSessionEndpoint  string
}

const cleanupInterval = 6 * time.Hour

func New(ctx context.Context, cfg Config, store Store, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	service := &Service{cfg: cfg, store: store, logger: logger, now: time.Now}
	if !cfg.Enabled {
		return service, nil
	}
	if store == nil {
		return nil, errors.New("auth: store is required when auth is enabled")
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(discoveryCtx, cfg.Issuer)
	if err != nil {
		return nil, errors.New("auth: discover OIDC provider " + cfg.Issuer + ": " + err.Error())
	}
	service.provider = provider
	service.oauth2Config = oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cfg.RedirectURI,
		Scopes:       cfg.Scopes,
	}
	service.idTokenVerifier = provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	var discovery struct {
		JWKSURI            string `json:"jwks_uri"`
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&discovery); err != nil {
		return nil, errors.New("auth: read provider metadata: " + err.Error())
	}
	service.endSessionEndpoint = discovery.EndSessionEndpoint
	if cfg.Audience != "" {
		if discovery.JWKSURI == "" {
			return nil, errors.New("auth: provider exposes no jwks_uri for access token verification")
		}
		keySet := oidc.NewRemoteKeySet(context.Background(), discovery.JWKSURI)
		service.accessTokenVerifier = oidc.NewVerifier(cfg.Issuer, keySet, &oidc.Config{ClientID: cfg.Audience})
	}
	return service, nil
}

func (s *Service) Enabled() bool {
	return s.cfg.Enabled
}

func (s *Service) Cleanup(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	sessions, err := s.store.DeleteExpiredSessions(ctx)
	if err != nil {
		return errors.New("auth: delete expired sessions: " + err.Error())
	}
	requests, err := s.store.DeleteExpiredAuthRequests(ctx)
	if err != nil {
		return errors.New("auth: delete expired auth requests: " + err.Error())
	}
	if sessions > 0 || requests > 0 {
		s.logger.Info("auth cleanup", "sessions", sessions, "auth_requests", requests)
	}
	return nil
}

// StartCleanup runs a cleanup pass immediately and then on a fixed interval
// until ctx is cancelled.
func (s *Service) StartCleanup(ctx context.Context, interval time.Duration) {
	if s.store == nil {
		return
	}
	if interval <= 0 {
		interval = cleanupInterval
	}
	go func() {
		if err := s.Cleanup(ctx); err != nil {
			s.logger.Warn("auth cleanup failed", "error", err)
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.Cleanup(ctx); err != nil {
					s.logger.Warn("auth cleanup failed", "error", err)
				}
			}
		}
	}()
}

type authMeUser struct {
	ID    string  `json:"id"`
	Email *string `json:"email"`
	Name  *string `json:"name"`
	Role  string  `json:"role"`
}

type authMeResponse struct {
	Enabled       bool        `json:"enabled"`
	Authenticated bool        `json:"authenticated"`
	ProviderName  string      `json:"provider_name"`
	User          *authMeUser `json:"user,omitempty"`
}

func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	response := authMeResponse{Enabled: s.cfg.Enabled, ProviderName: s.cfg.ProviderName}
	if s.cfg.Enabled {
		if token, ok := sessionCookie(r); ok {
			_, user, err := s.store.GetSessionUser(r.Context(), hashToken(token))
			switch {
			case err == nil:
				response.Authenticated = true
				response.User = &authMeUser{ID: user.ID, Email: user.Email, Name: user.Name, Role: user.Role}
			case !errors.Is(err, pgx.ErrNoRows):
				s.loggerFor(r.Context()).Error("auth me session lookup", "error", err)
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	secure := cookieSecure(s.cfg.CookieSecure, r)
	if token, ok := sessionCookie(r); ok && s.store != nil {
		if _, user, err := s.store.GetSessionUser(r.Context(), hashToken(token)); err == nil {
			s.loggerFor(r.Context()).Info("auth logout", "user_id", user.ID)
		}
		if err := s.store.DeleteSession(r.Context(), hashToken(token)); err != nil {
			s.loggerFor(r.Context()).Warn("auth logout delete session", "error", err)
		}
	}
	clearSessionCookie(w, secure)
	clearOIDCCookie(w, secure)

	if s.cfg.Enabled && s.endSessionEndpoint != "" {
		target, err := s.endSessionURL()
		if err != nil {
			s.loggerFor(r.Context()).Warn("auth logout end session url", "error", err)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"redirect_url": target})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) endSessionURL() (string, error) {
	parsed, err := url.Parse(s.endSessionEndpoint)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	if s.cfg.PostLogoutRedirectURI != "" {
		query.Set("post_logout_redirect_uri", s.cfg.PostLogoutRedirectURI)
	}
	if s.cfg.ClientID != "" {
		query.Set("client_id", s.cfg.ClientID)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

func requestIP(r *http.Request) *string {
	host := r.RemoteAddr
	if idx := strings.LastIndexByte(host, ':'); idx > 0 {
		host = host[:idx]
	}
	if host == "" {
		return nil
	}
	return &host
}

func truncate(value string, limit int) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > limit {
		value = value[:limit]
	}
	return &value
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
