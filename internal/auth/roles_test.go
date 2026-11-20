package auth

import "testing"

func ptr(value string) *string { return &value }

func TestResolveRole(t *testing.T) {
	admins := []string{"Admin@Example.com"}
	subs := []string{"sub-admin"}
	tests := []struct {
		name          string
		email         *string
		emailVerified bool
		subject       string
		want          string
	}{
		{name: "verified email case insensitive", email: ptr("admin@example.COM"), emailVerified: true, subject: "s1", want: RoleAdmin},
		{name: "unverified email ignored", email: ptr("admin@example.com"), emailVerified: false, subject: "s1", want: RoleUser},
		{name: "subject exact", email: nil, subject: "sub-admin", want: RoleAdmin},
		{name: "subject not substring", email: nil, subject: "sub-admin-extra", want: RoleUser},
		{name: "no match", email: ptr("someone@example.com"), emailVerified: true, subject: "s2", want: RoleUser},
		{name: "nil email", email: nil, subject: "s3", want: RoleUser},
		{name: "blank email", email: ptr("   "), emailVerified: true, subject: "s4", want: RoleUser},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveRole(tt.email, tt.emailVerified, tt.subject, admins, subs); got != tt.want {
				t.Fatalf("resolveRole() = %q, want %q", got, tt.want)
			}
		})
	}
}
