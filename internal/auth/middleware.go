package auth

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) Middleware(next http.Handler) http.Handler {
	if !s.cfg.Enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, status := s.principalFromRequest(r)
		if status != 0 {
			if status == http.StatusInternalServerError {
				s.loggerFor(r.Context()).Error("auth denied", "path", r.URL.Path, "reason", "session_lookup")
				writeAuthError(w, status, "internal", "服务暂时不可用")
				return
			}
			s.loggerFor(r.Context()).Warn("auth denied", "path", r.URL.Path, "reason", "unauthorized")
			writeAuthError(w, status, "unauthorized", "请先登录")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}

// AdminOrToken accepts either the static operator token or an admin session.
// The token keeps scripts and emergency access working when no admin has
// logged in.
func (s *Service) AdminOrToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token != "" {
				provided := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
				if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
					provided = strings.TrimSpace(auth[len("bearer "):])
				}
				if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
			if !s.cfg.Enabled {
				s.loggerFor(r.Context()).Warn("auth denied", "path", r.URL.Path, "reason", "admin_token")
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", "缺少或无效的管理员令牌")
				return
			}
			principal, status := s.principalFromRequest(r)
			switch status {
			case 0:
				if !principal.IsAdmin() {
					s.loggerFor(r.Context()).Warn("auth denied", "path", r.URL.Path, "reason", "admin_required", "user_id", principal.UserID)
					writeAuthError(w, http.StatusForbidden, "forbidden", "需要管理员角色")
					return
				}
				next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
			case http.StatusInternalServerError:
				s.loggerFor(r.Context()).Error("auth denied", "path", r.URL.Path, "reason", "session_lookup")
				writeAuthError(w, status, "internal", "服务暂时不可用")
			default:
				s.loggerFor(r.Context()).Warn("auth denied", "path", r.URL.Path, "reason", "unauthorized")
				writeAuthError(w, status, "unauthorized", "请先登录")
			}
		})
	}
}

// principalFromRequest returns 0 on success, otherwise the HTTP status to send.
func (s *Service) principalFromRequest(r *http.Request) (Principal, int) {
	token, ok := sessionCookie(r)
	if !ok {
		return Principal{}, http.StatusUnauthorized
	}
	_, user, err := s.store.GetSessionUser(r.Context(), hashToken(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, http.StatusUnauthorized
		}
		s.loggerFor(r.Context()).Error("auth session lookup", "error", err)
		return Principal{}, http.StatusInternalServerError
	}
	return Principal{
		UserID:  user.ID,
		Issuer:  user.Issuer,
		Subject: user.Subject,
		Role:    user.Role,
		Email:   user.Email,
		Name:    user.Name,
	}, 0
}
