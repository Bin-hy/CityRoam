package auth

import "context"

// Principal is the authenticated caller attached to a request context.
type Principal struct {
	UserID  string
	Issuer  string
	Subject string
	Role    string
	Email   *string
	Name    *string
}

func (p Principal) IsAdmin() bool {
	return p.Role == "admin"
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok
}
