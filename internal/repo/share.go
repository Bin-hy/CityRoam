package repo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

const shareSlugAttempts = 5

// CreateShareLink allocates a random slug and retries if the slug is taken.
func (s *Store) CreateShareLink(ctx context.Context, tripID, versionID string) (ShareLink, error) {
	for attempt := 0; attempt < shareSlugAttempts; attempt++ {
		slug, err := newSlug()
		if err != nil {
			return ShareLink{}, err
		}
		link, err := s.InsertShareLink(ctx, slug, tripID, versionID)
		if err == nil {
			return link, nil
		}
		if !isUniqueViolation(err) {
			return ShareLink{}, err
		}
	}
	return ShareLink{}, errors.New("store: could not allocate a unique share slug")
}

func (s *Store) InsertShareLink(ctx context.Context, slug, tripID, versionID string) (ShareLink, error) {
	link, err := s.q.CreateShareLink(ctx, CreateShareLinkParams{
		Slug:      slug,
		TripID:    tripID,
		VersionID: versionID,
	})
	if err != nil {
		return ShareLink{}, fmt.Errorf("store: create share link: %w", err)
	}
	return link, nil
}

func (s *Store) GetShareLink(ctx context.Context, slug string) (ShareLink, error) {
	return s.q.GetShareLink(ctx, slug)
}

func (s *Store) RevokeShareLink(ctx context.Context, slug string) error {
	return s.q.RevokeShareLink(ctx, slug)
}

// newSlug returns a 12-character slug from a 32-symbol alphabet, so each
// byte maps without modulo bias.
func newSlug() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	const length = 12

	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate share slug: %w", err)
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])&31]
	}
	return string(buf), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
