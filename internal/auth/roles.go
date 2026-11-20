package auth

import "strings"

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// resolveRole maps an IdP identity onto the local role. Email matching is
// case-insensitive but only counts for verified addresses; subject matching is
// exact. Anything else stays a regular user.
func resolveRole(email *string, emailVerified bool, subject string, adminEmails, adminSubs []string) string {
	if emailVerified && email != nil {
		candidate := strings.TrimSpace(*email)
		if candidate != "" {
			for _, allowed := range adminEmails {
				if strings.EqualFold(strings.TrimSpace(allowed), candidate) {
					return RoleAdmin
				}
			}
		}
	}
	for _, allowed := range adminSubs {
		if strings.TrimSpace(allowed) == subject {
			return RoleAdmin
		}
	}
	return RoleUser
}
