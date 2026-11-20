package skills

import (
	"container/list"
	"context"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBodyCacheEntries = 64
	DefaultBodyCacheBytes   = 4 << 20
)

type CacheStats struct {
	Hits            uint64
	Misses          uint64
	SourceLoads     uint64
	Evictions       uint64
	Refreshes       uint64
	RefreshFailures uint64
	Entries         int
	Bytes           int64
}

// RemoteCache is the reserved L3 slot for a future Redis-backed cache. v1.7
// never creates an implementation and never reads Redis configuration.
type RemoteCache interface {
	Get(ctx context.Context, key string) (Skill, bool, error)
	Put(ctx context.Context, key string, skill Skill, ttl time.Duration) error
}

type cacheKey struct {
	name    string
	version string
	sha256  string
}

func newCacheKey(pin CatalogEntry) cacheKey {
	return cacheKey{name: pin.Name, version: pin.Version, sha256: strings.ToLower(pin.SHA256)}
}

type bodyEntry struct {
	key   cacheKey
	pin   CatalogEntry
	skill Skill
	size  int
}

type bodyFlight struct {
	done  chan struct{}
	skill Skill
	err   error
}

type BodyCache struct {
	mu       sync.Mutex
	lru      *list.List
	entries  map[cacheKey]*list.Element
	inflight map[cacheKey]*bodyFlight
	bytes    int64
	maxItems int
	maxBytes int64

	hits, misses, sourceLoads, evictions uint64
}

func newBodyCache(maxItems int, maxBytes int64) *BodyCache {
	if maxItems <= 0 {
		maxItems = DefaultBodyCacheEntries
	}
	if maxBytes <= 0 {
		maxBytes = DefaultBodyCacheBytes
	}
	return &BodyCache{
		lru:      list.New(),
		entries:  make(map[cacheKey]*list.Element),
		inflight: make(map[cacheKey]*bodyFlight),
		maxItems: maxItems,
		maxBytes: maxBytes,
	}
}

func (c *BodyCache) GetOrLoad(ctx context.Context, pin CatalogEntry, load func(context.Context) (Skill, error)) (Skill, error) {
	key := newCacheKey(pin)

	c.mu.Lock()
	if element, ok := c.entries[key]; ok {
		c.lru.MoveToFront(element)
		c.hits++
		skill := element.Value.(*bodyEntry).skill
		c.mu.Unlock()
		return skill, nil
	}
	if flight, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		select {
		case <-flight.done:
			return flight.skill, flight.err
		case <-ctx.Done():
			return Skill{}, ctx.Err()
		}
	}
	c.misses++
	flight := &bodyFlight{done: make(chan struct{})}
	c.inflight[key] = flight
	c.mu.Unlock()

	skill, err := load(ctx)

	c.mu.Lock()
	delete(c.inflight, key)
	c.sourceLoads++
	flight.skill = skill
	flight.err = err
	if err == nil {
		c.putLocked(key, pin, skill)
	}
	close(flight.done)
	c.mu.Unlock()
	return skill, err
}

func (c *BodyCache) warm(pin CatalogEntry, skill Skill) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putLocked(newCacheKey(pin), pin, skill)
}

func (c *BodyCache) putLocked(key cacheKey, pin CatalogEntry, skill Skill) {
	size := entrySize(skill)
	if element, ok := c.entries[key]; ok {
		entry := element.Value.(*bodyEntry)
		c.bytes += int64(size - entry.size)
		entry.pin = pin
		entry.skill = skill
		entry.size = size
		c.lru.MoveToFront(element)
	} else {
		entry := &bodyEntry{key: key, pin: pin, skill: skill, size: size}
		element := c.lru.PushFront(entry)
		c.entries[key] = element
		c.bytes += int64(size)
	}
	c.evictLocked()
}

func (c *BodyCache) evictLocked() {
	for len(c.entries) > c.maxItems || c.bytes > c.maxBytes {
		element := c.lru.Back()
		if element == nil {
			return
		}
		entry := element.Value.(*bodyEntry)
		c.lru.Remove(element)
		delete(c.entries, entry.key)
		c.bytes -= int64(entry.size)
		c.evictions++
	}
}

func (c *BodyCache) stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{
		Hits:        c.hits,
		Misses:      c.misses,
		SourceLoads: c.sourceLoads,
		Evictions:   c.evictions,
		Entries:     len(c.entries),
		Bytes:       c.bytes,
	}
}

func entrySize(skill Skill) int {
	return 64 + len(skill.Name) + len(skill.Version) + len(skill.Description) + len(skill.SHA256) + len(skill.Body)
}
