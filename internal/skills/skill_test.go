package skills

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, dir, name, version, body string) string {
	t.Helper()
	path := filepath.Join(dir, name, version, FileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: test skill\ndescription_note: ignored\nversion: " + version + "\n---\n\n" + body
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	return path
}

func pinSkills(t *testing.T, dir string) Lock {
	t.Helper()
	lock, _, err := BuildLock(dir)
	if err != nil {
		t.Fatalf("build lock: %v", err)
	}
	if err := WriteLock(dir, lock); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	return lock
}

func TestLoadPinnedSkill(t *testing.T) {
	dir := t.TempDir()
	path := writeSkill(t, dir, "research-playbook", "1.0.0", "先核对闭馆日，再排路线。")
	lock := pinSkills(t, dir)

	registry, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if registry.Len() != 1 {
		t.Fatalf("len = %d", registry.Len())
	}
	skill, ok := registry.Lookup("research-playbook")
	if !ok {
		t.Fatal("skill not found")
	}
	if skill.Version != "1.0.0" || !strings.Contains(skill.Body, "闭馆日") {
		t.Fatalf("skill = %+v", skill)
	}
	if skill.SHA256 != lock.Skills["research-playbook"].SHA256 {
		t.Fatalf("hash = %s, lock = %s", skill.SHA256, lock.Skills["research-playbook"].SHA256)
	}
	if !strings.Contains(skill.CatalogEntry().Description, "test skill") {
		t.Fatalf("catalog = %+v", skill.CatalogEntry())
	}
	if got := Hash(mustRead(t, path)); got != skill.SHA256 {
		t.Fatalf("Hash(file) = %s, skill hash = %s", got, skill.SHA256)
	}
}

func TestLoadRejectsHashDrift(t *testing.T) {
	dir := t.TempDir()
	path := writeSkill(t, dir, "playbook", "1.0.0", "original body")
	pinSkills(t, dir)

	if err := os.WriteFile(path, []byte(strings.ReplaceAll(mustReadString(t, path), "original body", "edited body")), 0o644); err != nil {
		t.Fatalf("edit skill: %v", err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "does not match lock") {
		t.Fatalf("expected hash drift error, got %v", err)
	}
}

func TestLoadRejectsUnpinnedSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "playbook", "1.0.0", "body")
	pinSkills(t, dir)
	writeSkill(t, dir, "extra", "1.0.0", "body")

	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "not pinned") {
		t.Fatalf("expected unpinned error, got %v", err)
	}
}

func TestLoadKeepsPinnedVersionWhenNewerExists(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "playbook", "1.0.0", "old body")
	lock := pinSkills(t, dir)
	writeSkill(t, dir, "playbook", "1.1.0", "new body")

	registry, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	skill, _ := registry.Lookup("playbook")
	if skill.Version != "1.0.0" || skill.Body != "old body" {
		t.Fatalf("pinned skill changed: %+v", skill)
	}
	if lock.Skills["playbook"].Version != "1.0.0" {
		t.Fatalf("lock = %+v", lock)
	}

	next, warnings, err := BuildLock(dir)
	if err != nil {
		t.Fatalf("build lock: %v", err)
	}
	if next.Skills["playbook"].Version != "1.0.0" {
		t.Fatalf("build lock silently upgraded: %+v", next.Skills["playbook"])
	}
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, " "), "newer 1.1.0") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestLoadRejectsTooManySkills(t *testing.T) {
	dir := t.TempDir()
	for index := 0; index < MaxSkills+1; index++ {
		path := filepath.Join(dir, fmt.Sprintf("skill-%02d", index), "1.0.0")
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "exceeds the limit of 32") {
		t.Fatalf("expected skill count error, got %v", err)
	}
}

func TestLoadRejectsTotalBodyBudget(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("a", MaxBodyBytes)
	count := MaxTotalBodyBytes/MaxBodyBytes + 1
	for index := 0; index < count; index++ {
		writeSkill(t, dir, fmt.Sprintf("skill-%02d", index), "1.0.0", body)
	}
	pinSkills(t, dir)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "total body size exceeds") {
		t.Fatalf("expected total body error, got %v", err)
	}
}

func TestLoadRejectsInvalidSkill(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "missing version",
			content: "---\nname: playbook\ndescription: test\n---\nbody",
			want:    "version",
		},
		{
			name:    "name mismatch",
			content: "---\nname: other\ndescription: test\nversion: 1.0.0\n---\nbody",
			want:    "declares name",
		},
		{
			name:    "bad version",
			content: "---\nname: playbook\ndescription: test\nversion: v1\n---\nbody",
			want:    "semver",
		},
		{
			name:    "empty body",
			content: "---\nname: playbook\ndescription: test\nversion: 1.0.0\n---\n",
			want:    "empty",
		},
		{
			name:    "description too long",
			content: "---\nname: playbook\ndescription: " + strings.Repeat("用", MaxDescriptionSize+1) + "\nversion: 1.0.0\n---\nbody",
			want:    "description exceeds 300 characters",
		},
		{
			name:    "description multiple lines",
			content: "---\nname: playbook\ndescription: |\n  first line\n  second line\nversion: 1.0.0\n---\nbody",
			want:    "description must be a single line",
		},
		{
			name:    "body too large",
			content: "---\nname: playbook\ndescription: test\nversion: 1.0.0\n---\n" + strings.Repeat("a", MaxBodyBytes+1),
			want:    "skill body exceeds 32768 bytes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "playbook", "1.0.0", FileName)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			lock := Lock{Version: LockSchemaVersion, Skills: map[string]LockEntry{
				"playbook": {Version: "1.0.0", SHA256: strings.Repeat("0", 64)},
			}}
			if err := WriteLock(dir, lock); err != nil {
				t.Fatalf("write lock: %v", err)
			}
			if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestLoadMissingDirectoryDisablesSkills(t *testing.T) {
	registry, err := Load(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if registry.Len() != 0 {
		t.Fatalf("skills = %+v", registry.Snapshot())
	}
}

func TestRepositorySkillsVerify(t *testing.T) {
	registry, err := Load(filepath.Join("..", "..", "skills"))
	if err != nil {
		t.Fatalf("repository skills do not verify: %v", err)
	}
	if registry.Len() == 0 {
		t.Fatal("repository has no pinned skills")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func mustReadString(t *testing.T, path string) string {
	t.Helper()
	return string(mustRead(t, path))
}

func TestRegistryProvider(t *testing.T) {
	skill := Skill{
		Name:        "alpha",
		Version:     "1.0.0",
		Description: "用于测试注册表",
		SHA256:      strings.Repeat("a", 64),
		Body:        "body",
	}
	registry := NewRegistry(skill)
	ctx := context.Background()

	catalog, err := registry.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(catalog) != 1 || catalog[0].Name != "alpha" || catalog[0].Version != "1.0.0" {
		t.Fatalf("catalog = %+v", catalog)
	}
	catalog[0].Name = "mutated"
	if again, _ := registry.List(ctx); again[0].Name != "alpha" {
		t.Fatalf("List returned shared state: %+v", again)
	}

	loaded, err := registry.Resolve(ctx, skill.CatalogEntry())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if loaded.Body != "body" {
		t.Fatalf("body = %q", loaded.Body)
	}

	cases := []struct {
		name string
		pin  CatalogEntry
		code string
	}{
		{"version mismatch", CatalogEntry{Name: "alpha", Version: "2.0.0", SHA256: skill.SHA256}, UnavailableVersionMismatch},
		{"hash mismatch", CatalogEntry{Name: "alpha", Version: "1.0.0", SHA256: strings.Repeat("b", 64)}, UnavailableHashMismatch},
		{"unknown name", CatalogEntry{Name: "beta", Version: "1.0.0", SHA256: skill.SHA256}, UnavailableNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := registry.Resolve(ctx, tc.pin)
			var unavailableErr *UnavailableError
			if !errors.As(err, &unavailableErr) || unavailableErr.Code != tc.code {
				t.Fatalf("err = %v, want code %s", err, tc.code)
			}
		})
	}
}
