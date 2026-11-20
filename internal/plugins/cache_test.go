package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"trippilot/internal/repo"
)

type fakeCacheStore struct {
	mu        sync.Mutex
	rows      map[string]repo.PluginCache
	getErr    error
	setErr    error
	deleteErr error
	deletes   int
	sets      []repo.SetPluginCacheParams
}

func (s *fakeCacheStore) GetPluginCache(ctx context.Context, key string) (repo.PluginCache, error) {
	if s.getErr != nil {
		return repo.PluginCache{}, s.getErr
	}
	row, ok := s.rows[key]
	if !ok {
		return repo.PluginCache{}, pgx.ErrNoRows
	}
	return row, nil
}

func (s *fakeCacheStore) SetPluginCache(ctx context.Context, arg repo.SetPluginCacheParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setErr != nil {
		return s.setErr
	}
	s.sets = append(s.sets, arg)
	return nil
}

func (s *fakeCacheStore) DeleteExpiredPluginCache(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes++
	return s.deleteErr
}

func cacheLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRepoCache(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	store := &fakeCacheStore{rows: map[string]repo.PluginCache{
		"fresh":   {Key: "fresh", Provider: "fliggy", Value: json.RawMessage(`{"a":1}`), ExpiresAt: now.Add(time.Hour)},
		"expired": {Key: "expired", Provider: "fliggy", Value: json.RawMessage(`{"b":2}`), ExpiresAt: now.Add(-time.Minute)},
	}}
	cache := NewRepoCache(store, WithCacheClock(func() time.Time { return now }), WithCacheLogger(cacheLogger()))

	value, ok, err := cache.Get(context.Background(), "fresh")
	if err != nil || !ok || string(value) != `{"a":1}` {
		t.Fatalf("get fresh = %s %v %v", value, ok, err)
	}
	if store.deletes != 1 {
		t.Fatalf("cleanup calls = %d", store.deletes)
	}

	if _, ok, err := cache.Get(context.Background(), "missing"); err != nil || ok {
		t.Fatalf("missing = %v %v", ok, err)
	}
	if _, ok, err := cache.Get(context.Background(), "expired"); err != nil || ok {
		t.Fatalf("expired = %v %v", ok, err)
	}
	if store.deletes != 2 {
		t.Fatalf("expired delete calls = %d", store.deletes)
	}

	if err := cache.Set(context.Background(), "new", "deepseek", json.RawMessage(`{"x":1}`), 30*time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	if len(store.sets) != 1 || !store.sets[0].ExpiresAt.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("sets = %+v", store.sets)
	}
	if err := cache.Set(context.Background(), "", "p", json.RawMessage(`{}`), time.Minute); err == nil {
		t.Fatal("empty key must fail")
	}
	if err := cache.Set(context.Background(), "k", "p", json.RawMessage(`not json`), time.Minute); err == nil {
		t.Fatal("invalid value must fail")
	}
	if err := cache.Set(context.Background(), "k", "p", json.RawMessage(`{}`), 0); err == nil {
		t.Fatal("non-positive ttl must fail")
	}
}

func TestRepoCacheErrors(t *testing.T) {
	store := &fakeCacheStore{getErr: errors.New("db down"), setErr: errors.New("db down"), deleteErr: errors.New("db down")}
	cache := NewRepoCache(store, WithCacheLogger(cacheLogger()))
	if _, _, err := cache.Get(context.Background(), "k"); err == nil {
		t.Fatal("get error must surface")
	}
	if err := cache.Set(context.Background(), "k", "p", json.RawMessage(`{}`), time.Minute); err == nil {
		t.Fatal("set error must surface")
	}
}
