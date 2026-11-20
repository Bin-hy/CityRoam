package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkillWithDescription(t *testing.T, dir, name, version, description, body string) {
	t.Helper()
	path := filepath.Join(dir, name, version, FileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\nversion: " + version + "\n---\n\n" + body
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func TestLintWarnsForBodyAndCue(t *testing.T) {
	dir := t.TempDir()
	writeSkillWithDescription(t, dir, "long-playbook", "1.0.0", "用于测试超长正文", strings.Repeat("line\n", MaxLintBodyLines+1))
	pinSkills(t, dir)

	issues, err := Lint(dir)
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	if len(issues) != 1 || issues[0].Rule != LintBodyTooLong {
		t.Fatalf("issues = %+v", issues)
	}

	dir = t.TempDir()
	writeSkillWithDescription(t, dir, "no-cue", "1.0.0", "研究清单", "short body")
	pinSkills(t, dir)
	issues, err = Lint(dir)
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	if len(issues) != 1 || issues[0].Rule != LintMissingTriggerCue {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestLintAcceptsCleanSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillWithDescription(t, dir, "clean-playbook", "1.0.0", "Use when planning a museum day.", "short body")
	pinSkills(t, dir)

	issues, err := Lint(dir)
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestLintPropagatesHardErrors(t *testing.T) {
	dir := t.TempDir()
	writeSkillWithDescription(t, dir, "unpinned", "1.0.0", "用于测试", "body")
	if _, err := Lint(dir); err == nil {
		t.Fatal("missing lock was accepted")
	}
}
