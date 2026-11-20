package skills

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func pinnedEntry(t *testing.T, dir, name string) CatalogEntry {
	t.Helper()
	registry, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	skill, ok := registry.Lookup(name)
	if !ok {
		t.Fatalf("skill %s not found", name)
	}
	return skill.CatalogEntry()
}

func TestLocalSourceLoadUsesExactVersion(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "playbook", "1.0.0", "old body")
	pinSkills(t, dir)
	writeSkill(t, dir, "playbook", "2.0.0", "new body")

	pin := pinnedEntry(t, dir, "playbook")
	skill, err := (LocalSource{}).Load(context.Background(), dir, pin)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if skill.Version != "1.0.0" || skill.Body != "old body" {
		t.Fatalf("resolved %s body=%q", skill.Version, skill.Body)
	}
}

func TestLocalSourceRejectsHashMismatch(t *testing.T) {
	dir := t.TempDir()
	path := writeSkill(t, dir, "playbook", "1.0.0", "body")
	pinSkills(t, dir)
	pin := pinnedEntry(t, dir, "playbook")

	if err := os.WriteFile(path, []byte(strings.ReplaceAll(mustReadString(t, path), "body", "edited")), 0o644); err != nil {
		t.Fatalf("edit: %v", err)
	}
	_, err := (LocalSource{}).Load(context.Background(), dir, pin)
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) || unavailable.Code != UnavailableHashMismatch {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), dir) {
		t.Fatalf("error leaked a path: %v", err)
	}
}

func TestLocalSourceMissingAndInvalidPin(t *testing.T) {
	dir := t.TempDir()
	pin := CatalogEntry{Name: "playbook", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}
	_, err := (LocalSource{}).Load(context.Background(), dir, pin)
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) || unavailable.Code != UnavailableNotFound {
		t.Fatalf("missing: %v", err)
	}

	bad := pin
	bad.Name = "../escape"
	_, err = (LocalSource{}).Load(context.Background(), dir, bad)
	if !errors.As(err, &unavailable) || unavailable.Code != UnavailableInvalidPin {
		t.Fatalf("invalid: %v", err)
	}
}

func TestLocalSourceScanCachesVersions(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "playbook", "1.0.0", "old")
	pinSkills(t, dir)
	writeSkill(t, dir, "playbook", "1.1.0", "new")

	result, err := (LocalSource{}).Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Skills) != 1 || result.Skills[0].Version != "1.0.0" {
		t.Fatalf("skills = %+v", result.Skills)
	}
	if got := result.Versions["playbook"]; len(got) != 2 {
		t.Fatalf("versions = %v", got)
	}
}
