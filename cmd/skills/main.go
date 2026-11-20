package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"trippilot/internal/skills"
)

type runOptions struct {
	Write  bool
	Lint   bool
	Strict bool
}

func main() {
	dir := flag.String("dir", "skills", "skills 目录")
	write := flag.Bool("write", false, "写入或更新 skills.lock.json")
	lint := flag.Bool("lint", false, "检查技能编写规范（只告警）")
	strict := flag.Bool("strict", false, "与 -lint 联用，存在告警时返回非零")
	flag.Parse()

	opts := runOptions{Write: *write, Lint: *lint, Strict: *strict}
	if err := run(*dir, opts, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "skills:", err)
		os.Exit(1)
	}
}

func run(dir string, opts runOptions, stdout, stderr io.Writer) error {
	if opts.Write && opts.Lint {
		return errors.New("-write 与 -lint 不能同时使用")
	}
	if opts.Strict && !opts.Lint {
		return errors.New("-strict 只能与 -lint 一起使用")
	}
	if opts.Lint {
		return lintRun(dir, opts.Strict, stdout)
	}
	if opts.Write {
		return writeRun(dir, stdout, stderr)
	}
	return verifyRun(dir, stdout)
}

func lintRun(dir string, strict bool, stdout io.Writer) error {
	issues, err := skills.Lint(dir)
	if err != nil {
		return err
	}
	for _, issue := range issues {
		fmt.Fprintf(stdout, "warning: %s\n", issue)
	}
	fmt.Fprintf(stdout, "%d lint warning(s) in %s\n", len(issues), dir)
	if strict && len(issues) > 0 {
		return fmt.Errorf("%d lint warning(s)", len(issues))
	}
	return nil
}

func writeRun(dir string, stdout, stderr io.Writer) error {
	lock, warnings, err := skills.BuildLock(dir)
	if err != nil {
		return err
	}
	if err := skills.WriteLock(dir, lock); err != nil {
		return err
	}
	for _, warning := range warnings {
		fmt.Fprintln(stderr, "warning:", warning)
	}
	fmt.Fprintf(stdout, "wrote %s with %d pinned skill(s)\n", skills.LockFileName, len(lock.Skills))
	return nil
}

func verifyRun(dir string, stdout io.Writer) error {
	registry, err := skills.Load(dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "verified %d skill(s) from %s\n", registry.Len(), dir)
	for _, entry := range registry.Catalog() {
		fmt.Fprintf(stdout, "- %s@%s sha256=%s\n", entry.Name, entry.Version, entry.SHA256)
	}
	return nil
}
