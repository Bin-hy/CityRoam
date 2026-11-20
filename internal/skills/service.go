package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Provider is the only skill surface callers depend on. List serves the
// catalog; Resolve returns the exact pinned version and never substitutes the
// currently active version.
type Provider interface {
	List(ctx context.Context) ([]CatalogEntry, error)
	Resolve(ctx context.Context, pin CatalogEntry) (Skill, error)
}

const (
	UnavailableNotFound        = "not_found"
	UnavailableVersionMismatch = "version_mismatch"
	UnavailableHashMismatch    = "hash_mismatch"
	UnavailableInvalidPin      = "invalid_pin"
	UnavailableSource          = "source_error"
)

// UnavailableError keeps the machine-readable reason while avoiding paths,
// descriptions and bodies in user-facing output.
type UnavailableError struct {
	Code    string
	Name    string
	Version string
	Err     error
}

func (e *UnavailableError) Error() string {
	label := e.Name
	if e.Version != "" {
		label += "@" + e.Version
	}
	return fmt.Sprintf("skill %s unavailable (%s)", label, e.Code)
}

func (e *UnavailableError) Unwrap() error { return e.Err }

func unavailable(code, name, version string, err error) error {
	return &UnavailableError{Code: code, Name: name, Version: version, Err: err}
}

type ScanResult struct {
	Skills   []Skill
	Versions map[string][]string
}

// Source abstracts where pinned bodies live. Local files are the only
// implementation today; a database source can replace it without changing
// callers.
type Source interface {
	Scan(ctx context.Context, dir string) (ScanResult, error)
	Load(ctx context.Context, dir string, pin CatalogEntry) (Skill, error)
}

type LocalSource struct{}

func (LocalSource) Scan(ctx context.Context, dir string) (ScanResult, error) {
	if err := ctx.Err(); err != nil {
		return ScanResult{}, err
	}
	registry, err := Load(dir)
	if err != nil {
		return ScanResult{}, err
	}
	versions := make(map[string][]string)
	if registry.Len() > 0 {
		names, err := skillDirs(dir)
		if err != nil {
			return ScanResult{}, err
		}
		for _, name := range names {
			list, err := versionDirs(filepath.Join(dir, name))
			if err != nil {
				return ScanResult{}, err
			}
			versions[name] = list
		}
	}
	return ScanResult{Skills: registry.Snapshot(), Versions: versions}, nil
}

func (LocalSource) Load(ctx context.Context, dir string, pin CatalogEntry) (Skill, error) {
	if err := ctx.Err(); err != nil {
		return Skill{}, err
	}
	if err := validatePin(pin); err != nil {
		return Skill{}, unavailable(UnavailableInvalidPin, pin.Name, pin.Version, err)
	}
	if strings.TrimSpace(dir) == "" {
		return Skill{}, unavailable(UnavailableNotFound, pin.Name, pin.Version, nil)
	}
	path := filepath.Join(dir, pin.Name, pin.Version, FileName)
	skill, err := loadSkillFile(pin.Name, pin.Version, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Skill{}, unavailable(UnavailableNotFound, pin.Name, pin.Version, err)
		}
		return Skill{}, unavailable(UnavailableSource, pin.Name, pin.Version, err)
	}
	if !strings.EqualFold(skill.SHA256, pin.SHA256) {
		return Skill{}, unavailable(UnavailableHashMismatch, pin.Name, pin.Version, nil)
	}
	return skill, nil
}

func validatePin(pin CatalogEntry) error {
	if err := validateName(pin.Name); err != nil {
		return err
	}
	if !versionPattern.MatchString(pin.Version) {
		return fmt.Errorf("version %q is not semver", pin.Version)
	}
	if !hashPattern.MatchString(pin.SHA256) {
		return errors.New("sha256 is not a 64-character hex digest")
	}
	return nil
}

const DefaultRefreshInterval = 30 * time.Second

type Manifest struct {
	Catalog  []CatalogEntry
	Versions map[string][]string
}

type Service struct {
	dir       string
	source    Source
	bodies    *BodyCache
	interval  time.Duration
	now       func() time.Time
	remote    RemoteCache // L3 reserved; nil in v1.7
	refreshMu sync.Mutex

	mu              sync.RWMutex
	manifest        Manifest
	checkedAt       time.Time
	fingerprint     string
	refreshErr      error
	refreshes       uint64
	refreshFailures uint64
}

type ServiceOption func(*Service)

func WithSource(source Source) ServiceOption {
	return func(s *Service) {
		if source != nil {
			s.source = source
		}
	}
}

func WithClock(now func() time.Time) ServiceOption {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithRefreshInterval(interval time.Duration) ServiceOption {
	return func(s *Service) { s.interval = interval }
}

func Open(dir string, opts ...ServiceOption) (*Service, error) {
	s := &Service{
		dir:      strings.TrimSpace(dir),
		source:   LocalSource{},
		bodies:   newBodyCache(DefaultBodyCacheEntries, DefaultBodyCacheBytes),
		interval: DefaultRefreshInterval,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.interval <= 0 || s.interval > DefaultRefreshInterval {
		s.interval = DefaultRefreshInterval
	}
	if s.dir == "" {
		s.mu.Lock()
		s.checkedAt = s.now()
		s.mu.Unlock()
		return s, nil
	}
	fingerprint, err := localFingerprint(s.dir)
	if err != nil {
		return nil, err
	}
	result, err := s.source.Scan(context.Background(), s.dir)
	if err != nil {
		return nil, err
	}
	s.applyScan(result, fingerprint, s.now())
	s.recordRefreshSuccess()
	return s, nil
}

func (s *Service) List(ctx context.Context) ([]CatalogEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	if !s.checkedAt.IsZero() && s.now().Sub(s.checkedAt) < s.interval {
		catalog := cloneCatalog(s.manifest.Catalog)
		err := s.refreshErr
		s.mu.RUnlock()
		if err != nil {
			return nil, err
		}
		return catalog, nil
	}
	s.mu.RUnlock()
	return s.refresh(ctx)
}

func (s *Service) refresh(ctx context.Context) ([]CatalogEntry, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	s.mu.RLock()
	if !s.checkedAt.IsZero() && s.now().Sub(s.checkedAt) < s.interval {
		catalog := cloneCatalog(s.manifest.Catalog)
		err := s.refreshErr
		s.mu.RUnlock()
		if err != nil {
			return nil, err
		}
		return catalog, nil
	}
	previous := s.fingerprint
	previousErr := s.refreshErr
	s.mu.RUnlock()

	fingerprint, err := localFingerprint(s.dir)
	if err != nil {
		s.recordRefreshFailure(err)
		return nil, err
	}
	if fingerprint == previous && previousErr == nil {
		s.mu.Lock()
		s.checkedAt = s.now()
		catalog := cloneCatalog(s.manifest.Catalog)
		s.mu.Unlock()
		return catalog, nil
	}

	result, err := s.source.Scan(ctx, s.dir)
	if err != nil {
		s.recordRefreshFailure(err)
		return nil, err
	}
	s.applyScan(result, fingerprint, s.now())
	s.recordRefreshSuccess()
	s.mu.RLock()
	catalog := cloneCatalog(s.manifest.Catalog)
	s.mu.RUnlock()
	return catalog, nil
}

func (s *Service) Resolve(ctx context.Context, pin CatalogEntry) (Skill, error) {
	if err := ctx.Err(); err != nil {
		return Skill{}, err
	}
	if err := validatePin(pin); err != nil {
		return Skill{}, unavailable(UnavailableInvalidPin, pin.Name, pin.Version, err)
	}
	return s.bodies.GetOrLoad(ctx, pin, func(ctx context.Context) (Skill, error) {
		return s.source.Load(ctx, s.dir, pin)
	})
}

func (s *Service) Catalog() []CatalogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneCatalog(s.manifest.Catalog)
}

func (s *Service) Versions() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneVersions(s.manifest.Versions)
}

func (s *Service) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.manifest.Catalog)
}

func (s *Service) Stats() CacheStats {
	stats := s.bodies.stats()
	s.mu.RLock()
	stats.Refreshes = s.refreshes
	stats.RefreshFailures = s.refreshFailures
	s.mu.RUnlock()
	return stats
}

func (s *Service) applyScan(result ScanResult, fingerprint string, now time.Time) {
	for _, skill := range result.Skills {
		s.bodies.warm(skill.CatalogEntry(), skill)
	}
	s.mu.Lock()
	s.manifest = Manifest{
		Catalog:  catalogEntries(result.Skills),
		Versions: cloneVersions(result.Versions),
	}
	s.fingerprint = fingerprint
	s.checkedAt = now
	s.refreshErr = nil
	s.mu.Unlock()
}

func (s *Service) recordRefreshSuccess() {
	s.mu.Lock()
	s.refreshes++
	s.mu.Unlock()
}

func (s *Service) recordRefreshFailure(err error) {
	s.mu.Lock()
	s.refreshErr = fmt.Errorf("skills: refresh %s: %w", s.dir, err)
	s.checkedAt = s.now()
	s.refreshFailures++
	s.mu.Unlock()
}

func localFingerprint(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "absent", nil
	}
	var builder strings.Builder
	lock, err := os.ReadFile(filepath.Join(dir, LockFileName))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		builder.WriteString("lock:absent")
	case err != nil:
		return "", fmt.Errorf("skills: read %s: %w", filepath.Join(dir, LockFileName), err)
	default:
		sum := sha256.Sum256(lock)
		builder.WriteString("lock:")
		builder.WriteString(hex.EncodeToString(sum[:]))
	}

	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		builder.WriteString("|dir:absent")
		return fingerprintOf(builder.String()), nil
	}
	if err != nil {
		return "", fmt.Errorf("skills: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		builder.WriteString("|dir:file")
		return fingerprintOf(builder.String()), nil
	}
	names, err := skillDirs(dir)
	if err != nil {
		return "", err
	}
	for _, name := range names {
		versions, err := versionDirs(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		for _, version := range versions {
			path := filepath.Join(dir, name, version, FileName)
			info, err := os.Stat(path)
			if err != nil {
				fmt.Fprintf(&builder, "|%s@%s:missing", name, version)
				continue
			}
			fmt.Fprintf(&builder, "|%s@%s:%d:%d", name, version, info.Size(), info.ModTime().UnixNano())
		}
	}
	return fingerprintOf(builder.String()), nil
}

func fingerprintOf(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func cloneCatalog(entries []CatalogEntry) []CatalogEntry {
	if len(entries) == 0 {
		return nil
	}
	return append([]CatalogEntry(nil), entries...)
}

func cloneVersions(versions map[string][]string) map[string][]string {
	if len(versions) == 0 {
		return nil
	}
	out := make(map[string][]string, len(versions))
	for name, list := range versions {
		out[name] = append([]string(nil), list...)
	}
	return out
}
