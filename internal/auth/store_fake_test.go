package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"trippilot/internal/repo"
)

type fakeStore struct {
	mu         sync.Mutex
	now        func() time.Time
	nextUserID int
	users      map[string]repo.User
	sessions   map[string]repo.Session
	requests   map[string]repo.AuthRequest
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		now:      time.Now,
		users:    make(map[string]repo.User),
		sessions: make(map[string]repo.Session),
		requests: make(map[string]repo.AuthRequest),
	}
}

func userKey(issuer, subject string) string {
	return issuer + "\x00" + subject
}

func (s *fakeStore) UpsertUser(ctx context.Context, arg repo.UpsertUserParams) (repo.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := userKey(arg.Issuer, arg.Subject)
	user, ok := s.users[key]
	if !ok {
		s.nextUserID++
		user = repo.User{ID: fmt.Sprintf("user-%d", s.nextUserID), Issuer: arg.Issuer, Subject: arg.Subject}
	}
	user.Email = arg.Email
	user.EmailVerified = arg.EmailVerified
	user.Name = arg.Name
	user.AvatarUrl = arg.AvatarUrl
	user.Role = arg.Role
	s.users[key] = user
	return user, nil
}

func (s *fakeStore) GetUserByIssuerSubject(ctx context.Context, issuer, subject string) (repo.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userKey(issuer, subject)]
	if !ok {
		return repo.User{}, pgx.ErrNoRows
	}
	return user, nil
}

func (s *fakeStore) CreateSession(ctx context.Context, arg repo.CreateSessionParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[arg.ID] = repo.Session{
		ID:        arg.ID,
		UserID:    arg.UserID,
		ExpiresAt: arg.ExpiresAt,
		UserAgent: arg.UserAgent,
		Ip:        arg.Ip,
	}
	return nil
}

func (s *fakeStore) GetSessionUser(ctx context.Context, id string) (repo.Session, repo.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok || !session.ExpiresAt.After(s.now()) {
		return repo.Session{}, repo.User{}, pgx.ErrNoRows
	}
	for _, user := range s.users {
		if user.ID == session.UserID {
			return session, user, nil
		}
	}
	return repo.Session{}, repo.User{}, pgx.ErrNoRows
}

func (s *fakeStore) DeleteSession(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *fakeStore) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var deleted int64
	for id, session := range s.sessions {
		if !session.ExpiresAt.After(s.now()) {
			delete(s.sessions, id)
			deleted++
		}
	}
	return deleted, nil
}

func (s *fakeStore) CreateAuthRequest(ctx context.Context, arg repo.CreateAuthRequestParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[arg.State] = repo.AuthRequest{
		State:        arg.State,
		Nonce:        arg.Nonce,
		CodeVerifier: arg.CodeVerifier,
		ReturnTo:     arg.ReturnTo,
		ExpiresAt:    arg.ExpiresAt,
	}
	return nil
}

func (s *fakeStore) ConsumeAuthRequest(ctx context.Context, state string) (repo.AuthRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	request, ok := s.requests[state]
	if !ok {
		return repo.AuthRequest{}, pgx.ErrNoRows
	}
	delete(s.requests, state)
	if !request.ExpiresAt.After(s.now()) {
		return repo.AuthRequest{}, pgx.ErrNoRows
	}
	return request, nil
}

func (s *fakeStore) DeleteExpiredAuthRequests(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var deleted int64
	for state, request := range s.requests {
		if !request.ExpiresAt.After(s.now()) {
			delete(s.requests, state)
			deleted++
		}
	}
	return deleted, nil
}

func (s *fakeStore) pendingRequest(state string) (repo.AuthRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	request, ok := s.requests[state]
	return request, ok
}

func (s *fakeStore) sessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}
