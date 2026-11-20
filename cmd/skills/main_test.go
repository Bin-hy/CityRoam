package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trippilot/internal/skills"
)

func writeTestSkill(t *testing.T, dir, name, version, description, body string) {
	t.Helper()
	path := filepath.Join(dir, name, version, skills.FileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\nversion: " + version + "\n---\n\n" + body
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func pinTestSkills(t *testing.T, dir string) {
	t.Helper()
	lock, _, err := skills.BuildLock(dir)
	if err != nil {
		t.Fatalf("build lock: %v", err)
	}
	if err := skills.WriteLock(dir, lock); err != nil {
		t.Fatalf("write lock: %v", err)
	}
}

func TestRunVerify(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "alpha", "1.0.0", "用于测试验证", "body")
	pinTestSkills(t, dir)

	var stdout, stderr bytes.Buffer
	if err := run(dir, runOptions{}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "verified 1 skill(s)") || !strings.Contains(stdout.String(), "alpha@1.0.0") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunWrite(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "alpha", "1.0.0", "用于测试写入", "body")

	var stdout, stderr bytes.Buffer
	if err := run(dir, runOptions{Write: true}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "wrote "+skills.LockFileName) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, skills.LockFileName)); err != nil {
		t.Fatalf("lock not written: %v", err)
	}
}

func TestRunLint(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "alpha", "1.0.0", "只有一句普通描述", "body")
	pinTestSkills(t, dir)

	var stdout, stderr bytes.Buffer
	if err := run(dir, runOptions{Lint: true}, &stdout, &stderr); err != nil {
		t.Fatalf("lint should not fail without -strict: %v", err)
	}
	if !strings.Contains(stdout.String(), "warning: alpha@1.0.0 "+skills.LintMissingTriggerCue) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunLintStrict(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "alpha", "1.0.0", "只有一句普通描述", "body")
	pinTestSkills(t, dir)

	var stdout, stderr bytes.Buffer
	err := run(dir, runOptions{Lint: true, Strict: true}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "lint warning") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunLintStrictClean(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "alpha", "1.0.0", "用于测试干净技能", "body")
	pinTestSkills(t, dir)

	var stdout, stderr bytes.Buffer
	if err := run(dir, runOptions{Lint: true, Strict: true}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v, stdout = %q", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "0 lint warning(s)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunRejectsConflictingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(t.TempDir(), runOptions{Write: true, Lint: true}, &stdout, &stderr); err == nil {
		t.Fatal("expected -write/-lint conflict")
	}
	if err := run(t.TempDir(), runOptions{Strict: true}, &stdout, &stderr); err == nil {
		t.Fatal("expected -strict without -lint conflict")
	}
}
