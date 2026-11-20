package skills

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func cachePin(name string) CatalogEntry {
	return CatalogEntry{Name: name, Version: "1.0.0", SHA256: strings.Repeat("a", 64), Description: "用于缓存测试"}
}

func cacheSkill(pin CatalogEntry) Skill {
	return Skill{Name: pin.Name, Version: pin.Version, Description: pin.Description, SHA256: pin.SHA256, Body: "body"}
}

func TestBodyCacheHitsAndMisses(t *testing.T) {
	cache := newBodyCache(8, 1<<20)
	pin := cachePin("a")
	loads := 0
	load := func(context.Context) (Skill, error) {
		loads++
		return cacheSkill(pin), nil
	}
	if _, err := cache.GetOrLoad(context.Background(), pin, load); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := cache.GetOrLoad(context.Background(), pin, load); err != nil {
		t.Fatalf("hit: %v", err)
	}
	if loads != 1 {
		t.Fatalf("loads = %d", loads)
	}
	stats := cache.stats()
	if stats.Hits != 1 || stats.Misses != 1 || stats.SourceLoads != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestBodyCacheCoalescesConcurrentMisses(t *testing.T) {
	cache := newBodyCache(8, 1<<20)
	pin := cachePin("a")
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	loads := 0
	load := func(context.Context) (Skill, error) {
		once.Do(func() {
			loads++
			close(started)
			<-release
		})
		return cacheSkill(pin), nil
	}

	const callers = 10
	results := make(chan Skill, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for index := 0; index < callers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			skill, err := cache.GetOrLoad(context.Background(), pin, load)
			results <- skill
			errs <- err
		}()
	}
	<-started
	close(release)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("load error: %v", err)
		}
	}
	for skill := range results {
		if skill.Name != "a" {
			t.Fatalf("skill = %+v", skill)
		}
	}
	if loads != 1 {
		t.Fatalf("loads = %d", loads)
	}
	if got := cache.stats().SourceLoads; got != 1 {
		t.Fatalf("source loads = %d", got)
	}
}

func TestBodyCacheDoesNotStoreErrors(t *testing.T) {
	cache := newBodyCache(8, 1<<20)
	pin := cachePin("a")
	loads := 0
	load := func(context.Context) (Skill, error) {
		loads++
		return Skill{}, errors.New("boom")
	}
	for index := 0; index < 2; index++ {
		if _, err := cache.GetOrLoad(context.Background(), pin, load); err == nil {
			t.Fatal("expected error")
		}
	}
	if loads != 2 {
		t.Fatalf("loads = %d", loads)
	}
	if stats := cache.stats(); stats.Entries != 0 || stats.SourceLoads != 2 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestBodyCacheEvictsByCountAndBytes(t *testing.T) {
	cache := newBodyCache(1, 1<<20)
	first := cachePin("a")
	second := cachePin("b")
	for _, pin := range []CatalogEntry{first, second} {
		skill := cacheSkill(pin)
		if _, err := cache.GetOrLoad(context.Background(), pin, func(context.Context) (Skill, error) {
			return skill, nil
		}); err != nil {
			t.Fatalf("load: %v", err)
		}
	}
	stats := cache.stats()
	if stats.Entries != 1 || stats.Evictions == 0 {
		t.Fatalf("stats = %+v", stats)
	}

	recovered := false
	if _, err := cache.GetOrLoad(context.Background(), first, func(context.Context) (Skill, error) {
		recovered = true
		return cacheSkill(first), nil
	}); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if !recovered {
		t.Fatal("evicted entry was not reloaded from source")
	}

	tiny := newBodyCache(8, 1)
	if _, err := tiny.GetOrLoad(context.Background(), first, func(context.Context) (Skill, error) {
		return cacheSkill(first), nil
	}); err != nil {
		t.Fatalf("tiny load: %v", err)
	}
	if stats := tiny.stats(); stats.Entries != 0 || stats.Bytes != 0 {
		t.Fatalf("tiny stats = %+v", stats)
	}
}

func TestBodyCacheWarmDoesNotCountAsSourceLoad(t *testing.T) {
	cache := newBodyCache(8, 1<<20)
	pin := cachePin("a")
	cache.warm(pin, cacheSkill(pin))
	if _, err := cache.GetOrLoad(context.Background(), pin, func(context.Context) (Skill, error) {
		t.Fatal("warm entry missed")
		return Skill{}, nil
	}); err != nil {
		t.Fatalf("hit: %v", err)
	}
	if stats := cache.stats(); stats.SourceLoads != 0 || stats.Hits != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}
