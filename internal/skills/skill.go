// Package skills loads versioned Agent Skills from a directory tree.
//
// Layout:
//
//	skills/
//	  skills.lock.json
//	  <name>/<version>/SKILL.md
//
// Only the version pinned in skills.lock.json is active. A pinned file whose
// SHA-256 differs from the lock is rejected, so an edit without a version bump
// cannot silently change the prompt seen by a run.
package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	FileName           = "SKILL.md"
	LockFileName       = "skills.lock.json"
	LockSchemaVersion  = 1
	MaxBodyBytes       = 32 << 10
	MaxDescriptionSize = 300
	MaxSkills          = 32
	MaxTotalBodyBytes  = 256 << 10
)

var (
	namePattern    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	versionPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z.-]+))?$`)
	hashPattern    = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
)

// Skill is one pinned skill. Body is stored with the run snapshot so a resumed
// run keeps the exact instructions it started with.
type Skill struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	SHA256      string `json:"sha256"`
	Body        string `json:"body"`
}

// CatalogEntry is the always-visible description of a skill. The body is only
// loaded on demand.
type CatalogEntry struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	SHA256      string `json:"sha256"`
}

// LockEntry pins one skill version to its content hash.
type LockEntry struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// Lock is the on-disk version manifest.
type Lock struct {
	Version int                  `json:"version"`
	Skills  map[string]LockEntry `json:"skills"`
}

// Registry is an immutable set of active skills, ordered by name.
type Registry struct {
	skills []Skill
	byName map[string]Skill
}

// Hash returns the SHA-256 of normalized content, so CRLF checkouts do not
// change the pinned hash.
func Hash(data []byte) string {
	sum := sha256.Sum256([]byte(normalize(string(data))))
	return hex.EncodeToString(sum[:])
}

// NewRegistry builds a registry from already-validated skills. It is mainly
// useful for tests and embedded callers; production loading goes through Load.
func NewRegistry(list ...Skill) *Registry {
	registry := &Registry{byName: make(map[string]Skill, len(list))}
	for _, skill := range list {
		if skill.Name == "" || skill.Version == "" {
			continue
		}
		if _, duplicate := registry.byName[skill.Name]; duplicate {
			continue
		}
		registry.skills = append(registry.skills, skill)
		registry.byName[skill.Name] = skill
	}
	sort.Slice(registry.skills, func(i, j int) bool {
		return registry.skills[i].Name < registry.skills[j].Name
	})
	return registry
}

// Load reads skills.lock.json and loads exactly the pinned versions. A missing
// directory means skills are disabled; a present directory with drift is an
// error so a deployment cannot silently change prompts.
func Load(dir string) (*Registry, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return NewRegistry(), nil
	}
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return NewRegistry(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("skills: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skills: %s is not a directory", dir)
	}

	dirs, err := skillDirs(dir)
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		return NewRegistry(), nil
	}
	if len(dirs) > MaxSkills {
		return nil, fmt.Errorf("skills: %d skills exceeds the limit of %d", len(dirs), MaxSkills)
	}

	lock, err := readLock(dir)
	if err != nil {
		return nil, err
	}
	used := make(map[string]struct{}, len(dirs))
	total := 0
	loaded := make([]Skill, 0, len(dirs))
	for _, name := range dirs {
		entry, pinned := lock.Skills[name]
		if !pinned {
			return nil, fmt.Errorf("skills: %s is not pinned in %s; run `go run ./cmd/skills -write` and review the diff", name, LockFileName)
		}
		path := filepath.Join(dir, name, entry.Version, FileName)
		skill, err := loadSkillFile(name, entry.Version, path)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(skill.SHA256, entry.SHA256) {
			return nil, fmt.Errorf("skills: %s@%s hash %s does not match lock %s; bump the version instead of editing a pinned skill", name, entry.Version, skill.SHA256, entry.SHA256)
		}
		total += len(skill.Body)
		if total > MaxTotalBodyBytes {
			return nil, fmt.Errorf("skills: total body size exceeds %d bytes", MaxTotalBodyBytes)
		}
		loaded = append(loaded, skill)
		used[name] = struct{}{}
	}
	for name := range lock.Skills {
		if _, ok := used[name]; !ok {
			return nil, fmt.Errorf("skills: lock pins %s but no %s/%s directory exists", name, name, lock.Skills[name].Version)
		}
	}
	if err := ValidateCatalog(catalogEntries(loaded)); err != nil {
		return nil, err
	}
	return NewRegistry(loaded...), nil
}

// Snapshot returns a copy safe to persist or hand to another component.
func (r *Registry) Snapshot() []Skill {
	if r == nil || len(r.skills) == 0 {
		return nil
	}
	return append([]Skill(nil), r.skills...)
}

// Catalog returns prompt-safe metadata for every active skill.
func (r *Registry) Catalog() []CatalogEntry {
	if r == nil || len(r.skills) == 0 {
		return nil
	}
	out := make([]CatalogEntry, 0, len(r.skills))
	for _, skill := range r.skills {
		out = append(out, skill.CatalogEntry())
	}
	return out
}

// List implements Provider for callers that cannot tolerate the disk-backed
// Service (tests and offline evaluation).
func (r *Registry) List(ctx context.Context) ([]CatalogEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.Catalog(), nil
}

// Resolve returns the exact pinned skill. The active version is never used as a
// substitute, so a run pinned to a removed version fails loudly.
func (r *Registry) Resolve(ctx context.Context, pin CatalogEntry) (Skill, error) {
	if err := ctx.Err(); err != nil {
		return Skill{}, err
	}
	skill, ok := r.Lookup(pin.Name)
	if !ok {
		return Skill{}, unavailable(UnavailableNotFound, pin.Name, pin.Version, nil)
	}
	if skill.Version != pin.Version {
		return Skill{}, unavailable(UnavailableVersionMismatch, pin.Name, pin.Version, nil)
	}
	if !strings.EqualFold(skill.SHA256, pin.SHA256) {
		return Skill{}, unavailable(UnavailableHashMismatch, pin.Name, pin.Version, nil)
	}
	return skill, nil
}

// Lookup returns one active skill by name.
func (r *Registry) Lookup(name string) (Skill, bool) {
	if r == nil {
		return Skill{}, false
	}
	skill, ok := r.byName[name]
	return skill, ok
}

// Len reports how many skills are active.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.skills)
}

// CatalogEntry flattens a skill description for the prompt.
func (s Skill) CatalogEntry() CatalogEntry {
	return CatalogEntry{
		Name:        s.Name,
		Version:     s.Version,
		Description: strings.Join(strings.Fields(s.Description), " "),
		SHA256:      s.SHA256,
	}
}

// BuildLock scans the directory and returns a lock. Existing pins are kept so
// an upgrade must change the version explicitly; warnings explain every pin
// decision the caller might want to review.
func BuildLock(dir string) (Lock, []string, error) {
	dir = strings.TrimSpace(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return Lock{}, nil, fmt.Errorf("skills: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return Lock{}, nil, fmt.Errorf("skills: %s is not a directory", dir)
	}
	previous, err := readLockIfExists(dir)
	if errors.Is(err, fs.ErrNotExist) {
		previous = Lock{Version: LockSchemaVersion, Skills: map[string]LockEntry{}}
	} else if err != nil {
		return Lock{}, nil, err
	}
	if previous.Skills == nil {
		previous.Skills = map[string]LockEntry{}
	}

	names, err := skillDirs(dir)
	if err != nil {
		return Lock{}, nil, err
	}
	if len(names) > MaxSkills {
		return Lock{}, nil, fmt.Errorf("skills: %d skills exceeds the limit of %d", len(names), MaxSkills)
	}

	lock := Lock{Version: LockSchemaVersion, Skills: make(map[string]LockEntry, len(names))}
	var warnings []string
	loaded := make([]Skill, 0, len(names))
	for _, name := range names {
		versions, err := versionDirs(filepath.Join(dir, name))
		if err != nil {
			return Lock{}, nil, err
		}
		active := ""
		if pinned, ok := previous.Skills[name]; ok {
			if contains(versions, pinned.Version) {
				active = pinned.Version
				if newest := highestVersion(versions); newest != pinned.Version {
					warnings = append(warnings, fmt.Sprintf("%s stays pinned at %s; newer %s is available and needs an explicit version change in %s", name, pinned.Version, newest, LockFileName))
				}
			} else {
				active = highestVersion(versions)
				warnings = append(warnings, fmt.Sprintf("%s was pinned at missing version %s; falling back to %s", name, pinned.Version, active))
			}
		} else {
			active = highestVersion(versions)
			if len(versions) > 1 {
				warnings = append(warnings, fmt.Sprintf("%s has no previous pin; selecting the highest version %s", name, active))
			}
		}

		path := filepath.Join(dir, name, active, FileName)
		skill, err := loadSkillFile(name, active, path)
		if err != nil {
			return Lock{}, nil, err
		}
		lock.Skills[name] = LockEntry{Version: skill.Version, SHA256: skill.SHA256}
		loaded = append(loaded, skill)
	}
	for name := range previous.Skills {
		if _, ok := lock.Skills[name]; !ok {
			warnings = append(warnings, fmt.Sprintf("%s was removed from %s", name, LockFileName))
		}
	}
	if err := ValidateCatalog(catalogEntries(loaded)); err != nil {
		return Lock{}, nil, err
	}
	sort.Strings(warnings)
	return lock, warnings, nil
}

// WriteLock writes the manifest next to the skill directories.
func WriteLock(dir string, lock Lock) error {
	if lock.Version == 0 {
		lock.Version = LockSchemaVersion
	}
	encoded, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("skills: encode lock: %w", err)
	}
	path := filepath.Join(dir, LockFileName)
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("skills: write %s: %w", path, err)
	}
	return nil
}

func readLock(dir string) (Lock, error) {
	lock, err := readLockIfExists(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return Lock{}, fmt.Errorf("skills: %s is missing in %s; run `go run ./cmd/skills -write`", LockFileName, dir)
	}
	return lock, err
}

func readLockIfExists(dir string) (Lock, error) {
	path := filepath.Join(dir, LockFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Lock{}, fs.ErrNotExist
	}
	if err != nil {
		return Lock{}, fmt.Errorf("skills: read %s: %w", path, err)
	}
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return Lock{}, fmt.Errorf("skills: decode %s: %w", path, err)
	}
	if lock.Version != LockSchemaVersion {
		return Lock{}, fmt.Errorf("skills: %s has unsupported schema version %d", path, lock.Version)
	}
	if lock.Skills == nil {
		lock.Skills = map[string]LockEntry{}
	}
	for name, entry := range lock.Skills {
		if err := validateName(name); err != nil {
			return Lock{}, fmt.Errorf("skills: lock name: %w", err)
		}
		if !versionPattern.MatchString(entry.Version) {
			return Lock{}, fmt.Errorf("skills: lock %s has invalid version %q", name, entry.Version)
		}
		if !hashPattern.MatchString(entry.SHA256) {
			return Lock{}, fmt.Errorf("skills: lock %s has invalid SHA-256", name)
		}
	}
	return lock, nil
}

func loadSkillFile(name, version, path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, fmt.Errorf("skills: read %s: %w", path, err)
	}
	hash := Hash(data)
	meta, body, err := parseFile(string(data))
	if err != nil {
		return Skill{}, fmt.Errorf("skills: parse %s: %w", path, err)
	}
	if err := validateMeta(meta, body); err != nil {
		return Skill{}, fmt.Errorf("skills: %s: %w", path, err)
	}
	if meta.Name != name {
		return Skill{}, fmt.Errorf("skills: %s declares name %q but lives under %q", path, meta.Name, name)
	}
	if meta.Version != version {
		return Skill{}, fmt.Errorf("skills: %s declares version %q but lives under %q", path, meta.Version, version)
	}
	return Skill{
		Name:        meta.Name,
		Version:     meta.Version,
		Description: meta.Description,
		SHA256:      hash,
		Body:        body,
	}, nil
}

type metadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

func parseFile(raw string) (metadata, string, error) {
	text := normalize(raw)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return metadata{}, "", errors.New("missing YAML frontmatter")
	}
	end := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			end = index
			break
		}
	}
	if end < 0 {
		return metadata{}, "", errors.New("unterminated YAML frontmatter")
	}
	front := strings.Join(lines[1:end], "\n")
	if strings.TrimSpace(front) == "" {
		return metadata{}, "", errors.New("empty YAML frontmatter")
	}
	var meta metadata
	if err := yaml.Unmarshal([]byte(front), &meta); err != nil {
		return metadata{}, "", fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	meta.Name = strings.TrimSpace(meta.Name)
	meta.Description = strings.TrimSpace(meta.Description)
	meta.Version = strings.TrimSpace(meta.Version)
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	return meta, body, nil
}

func validateMeta(meta metadata, body string) error {
	if err := validateName(meta.Name); err != nil {
		return err
	}
	if meta.Description == "" {
		return errors.New("description is required")
	}
	if strings.ContainsAny(meta.Description, "\r\n") {
		return errors.New("description must be a single line")
	}
	if len([]rune(meta.Description)) > MaxDescriptionSize {
		return fmt.Errorf("description exceeds %d characters", MaxDescriptionSize)
	}
	if !versionPattern.MatchString(meta.Version) {
		return fmt.Errorf("version %q must be semver such as 1.0.0", meta.Version)
	}
	if body == "" {
		return errors.New("skill body is empty")
	}
	if len(body) > MaxBodyBytes {
		return fmt.Errorf("skill body exceeds %d bytes", MaxBodyBytes)
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 64 {
		return errors.New("name exceeds 64 characters")
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name %q must use lowercase letters, digits and single hyphens", name)
	}
	return nil
}

func skillDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("skills: read %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if err := validateName(entry.Name()); err != nil {
			return nil, fmt.Errorf("skills: %s: %w", entry.Name(), err)
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func versionDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("skills: read %s: %w", dir, err)
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !versionPattern.MatchString(entry.Name()) {
			return nil, fmt.Errorf("skills: %s has invalid version directory %q", dir, entry.Name())
		}
		versions = append(versions, entry.Name())
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("skills: %s has no version directories", dir)
	}
	sort.Strings(versions)
	return versions, nil
}

func highestVersion(versions []string) string {
	best := ""
	for _, version := range versions {
		if best == "" || compareVersions(version, best) > 0 {
			best = version
		}
	}
	return best
}

func compareVersions(a, b string) int {
	pa, oka := parseVersion(a)
	pb, okb := parseVersion(b)
	if !oka || !okb {
		return strings.Compare(a, b)
	}
	for _, pair := range [][2]int{{pa.major, pb.major}, {pa.minor, pb.minor}, {pa.patch, pb.patch}} {
		if pair[0] != pair[1] {
			if pair[0] < pair[1] {
				return -1
			}
			return 1
		}
	}
	if pa.prerelease == pb.prerelease {
		return 0
	}
	if pa.prerelease == "" {
		return 1
	}
	if pb.prerelease == "" {
		return -1
	}
	return strings.Compare(pa.prerelease, pb.prerelease)
}

type semver struct {
	major, minor, patch int
	prerelease          string
}

func parseVersion(raw string) (semver, bool) {
	matches := versionPattern.FindStringSubmatch(raw)
	if matches == nil {
		return semver{}, false
	}
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])
	return semver{major: major, minor: minor, patch: patch, prerelease: matches[4]}, true
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func normalize(raw string) string {
	raw = strings.TrimPrefix(raw, "\ufeff")
	return strings.ReplaceAll(raw, "\r\n", "\n")
}
