package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"trippilot/internal/repo"
)

// Cache stores normalized tool results. Implementations may treat failures as
// misses; the Governor never lets caching break a research run.
type Cache interface {
	Get(ctx context.Context, key string) (json.RawMessage, bool, error)
	Set(ctx context.Context, key, provider string, value json.RawMessage, ttl time.Duration) error
}

// CacheKeyer is implemented by tools that opt into caching. It is optional so
// the Governor can wrap uncached tools unchanged.
type CacheKeyer interface {
	CacheKey(input json.RawMessage) (string, error)
	CacheTTL() time.Duration
}

type cacheStore interface {
	GetPluginCache(ctx context.Context, key string) (repo.PluginCache, error)
	SetPluginCache(ctx context.Context, arg repo.SetPluginCacheParams) error
	DeleteExpiredPluginCache(ctx context.Context) error
}

type RepoCache struct {
	store   cacheStore
	now     func() time.Time
	logger  *slog.Logger
	cleanup sync.Once
}

func NewRepoCache(store cacheStore, opts ...CacheOption) *RepoCache {
	cache := &RepoCache{
		store:  store,
		now:    time.Now,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(cache)
	}
	return cache
}

type CacheOption func(*RepoCache)

func WithCacheClock(now func() time.Time) CacheOption {
	return func(c *RepoCache) {
		if now != nil {
			c.now = now
		}
	}
}

func WithCacheLogger(logger *slog.Logger) CacheOption {
	return func(c *RepoCache) {
		if logger != nil {
			c.logger = logger
		}
	}
}

func (c *RepoCache) Get(ctx context.Context, key string) (json.RawMessage, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, false, fmt.Errorf("plugins: cache key is required")
	}
	c.cleanup.Do(func() {
		if err := c.store.DeleteExpiredPluginCache(ctx); err != nil {
			c.logger.Warn("plugins: expired cache cleanup failed", "error", err)
		}
	})

	row, err := c.store.GetPluginCache(ctx, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("plugins: get cache: %w", err)
	}
	if !row.ExpiresAt.After(c.now()) {
		if err := c.store.DeleteExpiredPluginCache(ctx); err != nil {
			c.logger.Warn("plugins: delete expired cache failed", "error", err)
		}
		return nil, false, nil
	}
	return append(json.RawMessage(nil), row.Value...), true, nil
}

func (c *RepoCache) Set(ctx context.Context, key, provider string, value json.RawMessage, ttl time.Duration) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("plugins: cache key is required")
	}
	if ttl <= 0 {
		return fmt.Errorf("plugins: cache ttl must be positive")
	}
	if !json.Valid(value) {
		return fmt.Errorf("plugins: cache value is not valid JSON")
	}
	err := c.store.SetPluginCache(ctx, repo.SetPluginCacheParams{
		Key:       key,
		Provider:  provider,
		Value:     value,
		ExpiresAt: c.now().Add(ttl),
	})
	if err != nil {
		return fmt.Errorf("plugins: set cache: %w", err)
	}
	return nil
}
