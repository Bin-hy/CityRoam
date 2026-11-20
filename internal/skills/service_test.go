package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	mu      sync.Mutex
	scans   int
	loads   int
	result  ScanResult
	scanErr error
	loadErr error
	bodies  map[string]Skill
	onLoad  func()
}

func (f *fakeSource) Scan(context.Context, string) (ScanResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scans++
	if f.scanErr != nil {
		return ScanResult{}, f.scanErr
	}
	return ScanResult{
		Skills:   append([]Skill(nil), f.result.Skills...),
		Versions: cloneVersions(f.result.Versions),
	}, nil
}

func (f *fakeSource) Load(_ context.Context, _ string, pin CatalogEntry) (Skill, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	if f.onLoad != nil {
		f.onLoad()
	}
	if f.loadErr != nil {
		return Skill{}, f.loadErr
	}
	skill, ok := f.bodies[pin.Name+"@"+pin.Version]
	if !ok {
		return Skill{}, unavailable(UnavailableNotFound, pin.Name, pin.Version, nil)
	}
	return skill, nil
}

func (f *fakeSource) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.scans, f.loads
}

func (f *fakeSource) setScanErr(err error) {
	f.mu.Lock()
	f.scanErr = err
	f.mu.Unlock()
}

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(delta time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(delta)
	c.mu.Unlock()
}

func serviceSkill(name, version, body string) Skill {
	return Skill{
		Name:        name,
		Version:     version,
		Description: "用于服务测试",
		SHA256:      strings.Repeat("a", 64),
		Body:        body,
	}
}

func servicePin(skill Skill) CatalogEntry {
	return CatalogEntry{Name: skill.Name, Version: skill.Version, Description: skill.Description, SHA256: skill.SHA256}
}

func newTestService(t *testing.T, source *fakeSource, clock *testClock, interval time.Duration) *Service {
	t.Helper()
	service, err := Open(t.TempDir(),
		WithSource(source),
		WithClock(clock.Now),
		WithRefreshInterval(interval),
	)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return service
}

func TestServiceListCachesManifest(t *testing.T) {
	clock := &testClock{now: time.Unix(1000, 0)}
	skill := serviceSkill("a", "1.0.0", "body")
	source := &fakeSource{result: ScanResult{Skills: []Skill{skill}}}
	service := newTestService(t, source, clock, 10*time.Millisecond)

	for index := 0; index < 3; index++ {
		catalog, err := service.List(context.Background())
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(catalog) != 1 || catalog[0].Name != "a" {
			t.Fatalf("catalog = %+v", catalog)
		}
	}
	clock.Advance(5 * time.Millisecond)
	if _, err := service.List(context.Background()); err != nil {
		t.Fatalf("list: %v", err)
	}
	scans, _ := source.counts()
	if scans != 1 {
		t.Fatalf("scans = %d", scans)
	}
	if stats := service.Stats(); stats.Refreshes != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestServiceRefreshesOnFingerprintChange(t *testing.T) {
	clock := &testClock{now: time.Unix(1000, 0)}
	dir := t.TempDir()
	first := serviceSkill("a", "1.0.0", "body")
	second := serviceSkill("b", "1.0.0", "body")
	source := &fakeSource{result: ScanResult{Skills: []Skill{first}}}
	service, err := Open(dir, WithSource(source), WithClock(clock.Now), WithRefreshInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, LockFileName), []byte("lock-a"), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	clock.Advance(20 * time.Millisecond)
	source.mu.Lock()
	source.result.Skills = []Skill{first, second}
	source.mu.Unlock()

	catalog, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(catalog) != 2 {
		t.Fatalf("catalog = %+v", catalog)
	}
	scans, _ := source.counts()
	if scans != 2 {
		t.Fatalf("scans = %d", scans)
	}
}

func TestServiceRefreshFailureKeepsCatalog(t *testing.T) {
	clock := &testClock{now: time.Unix(1000, 0)}
	dir := t.TempDir()
	skill := serviceSkill("a", "1.0.0", "body")
	source := &fakeSource{result: ScanResult{Skills: []Skill{skill}}}
	service, err := Open(dir, WithSource(source), WithClock(clock.Now), WithRefreshInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, LockFileName), []byte("lock-a"), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	source.setScanErr(errors.New("broken"))
	clock.Advance(20 * time.Millisecond)

	if _, err := service.List(context.Background()); err == nil {
		t.Fatal("expected refresh failure")
	}
	if catalog := service.Catalog(); len(catalog) != 1 {
		t.Fatalf("old catalog lost: %+v", catalog)
	}
	scansBefore, _ := source.counts()
	if _, err := service.List(context.Background()); err == nil {
		t.Fatal("expected cached refresh failure")
	}
	scansAfter, _ := source.counts()
	if scansAfter != scansBefore {
		t.Fatalf("failure did not back off: %d -> %d", scansBefore, scansAfter)
	}

	source.setScanErr(nil)
	clock.Advance(20 * time.Millisecond)
	if _, err := service.List(context.Background()); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	if stats := service.Stats(); stats.RefreshFailures != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestServiceResolveUsesL2AndSurvivesRefreshFailure(t *testing.T) {
	clock := &testClock{now: time.Unix(1000, 0)}
	dir := t.TempDir()
	warm := serviceSkill("a", "1.0.0", "warm body")
	cold := serviceSkill("b", "1.0.0", "cold body")
	source := &fakeSource{
		result: ScanResult{Skills: []Skill{warm}},
		bodies: map[string]Skill{"a@1.0.0": warm, "b@1.0.0": cold},
	}
	service, err := Open(dir, WithSource(source), WithClock(clock.Now), WithRefreshInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, LockFileName), []byte("lock-a"), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	source.setScanErr(errors.New("broken"))
	clock.Advance(20 * time.Millisecond)
	if _, err := service.List(context.Background()); err == nil {
		t.Fatal("expected refresh failure")
	}

	skill, err := service.Resolve(context.Background(), servicePin(warm))
	if err != nil {
		t.Fatalf("warm resolve: %v", err)
	}
	if skill.Body != "warm body" {
		t.Fatalf("warm body = %q", skill.Body)
	}
	if _, loads := source.counts(); loads != 0 {
		t.Fatalf("warm resolve hit source: %d", loads)
	}

	skill, err = service.Resolve(context.Background(), servicePin(cold))
	if err != nil {
		t.Fatalf("cold resolve: %v", err)
	}
	if skill.Body != "cold body" {
		t.Fatalf("cold body = %q", skill.Body)
	}
	if _, loads := source.counts(); loads != 1 {
		t.Fatalf("cold loads = %d", loads)
	}
}

func TestServiceResolveRejectsInvalidPin(t *testing.T) {
	clock := &testClock{now: time.Unix(1000, 0)}
	service := newTestService(t, &fakeSource{}, clock, 10*time.Millisecond)
	_, err := service.Resolve(context.Background(), CatalogEntry{Name: "../x", Version: "1.0.0", SHA256: "bad"})
	if err == nil {
		t.Fatal("invalid pin accepted")
	}
}

func TestOpenFailsOnInitialScanError(t *testing.T) {
	source := &fakeSource{scanErr: errors.New("broken")}
	if _, err := Open(t.TempDir(), WithSource(source)); err == nil {
		t.Fatal("expected open failure")
	}
}
