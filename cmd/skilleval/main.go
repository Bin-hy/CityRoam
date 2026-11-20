package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"trippilot/internal/agent"
	"trippilot/internal/skillseval"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dir := flag.String("dir", "eval/skills-fixtures", "directory with pinned fixture skills")
	casesPath := flag.String("cases", "eval/skills-cases/cases.json", "JSON file with trigger cases")
	timeout := flag.Duration("timeout", skillseval.DefaultTimeout, "per-case timeout")
	flag.Parse()

	baseURL := strings.TrimSpace(os.Getenv("LLM_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	model := strings.TrimSpace(os.Getenv("LLM_MODEL"))
	missing := make([]string, 0, 3)
	if baseURL == "" {
		missing = append(missing, "LLM_BASE_URL")
	}
	if apiKey == "" {
		missing = append(missing, "LLM_API_KEY")
	}
	if model == "" {
		missing = append(missing, "LLM_MODEL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	cases, err := skillseval.LoadCases(*casesPath)
	if err != nil {
		return err
	}
	llm, err := agent.NewOpenAIClient(baseURL, apiKey, model)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	report, err := skillseval.Run(ctx, skillseval.Config{
		SkillsDir: *dir,
		Cases:     cases,
		LLM:       llm,
		Timeout:   *timeout,
	})
	if err != nil {
		return err
	}

	fmt.Printf("skill trigger eval: %s (%s)\n", *dir, model)
	fmt.Printf("%-24s %-10s %-8s %s\n", "case", "result", "loaded", "detail")
	for _, result := range report.Results {
		verdict := "PASS"
		detail := ""
		if result.Err != nil {
			verdict = "FAIL"
			detail = result.Err.Error()
		}
		loaded := strings.Join(result.Loaded, ",")
		if loaded == "" {
			loaded = "-"
		}
		fmt.Printf("%-24s %-10s %-8s %s\n", result.Case.ID, verdict, loaded, detail)
	}
	fmt.Printf("\npassed %d / %d, failed %d\n", report.Passed, len(report.Results), report.Failed)

	if report.Failed > 0 {
		return fmt.Errorf("%d skill trigger case(s) failed", report.Failed)
	}
	return nil
}
