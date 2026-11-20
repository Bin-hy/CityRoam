package repo

import (
	"context"
	"fmt"
)

func (s *Store) UpsertUser(ctx context.Context, arg UpsertUserParams) (User, error) {
	user, err := s.q.UpsertUser(ctx, arg)
	if err != nil {
		return User{}, fmt.Errorf("store: upsert user: %w", err)
	}
	return user, nil
}

func (s *Store) GetUserByIssuerSubject(ctx context.Context, issuer, subject string) (User, error) {
	return s.q.GetUserByIssuerSubject(ctx, GetUserByIssuerSubjectParams{Issuer: issuer, Subject: subject})
}

func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	return s.q.GetUserByID(ctx, id)
}

func (s *Store) CreateSession(ctx context.Context, arg CreateSessionParams) error {
	if err := s.q.CreateSession(ctx, arg); err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	return nil
}

// GetSessionUser loads an unexpired session together with its user in one query
// so role changes apply on the next request.
func (s *Store) GetSessionUser(ctx context.Context, id string) (Session, User, error) {
	row, err := s.q.GetSessionUser(ctx, id)
	if err != nil {
		return Session{}, User{}, err
	}
	session := Session{
		ID:        row.SessionID,
		UserID:    row.UserID,
		ExpiresAt: row.ExpiresAt,
		CreatedAt: row.SessionCreatedAt,
	}
	user := User{
		ID:            row.UserID,
		Issuer:        row.Issuer,
		Subject:       row.Subject,
		Email:         row.Email,
		EmailVerified: row.EmailVerified,
		Name:          row.Name,
		AvatarUrl:     row.AvatarUrl,
		Role:          row.Role,
		CreatedAt:     row.UserCreatedAt,
	}
	return session, user, nil
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	return s.q.DeleteSession(ctx, id)
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	return s.q.DeleteExpiredSessions(ctx)
}

func (s *Store) CreateAuthRequest(ctx context.Context, arg CreateAuthRequestParams) error {
	if err := s.q.CreateAuthRequest(ctx, arg); err != nil {
		return fmt.Errorf("store: create auth request: %w", err)
	}
	return nil
}

func (s *Store) ConsumeAuthRequest(ctx context.Context, state string) (AuthRequest, error) {
	return s.q.ConsumeAuthRequest(ctx, state)
}

func (s *Store) DeleteExpiredAuthRequests(ctx context.Context) (int64, error) {
	return s.q.DeleteExpiredAuthRequests(ctx)
}

// ListTripsByOwner returns the owner's trips with their latest run and version
// so the list endpoint stays one query.
func (s *Store) ListTripsByOwner(ctx context.Context, ownerID string, limit, offset int32) ([]ListTripsByOwnerRow, error) {
	return s.q.ListTripsByOwner(ctx, ListTripsByOwnerParams{OwnerID: &ownerID, Limit: limit, Offset: offset})
}

// ListAnonymousTrips returns trips created while auth was disabled.
func (s *Store) ListAnonymousTrips(ctx context.Context, limit, offset int32) ([]ListAnonymousTripsRow, error) {
	return s.q.ListAnonymousTrips(ctx, ListAnonymousTripsParams{Limit: limit, Offset: offset})
}
